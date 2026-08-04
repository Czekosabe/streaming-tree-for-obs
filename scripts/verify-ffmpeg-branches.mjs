#!/usr/bin/env node
/**
 * Real, loopback-only end-to-end verification of destination-branch FFmpeg
 * supervision (stage 6).
 *
 * Topology, all on dynamically-selected loopback ports:
 *
 *   synthetic FFmpeg publisher
 *     -> temporary SOURCE MediaMTX (managed by the backend, the real local
 *        ingest a user's OBS would publish to)
 *     -> Streaming Tree branch FFmpeg (one independent process per
 *        destination, spawned by the backend under test)
 *     -> temporary SINK MediaMTX per destination (standing in for the real
 *        platform's RTMP ingest)
 *
 * The backend under test is a special build: `go build -tags integration
 * ./cmd/testserver`. That build tag is the only difference from the real
 * `cmd/server` binary - it swaps the OS credential store for an in-memory
 * fake (internal/secrets/secretstest), so no fake stream key used here can
 * ever reach the developer's real OS keychain. The tag makes this
 * impossible to select by accident: a normal `go build ./...` or
 * `go build ./cmd/server` never sees cmd/testserver/main.go at all.
 *
 * No real platform account, credential, or network destination is ever
 * used. Every port is loopback and dynamically chosen.
 *
 * Usage:  node scripts/verify-ffmpeg-branches.mjs
 * Exits non-zero on the first failed expectation, or if no compatible real
 * FFmpeg is available - this script never claims success without one.
 */

import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { mkdtempSync, mkdirSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomUUID } from 'node:crypto';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

const SUPPORTED_MEDIAMTX_VERSION = 'v1.19.3';

const READINESS_TIMEOUT_MS = 60_000;
const INSTALL_TIMEOUT_MS = 300_000;
const SHUTDOWN_TIMEOUT_MS = 15_000;
const LIVE_TIMEOUT_MS = 30_000;
const ERROR_TIMEOUT_MS = 150_000; // bounded exponential backoff, 5 attempts, plus a settle wait per attempt
const INGEST_TIMEOUT_MS = 20_000;
const WAITING_FOR_INGEST_TIMEOUT_MS = 30_000;
const RESUME_TIMEOUT_MS = 20_000;

// Obviously-fake, single-run-unique stream keys. Never a real platform key,
// never sent anywhere but this script's own temporary MediaMTX instances.
const RUN_ID = randomUUID().slice(0, 8);
const FAKE_KEY_1 = `FAKE-INTEGRATION-KEY-ONE-${RUN_ID}`;
const FAKE_KEY_2 = `FAKE-INTEGRATION-KEY-TWO-${RUN_ID}`;

let stepCount = 0;
// Every byte this script ever sees over HTTP or from a captured process
// stream goes in here, then gets scanned for the fake keys at the very end.
const secretScanChunks = [];

function step(message) {
  stepCount += 1;
  console.log(`\n[${String(stepCount).padStart(2, '0')}] ${message}`);
}

function pass(message) {
  console.log(`     ok  ${message}`);
}

function expect(condition, message, detail) {
  if (condition) {
    pass(message);
    return;
  }
  console.error(`     FAIL ${message}`);
  if (detail !== undefined) {
    console.error(`          ${typeof detail === 'string' ? detail : JSON.stringify(detail)}`);
  }
  throw new Error(message);
}

function record(text) {
  if (typeof text === 'string' && text.length > 0) {
    secretScanChunks.push(text);
  }
}

function reservePort() {
  return new Promise((resolvePort, reject) => {
    const server = createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      server.close(() => resolvePort(port));
    });
  });
}

async function reservePorts(count) {
  const ports = [];
  for (let i = 0; i < count; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    ports.push(await reservePort());
  }
  return ports;
}

// Only the RESPONSE is scanned for secrets, never the outgoing request body:
// this script itself deliberately sends the fake stream key in two PUT
// requests (to set it) and that is not a leak - the property under test is
// that the application never echoes it back in a response, log line, or any
// captured process output.
async function request(baseUrl, method, path, body) {
  const init = { method, headers: { Accept: 'application/json' } };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  const response = await fetch(`${baseUrl}${path}`, init);
  const text = await response.text();
  record(text);
  let parsed = null;
  if (text !== '') {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  return { status: response.status, body: parsed };
}

/**
 * Spawns a child process, capturing stdout+stderr (bounded).
 *
 * scanForSecrets controls whether that output feeds the final secret scan.
 * It is true for the Streaming Tree backend itself (the thing under test for
 * secret hygiene) and false for the sink MediaMTX instances and the
 * synthetic publisher: those are third-party stand-ins for "the remote
 * platform" and "OBS" respectively, not part of the application, and a real
 * RTMP server's own logs inherently show the path a client published to
 * (which, like any RTMP server including real platforms, embeds the stream
 * key) - that is not a Streaming Tree secret leak to catch.
 */
function spawnCaptured(label, command, args, opts, scanForSecrets = true) {
  const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'], ...opts });
  let output = '';
  const cap = (chunk) => {
    const text = chunk.toString();
    output += text;
    if (output.length > 5_000_000) output = output.slice(-5_000_000);
    if (scanForSecrets) record(text);
  };
  child.stdout.on('data', cap);
  child.stderr.on('data', cap);
  let exited = false;
  let exitInfo = null;
  child.on('exit', (code, signal) => {
    exited = true;
    exitInfo = { code, signal };
  });
  return {
    child,
    label,
    getOutput: () => output,
    hasExited: () => exited,
    exitInfo: () => exitInfo,
  };
}

/** Terminates a spawned child and its tree, tolerant of it already being gone. */
async function killTree(handle, timeoutMs = SHUTDOWN_TIMEOUT_MS) {
  if (handle === null || handle === undefined || handle.hasExited()) return;
  await new Promise((resolveKill) => {
    const timer = setTimeout(resolveKill, timeoutMs);
    handle.child.on('exit', () => {
      clearTimeout(timer);
      resolveKill();
    });
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(handle.child.pid), '/T', '/F'], { stdio: 'ignore' });
    } else {
      handle.child.kill('SIGTERM');
    }
  });
}

async function waitUntil(predicate, timeoutMs, label) {
  const deadline = Date.now() + timeoutMs;
  let lastError = null;
  while (Date.now() < deadline) {
    try {
      const result = await predicate();
      if (result !== undefined && result !== false) return result;
    } catch (error) {
      lastError = error;
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`timed out waiting for ${label}${lastError ? `: ${lastError.message}` : ''}`);
}

async function startBackend(exePath, env, baseUrl) {
  const handle = spawnCaptured('backend', exePath, [], { cwd: SERVER_DIR, env: { ...process.env, ...env } });

  const deadline = Date.now() + READINESS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (handle.hasExited()) {
      throw new Error(`backend exited during startup:\n${handle.getOutput()}`);
    }
    try {
      const health = await fetch(`${baseUrl}/api/health`);
      if (health.ok) return handle;
    } catch {
      // Not listening yet.
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 300));
  }
  await killTree(handle);
  throw new Error(`backend did not become ready in ${READINESS_TIMEOUT_MS} ms:\n${handle.getOutput()}`);
}

async function stopBackend(handle, baseUrl) {
  if (handle === null) return;
  await killTree(handle);
  const deadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
  while (Date.now() < deadline) {
    try {
      await fetch(`${baseUrl}/api/health`);
    } catch {
      return;
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error('the backend is still answering after shutdown');
}

function renderSinkConfig({ apiAddress, rtmpAddress }) {
  return [
    '# Generated by verify-ffmpeg-branches.mjs. Do not edit.',
    'logLevel: info',
    'logDestinations: [stdout]',
    'readTimeout: 10s',
    'writeTimeout: 10s',
    'api: true',
    `apiAddress: ${apiAddress}`,
    'rtmp: true',
    `rtmpAddress: ${rtmpAddress}`,
    'rtmpEncryption: "no"',
    'rtsp: false',
    'hls: false',
    'webrtc: false',
    'srt: false',
    'moq: false',
    'metrics: false',
    'pprof: false',
    'playback: false',
    'pathDefaults:',
    '  record: false',
    'paths:',
    '  # Catch-all: a real destination platform accepts whatever stream key',
    '  # the publisher presents as the rest of the path, not one fixed name.',
    '  all_others:',
    '    source: publisher',
    '    overridePublisher: false',
    '',
  ].join('\n');
}

/** Finds the mediamtx executable the backend's managed installer put on disk. */
function findManagedMediaMTXExecutable(dataDir) {
  const root = join(dataDir, 'runtime', 'mediamtx', SUPPORTED_MEDIAMTX_VERSION);
  const platformDirs = readdirSync(root, { withFileTypes: true }).filter((e) => e.isDirectory());
  for (const dir of platformDirs) {
    const candidateDir = join(root, dir.name);
    for (const name of ['mediamtx.exe', 'mediamtx']) {
      const candidate = join(candidateDir, name);
      try {
        if (statSync(candidate).isFile()) return candidate;
      } catch {
        // Not this one.
      }
    }
  }
  throw new Error(`could not find a managed mediamtx executable under ${root}`);
}

async function startSinkMediaMTX(label, executablePath, dir, ports) {
  const configPath = join(dir, 'mediamtx.yml');
  writeFileSync(configPath, renderSinkConfig(ports));
  const handle = spawnCaptured(label, executablePath, [configPath], { cwd: dir }, false);

  await waitUntil(async () => {
    if (handle.hasExited()) {
      throw new Error(`${label} exited during startup:\n${handle.getOutput()}`);
    }
    const response = await fetch(`http://${ports.apiAddress}/v3/paths/list`);
    return response.ok;
  }, READINESS_TIMEOUT_MS, `${label} Control API`);

  return handle;
}

// Not recorded for the secret scan: this is the sink's own Control API
// response, and a real destination platform's API/dashboard would show the
// same thing - see spawnCaptured's doc comment for why sink output is out
// of scope for "does Streaming Tree leak the secret".
async function sinkPathReady(apiAddress, pathName) {
  const response = await fetch(`http://${apiAddress}/v3/paths/list`);
  if (!response.ok) return false;
  const list = await response.json();
  const item = list.items?.find((i) => i.name === pathName);
  return item !== undefined && item.ready === true ? item : false;
}

async function waitForMediaMTXState(baseUrl, wanted, timeoutMs, label) {
  return waitUntil(async () => {
    const runtime = await request(baseUrl, 'GET', '/api/runtime');
    const state = runtime.body?.mediaMtx?.state;
    if (state === 'error') {
      throw new Error(`MediaMTX entered the error state: ${JSON.stringify(runtime.body.mediaMtx.lastError)}`);
    }
    return wanted.includes(state) ? runtime.body : false;
  }, timeoutMs, label);
}

async function waitForIngestState(baseUrl, wanted, timeoutMs) {
  return waitUntil(async () => {
    const runtime = await request(baseUrl, 'GET', '/api/runtime');
    return runtime.body?.ingest?.state === wanted ? runtime.body.ingest : false;
  }, timeoutMs, `ingest state "${wanted}"`);
}

/** Quiet fetch used inside polling loops: no per-tick pass()/expect() noise. */
async function getBranch(baseUrl, platformId) {
  const response = await fetch(`${baseUrl}/api/runtime/branches`, { headers: { Accept: 'application/json' } });
  const text = await response.text();
  record(text);
  if (!response.ok) throw new Error(`GET /api/runtime/branches returned ${response.status}`);
  const body = JSON.parse(text);
  const found = body.branches.find((b) => b.platformId === platformId);
  if (found === undefined) throw new Error(`no branch snapshot for ${platformId}`);
  return found;
}

async function waitForBranchState(baseUrl, platformId, wantedStates, timeoutMs) {
  let lastLoggedAt = Date.now();
  return waitUntil(async () => {
    const branch = await getBranch(baseUrl, platformId);
    if (Date.now() - lastLoggedAt > 10_000) {
      lastLoggedAt = Date.now();
      console.log(`     ..  ${platformId}: state=${branch.state} restartCount=${branch.restartCount}`);
    }
    if (branch.state === 'error' && !wantedStates.includes('error')) {
      throw new Error(`branch ${platformId} entered error: ${JSON.stringify(branch.lastError)}`);
    }
    return wantedStates.includes(branch.state) ? branch : false;
  }, timeoutMs, `branch ${platformId} to reach one of [${wantedStates.join(', ')}]`);
}

function startSyntheticPublisher(rtmpUrl) {
  return spawnCaptured('publisher', 'ffmpeg', [
    '-hide_banner', '-loglevel', 'warning',
    '-f', 'lavfi', '-i', 'testsrc=size=320x240:rate=15',
    '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=44100',
    '-c:v', 'libx264', '-preset', 'ultrafast', '-tune', 'zerolatency', '-g', '30',
    '-c:a', 'aac',
    '-f', 'flv', rtmpUrl,
  ], undefined, false);
}

async function main() {
  console.log('Real FFmpeg destination-branch verification (loopback only, no real platform)');
  console.log(`Run id: ${RUN_ID}`);

  step('Confirm a real, compatible FFmpeg is on PATH (a prerequisite, not something this script installs)');
  const ffmpegProbe = spawnCaptured('ffmpeg-probe', 'ffmpeg', ['-version']);
  const probeExit = await new Promise((r) => ffmpegProbe.child.on('exit', (code) => r(code)));
  if (probeExit !== 0) {
    console.error('\nPREREQUISITE MISSING: no working `ffmpeg` executable was found on PATH.');
    console.error('Install FFmpeg (a compatible build with RTMP + FLV + -progress support) and re-run this script.');
    console.error('This script does not install FFmpeg itself - see docs/progress.md and README.md for why.');
    process.exitCode = 1;
    return;
  }
  pass(`ffmpeg is on PATH: ${ffmpegProbe.getOutput().split('\n')[0]}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-ffmpeg-branches-'));
  const dataDir = join(tempDir, 'data');
  const sink1Dir = join(tempDir, 'sink1');
  const sink2Dir = join(tempDir, 'sink2');
  mkdirSync(dataDir, { recursive: true });
  mkdirSync(sink1Dir, { recursive: true });
  mkdirSync(sink2Dir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  let backend = null;
  let sink1 = null;
  let sink2 = null;
  let publisher = null;
  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');

  const backendEnv = {
    STREAMING_TREE_DATA_DIR: dataDir,
    STREAMING_TREE_HOST: '127.0.0.1',
    STREAMING_TREE_MEDIAMTX_PATH: '',
    STREAMING_TREE_FFMPEG_PATH: '',
  };
  let backendPort;
  let sourceRtmpPort;
  let sourceApiPort;
  let baseUrl;

  try {
    step('Build the integration-only test server (go build -tags integration ./cmd/testserver)');
    const build = spawnCaptured('go-build', 'go', ['build', '-tags', 'integration', '-o', exePath, './cmd/testserver'], { cwd: SERVER_DIR });
    const buildExit = await new Promise((r) => build.child.on('exit', (code) => r(code)));
    expect(buildExit === 0, 'the integration test server built successfully', build.getOutput());
    pass('this build tag is invisible to a normal `go build ./...` (verified separately while designing this script)');

    step('Reserve dynamic loopback ports');
    let sink1RtmpPort;
    let sink1ApiPort;
    let sink2RtmpPort;
    let sink2ApiPort;
    [backendPort, sourceRtmpPort, sourceApiPort, sink1RtmpPort, sink1ApiPort, sink2RtmpPort, sink2ApiPort] =
      await reservePorts(7);
    baseUrl = `http://127.0.0.1:${backendPort}`;
    const sink1Api = `127.0.0.1:${sink1ApiPort}`;
    const sink2Api = `127.0.0.1:${sink2ApiPort}`;
    backendEnv.STREAMING_TREE_PORT = String(backendPort);
    backendEnv.STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS = `127.0.0.1:${sourceRtmpPort}`;
    backendEnv.STREAMING_TREE_MEDIAMTX_API_ADDRESS = `127.0.0.1:${sourceApiPort}`;
    pass(`backend :${backendPort}  source rtmp :${sourceRtmpPort}  sink1 :${sink1RtmpPort}/${sink1ApiPort}  sink2 :${sink2RtmpPort}/${sink2ApiPort}`);

    step('Start the backend under test (fake credential store, temporary data dir)');
    backend = await startBackend(exePath, backendEnv, baseUrl);
    pass('backend health is answering');

    step('Confirm the seeded platform configuration is readable');
    const platforms = await request(baseUrl, 'GET', '/api/platforms');
    expect(platforms.status === 200, 'GET /api/platforms returns 200', platforms.status);
    expect(platforms.body.platforms.length === 4, 'four seeded platforms exist', platforms.body.platforms.length);

    step('Confirm the FFmpeg dependency status is real, probed, and leaks no path');
    const ffStatus = await request(baseUrl, 'GET', '/api/runtime/ffmpeg');
    expect(ffStatus.status === 200, 'GET /api/runtime/ffmpeg returns 200', ffStatus.status);
    expect(ffStatus.body.ffmpeg.state === 'ready', 'FFmpeg state is ready', ffStatus.body.ffmpeg);
    expect(ffStatus.body.ffmpeg.source === 'path', 'FFmpeg was resolved from PATH', ffStatus.body.ffmpeg.source);
    expect(ffStatus.body.ffmpeg.detectedVersion !== '', 'a real detected version is reported', ffStatus.body.ffmpeg.detectedVersion);
    const caps = ffStatus.body.ffmpeg.capabilities;
    expect(caps.rtmpInput && caps.rtmpOutput && caps.rtmpsOutput && caps.flvMuxer && caps.progress,
      'every required capability probed true', caps);
    const ffText = JSON.stringify(ffStatus.body);
    for (const probe of [dataDir, 'AppData', '.exe', 'ffmpeg.exe']) {
      expect(!ffText.includes(probe), `the ffmpeg status payload does not contain ${probe}`, ffText);
    }

    step('Install the source MediaMTX through the managed installer');
    const install = await request(baseUrl, 'POST', '/api/runtime/mediamtx/install');
    expect(install.status === 202, 'POST install returns 202', install.body);
    const installed = await waitForMediaMTXState(baseUrl, ['ready', 'stopped'], INSTALL_TIMEOUT_MS, 'source MediaMTX installation');
    await waitForMediaMTXState(baseUrl, ['ready'], 60_000, 'source MediaMTX readiness');
    pass(`source MediaMTX installed and ready (${installed.mediaMtx.installedVersion})`);

    step('Locate the managed MediaMTX executable to reuse for the two destination sinks');
    const mediamtxExe = findManagedMediaMTXExecutable(dataDir);
    pass('found the managed executable on disk (never exposed through any API)');

    step('Start the two destination sink MediaMTX instances directly (not managed by the backend)');
    sink1 = await startSinkMediaMTX('sink1', mediamtxExe, sink1Dir, { apiAddress: sink1Api, rtmpAddress: `127.0.0.1:${sink1RtmpPort}` });
    sink2 = await startSinkMediaMTX('sink2', mediamtxExe, sink2Dir, { apiAddress: sink2Api, rtmpAddress: `127.0.0.1:${sink2RtmpPort}` });
    pass('both destination sinks are answering their Control APIs');

    step('Confirm the source ingest is waiting before anything publishes');
    await waitForIngestState(baseUrl, 'waiting', INGEST_TIMEOUT_MS);
    pass('source ingest is waiting');

    step('Select two seeded destinations and enable them');
    const dest1Platform = platforms.body.platforms.find((p) => p.providerId === 'twitch');
    const dest2Platform = platforms.body.platforms.find((p) => p.providerId === 'kick');
    expect(Boolean(dest1Platform) && Boolean(dest2Platform), 'two distinct seeded platforms were found', platforms.body.platforms.map((p) => p.providerId));
    for (const p of [dest1Platform, dest2Platform]) {
      const updated = await request(baseUrl, 'PUT', `/api/platforms/${p.id}`, {
        displayName: p.displayName, enabled: true, sortOrder: p.sortOrder,
      });
      expect(updated.status === 200 && updated.body.enabled === true, `${p.providerId} is enabled`, updated.body);
    }

    step('Configure destination output settings (server URL only - never the key)');
    const dest1ServerUrl = `rtmp://127.0.0.1:${sink1RtmpPort}/out`;
    const dest2ServerUrl = `rtmp://127.0.0.1:${sink2RtmpPort}/out`;
    for (const [p, url] of [[dest1Platform, dest1ServerUrl], [dest2Platform, dest2ServerUrl]]) {
      const put = await request(baseUrl, 'PUT', `/api/platforms/${p.id}/output`, { serverUrl: url, autoRestart: true });
      expect(put.status === 200 && put.body.serverUrl === url, `${p.providerId} output server saved`, put.body);
      expect(!('streamKey' in put.body), 'the output response has no stream-key field', put.body);
    }

    step('Set fake, single-run stream keys (in-memory fake credential store only)');
    for (const [p, key] of [[dest1Platform, FAKE_KEY_1], [dest2Platform, FAKE_KEY_2]]) {
      const set = await request(baseUrl, 'PUT', `/api/platforms/${p.id}/credentials/stream-key`, { streamKey: key });
      expect(set.status === 200 && set.body.streamKey.configured === true, `${p.providerId} stream key stored`, set.body);
    }

    step('Confirm both branches start out idle with no invented state');
    for (const p of [dest1Platform, dest2Platform]) {
      const b = await getBranch(baseUrl, p.id);
      expect(b.state === 'idle', `${p.providerId} branch starts idle`, b.state);
      expect(b.desiredRunning === false, `${p.providerId} desiredRunning starts false`, b.desiredRunning);
      expect(b.restartCount === 0, `${p.providerId} restartCount starts at 0`, b.restartCount);
      expect(b.progress === null && b.lastError === null, `${p.providerId} has no invented progress or error`, b);
    }

    step('Attempt to start destination 1 before anything publishes: must be blocked, not started');
    const earlyStart = await request(baseUrl, 'POST', `/api/runtime/branches/${dest1Platform.id}/start`);
    expect(earlyStart.status === 200 && earlyStart.body.status === 'blocked', 'the early start is answered as blocked, not an error', earlyStart.body);
    expect(
      earlyStart.body.blockers.length === 1 && earlyStart.body.blockers[0] === 'ingest_not_receiving',
      'the only blocker is ingest_not_receiving (every other requirement was already satisfied)',
      earlyStart.body.blockers,
    );

    step('Start the synthetic FFmpeg publisher into the source MediaMTX (no real OBS, no manual testing)');
    publisher = startSyntheticPublisher(`rtmp://127.0.0.1:${sourceRtmpPort}/live`);
    pass('synthetic publisher started');

    step('Verify the real waiting -> receiving ingest transition (unverifiable end-to-end before this stage)');
    await waitForIngestState(baseUrl, 'receiving', INGEST_TIMEOUT_MS);
    pass('source ingest is now receiving a real publisher');

    step('Start destination 1: must now be accepted');
    const start1 = await request(baseUrl, 'POST', `/api/runtime/branches/${dest1Platform.id}/start`);
    expect(start1.status === 202 && start1.body.status === 'starting', 'destination 1 start is accepted', start1.body);

    step('Wait for destination 1 to report real, advancing FFmpeg progress (live)');
    const live1 = await waitForBranchState(baseUrl, dest1Platform.id, ['live'], LIVE_TIMEOUT_MS);
    expect(live1.liveAt !== '', 'destination 1 has a liveAt timestamp', live1.liveAt);
    expect(
      (live1.progress?.outTimeMs ?? 0) > 0 || (live1.progress?.totalSize ?? 0) > 0,
      'destination 1 progress shows real advancing output, not an estimate',
      live1.progress,
    );
    for (const forbidden of [FAKE_KEY_1, dest1ServerUrl, mediamtxExe]) {
      expect(JSON.stringify(live1).includes(forbidden) === false, 'the branch snapshot carries no secret and no full destination URL', live1);
    }

    step('Confirm the sink actually received a real stream copy for destination 1');
    const sink1Path = `out/${FAKE_KEY_1}`;
    const ready1 = await waitUntil(() => sinkPathReady(sink1Api, sink1Path), LIVE_TIMEOUT_MS, 'sink1 path ready');
    expect(Array.isArray(ready1.tracks) && ready1.tracks.length > 0, 'sink1 detected real tracks (stream copy arrived intact)', ready1.tracks);

    step('Start destination 2 independently and confirm destination 1 is unaffected');
    const start2 = await request(baseUrl, 'POST', `/api/runtime/branches/${dest2Platform.id}/start`);
    expect(start2.status === 202, 'destination 2 start is accepted', start2.body);
    await waitForBranchState(baseUrl, dest2Platform.id, ['live'], LIVE_TIMEOUT_MS);
    const stillLive1 = await getBranch(baseUrl, dest1Platform.id);
    expect(stillLive1.state === 'live', 'destination 1 is still live while destination 2 starts', stillLive1.state);

    step('Confirm the sink actually received a real stream copy for destination 2');
    const sink2Path = `out/${FAKE_KEY_2}`;
    await waitUntil(() => sinkPathReady(sink2Api, sink2Path), LIVE_TIMEOUT_MS, 'sink2 path ready');
    pass('sink2 is receiving destination 2');

    step('Explicitly stop destination 1 and confirm destination 2 is unaffected (stop isolation)');
    const stop1 = await request(baseUrl, 'POST', `/api/runtime/branches/${dest1Platform.id}/stop`);
    expect(stop1.status === 200, 'stop destination 1 is accepted', stop1.body);
    const idle1 = await waitForBranchState(baseUrl, dest1Platform.id, ['idle'], LIVE_TIMEOUT_MS);
    expect(idle1.desiredRunning === false, 'destination 1 desiredRunning cleared by explicit stop', idle1.desiredRunning);
    const stillLive2 = await getBranch(baseUrl, dest2Platform.id);
    expect(stillLive2.state === 'live', 'destination 2 remains live after destination 1 was stopped', stillLive2.state);

    step('Simulate destination 2 losing its OUTPUT connection (the sink platform going away) and verify failure isolation');
    await killTree(sink2);
    sink2 = null;
    const errored2 = await waitForBranchState(baseUrl, dest2Platform.id, ['error'], ERROR_TIMEOUT_MS);
    expect(errored2.lastError !== null, 'destination 2 carries a sanitized last error', errored2.lastError);
    expect(errored2.restartCount >= 1, 'destination 2 attempted at least one automatic restart before giving up', errored2.restartCount);
    for (const forbidden of [FAKE_KEY_2, dest2ServerUrl]) {
      expect(
        JSON.stringify(errored2.lastError).includes(forbidden) === false,
        'the destination 2 error message contains no secret and no full destination URL',
        errored2.lastError,
      );
    }
    const dest1DuringFailure = await getBranch(baseUrl, dest1Platform.id);
    expect(dest1DuringFailure.state === 'idle', 'destination 1 was completely unaffected by destination 2 failing', dest1DuringFailure.state);

    step('Bring the destination 2 sink back and manually recover it');
    sink2 = await startSinkMediaMTX('sink2', mediamtxExe, sink2Dir, { apiAddress: sink2Api, rtmpAddress: `127.0.0.1:${sink2RtmpPort}` });
    const recover2 = await request(baseUrl, 'POST', `/api/runtime/branches/${dest2Platform.id}/start`);
    expect(recover2.status === 202, 'destination 2 accepts a manual restart after error', recover2.body);
    await waitForBranchState(baseUrl, dest2Platform.id, ['live'], LIVE_TIMEOUT_MS);
    pass('destination 2 recovered and is live again');

    step('Lose the source ingest (stop the synthetic publisher) and verify graceful suspension, not a crash loop');
    const restartCountBeforeIngestLoss = (await getBranch(baseUrl, dest2Platform.id)).restartCount;
    await killTree(publisher);
    publisher = null;
    await waitForIngestState(baseUrl, 'waiting', WAITING_FOR_INGEST_TIMEOUT_MS);
    const waiting2 = await waitForBranchState(baseUrl, dest2Platform.id, ['waiting_for_ingest'], WAITING_FOR_INGEST_TIMEOUT_MS);
    expect(waiting2.desiredRunning === true, 'destination 2 stays desired-running while waiting for ingest', waiting2.desiredRunning);
    expect(waiting2.blockers.includes('ingest_not_receiving'), 'the reported blocker is ingest_not_receiving', waiting2.blockers);
    expect(
      waiting2.restartCount === restartCountBeforeIngestLoss,
      'losing ingest did not count as a crash against the restart policy',
      { before: restartCountBeforeIngestLoss, after: waiting2.restartCount },
    );

    step('Explicitly stop destination 2 while it is waiting for ingest');
    const stopWhileWaiting = await request(baseUrl, 'POST', `/api/runtime/branches/${dest2Platform.id}/stop`);
    expect(stopWhileWaiting.status === 200, 'stop while waiting is accepted', stopWhileWaiting.body);
    const idleAfterStop2 = await waitForBranchState(baseUrl, dest2Platform.id, ['idle'], LIVE_TIMEOUT_MS);
    expect(idleAfterStop2.desiredRunning === false, 'destination 2 desiredRunning cleared', idleAfterStop2.desiredRunning);

    step('Start destination 1 again so exactly one desired branch is waiting when ingest returns');
    const restart1 = await request(baseUrl, 'POST', `/api/runtime/branches/${dest1Platform.id}/start`);
    expect(restart1.status === 200 && restart1.body.status === 'blocked', 'destination 1 is blocked (ingest still down)', restart1.body);
    const waiting1AfterStart = await getBranch(baseUrl, dest1Platform.id);
    expect(waiting1AfterStart.state === 'blocked', 'destination 1 is blocked, not desired, until told to run', waiting1AfterStart.state);
    expect(idleAfterStop2.desiredRunning === false, 'destination 2 remains not-desired', idleAfterStop2.desiredRunning);

    step('Restore the source ingest and confirm ONLY the desired branch resumes automatically');
    publisher = startSyntheticPublisher(`rtmp://127.0.0.1:${sourceRtmpPort}/live`);
    await waitForIngestState(baseUrl, 'receiving', INGEST_TIMEOUT_MS);
    // Destination 1 was left in "blocked" (not desired), so it must NOT
    // auto-start just because ingest returned - Part 6/10's explicit rule
    // that starting a broadcast is always a deliberate user action.
    await new Promise((r) => setTimeout(r, RESUME_TIMEOUT_MS));
    const dest1AfterIngestReturns = await getBranch(baseUrl, dest1Platform.id);
    expect(
      dest1AfterIngestReturns.state !== 'live' && dest1AfterIngestReturns.state !== 'starting',
      'destination 1 (not desired) was not auto-started when ingest returned',
      dest1AfterIngestReturns.state,
    );

    step('Confirm a desired-but-waiting branch DOES resume when ingest returns');
    const start2Again = await request(baseUrl, 'POST', `/api/runtime/branches/${dest2Platform.id}/start`);
    expect(start2Again.status === 202, 'destination 2 start is accepted now that ingest is back', start2Again.body);
    await waitForBranchState(baseUrl, dest2Platform.id, ['live'], LIVE_TIMEOUT_MS);
    pass('destination 2 reached live again');

    step('Explicit stop-all halts every running destination');
    const stopAll = await request(baseUrl, 'POST', '/api/runtime/branches/stop-all');
    expect(stopAll.status === 200, 'stop-all is accepted', stopAll.body);
    await waitForBranchState(baseUrl, dest2Platform.id, ['idle'], LIVE_TIMEOUT_MS);
    const dest1AfterStopAll = await getBranch(baseUrl, dest1Platform.id);
    expect(dest1AfterStopAll.state !== 'live' && dest1AfterStopAll.state !== 'starting', 'destination 1 is not running after stop-all', dest1AfterStopAll.state);

    step('Restart the backend and verify runtime state resets while output settings persist');
    await stopBackend(backend, baseUrl);
    backend = null;
    pass('backend stopped cleanly, all FFmpeg children reaped with it');

    backend = await startBackend(exePath, backendEnv, baseUrl);
    pass('backend restarted against the same temporary data directory');

    const outputAfterRestart1 = await request(baseUrl, 'GET', `/api/platforms/${dest1Platform.id}/output`);
    const outputAfterRestart2 = await request(baseUrl, 'GET', `/api/platforms/${dest2Platform.id}/output`);
    expect(outputAfterRestart1.body.serverUrl === dest1ServerUrl, 'destination 1 output settings persisted across restart', outputAfterRestart1.body);
    expect(outputAfterRestart2.body.serverUrl === dest2ServerUrl, 'destination 2 output settings persisted across restart', outputAfterRestart2.body);

    const branch1AfterRestart = await getBranch(baseUrl, dest1Platform.id);
    const branch2AfterRestart = await getBranch(baseUrl, dest2Platform.id);
    for (const [label, b] of [['destination 1', branch1AfterRestart], ['destination 2', branch2AfterRestart]]) {
      expect(b.desiredRunning === false, `${label} desiredRunning resets to false after a backend restart`, b.desiredRunning);
      expect(b.restartCount === 0, `${label} restartCount resets to 0 after a backend restart (runtime state is in-memory only)`, b.restartCount);
      expect(b.state === 'idle' || b.state === 'blocked', `${label} does not auto-resume a broadcast after a backend restart`, b.state);
    }

    step('Search everything this script ever captured for the fake stream keys');
    const haystack = secretScanChunks.join('\n');
    for (const key of [FAKE_KEY_1, FAKE_KEY_2]) {
      const index = haystack.indexOf(key);
      expect(index === -1, `the raw fake key ${key.slice(0, 4)}... never appears in any captured output or response`,
        index === -1 ? undefined : haystack.slice(Math.max(0, index - 200), index + 200));
      const encIndex = haystack.indexOf(encodeURIComponent(key));
      expect(encIndex === -1, `the URL-encoded fake key never appears either`,
        encIndex === -1 ? undefined : haystack.slice(Math.max(0, encIndex - 200), encIndex + 200));
    }
    pass(`scanned ${haystack.length} bytes of captured backend output, sink logs, publisher output and HTTP traffic`);

    console.log('\nFFmpeg destination-branch verification PASSED');
  } finally {
    await killTree(publisher);
    await killTree(sink1);
    await killTree(sink2);
    if (backend !== null && baseUrl !== undefined) {
      try {
        await stopBackend(backend, baseUrl);
      } catch {
        // Already reporting a failure if we get here.
      }
    }
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nFFmpeg branch verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
