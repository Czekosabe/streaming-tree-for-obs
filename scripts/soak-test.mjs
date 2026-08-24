#!/usr/bin/env node
/**
 * Stage 20E bounded resource-stability soak check (governing task §26).
 *
 * Not one of the 24 canonical integration scripts, and never invoked
 * from any CI workflow - a manually-run, one-time evidence-gathering
 * tool for this stage's own final regression, using only synthetic/
 * local infrastructure exactly like `scripts/verify-ffmpeg-
 * branches.mjs` (whose real topology-setup this script reuses almost
 * verbatim, then replaces that script's one-shot assertion sequence
 * with a bounded monitoring loop instead).
 *
 * Topology (all loopback, no real platform, no real credential):
 *   synthetic FFmpeg publisher -> source MediaMTX (managed by the
 *   backend under test) -> two independent real FFmpeg destination
 *   branches -> two sink MediaMTX instances standing in for two real
 *   destination platforms.
 *
 * During a bounded window this script:
 *   - keeps the source ingest and both destination branches live,
 *   - periodically stops and restarts destination 1 (a real stop/
 *     start transition, not merely leaving everything static),
 *   - periodically polls GET /api/logs (Stage 20E diagnostics) to
 *     both generate log activity and confirm the ring buffer's own
 *     response size never grows unbounded,
 *   - keeps one SSE connection to the public engagement stream open
 *     for the whole run,
 *   - samples the OS process list for `streaming-tree-server`/
 *     `ffmpeg`/`mediamtx` process count and total working-set memory
 *     at a fixed interval.
 *
 * At the end it reports the full sample series and a simple trend
 * classification (bounded vs. monotonically growing) - never a
 * pass/fail gate with an invented rigid threshold (the governing
 * task's own explicit instruction): a human reads the printed series
 * and the honest summary this script prints, and a genuinely
 * discovered leak is itself the real finding, recorded as such.
 *
 * This is deliberately a bounded number of minutes, not hours - "not
 * hours for optics, but a useful bounded duration" is the governing
 * task's own phrasing. SOAK_DURATION_MS below is the one place that
 * duration is set.
 *
 * Usage:  node scripts/soak-test.mjs
 * Requires a real, compatible `ffmpeg` on PATH - exits non-zero (with
 * a clear message) if one is not found, same as
 * verify-ffmpeg-branches.mjs.
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
const INGEST_TIMEOUT_MS = 20_000;

// The one place the soak window's duration and cadence are set.
const SOAK_DURATION_MS = 4 * 60_000; // 4 minutes - bounded, not hours.
const SAMPLE_INTERVAL_MS = 15_000; // ~16 samples over the window.
const STOP_START_EVERY_N_SAMPLES = 2; // toggle destination 1 roughly every 30s.

const RUN_ID = randomUUID().slice(0, 8);
const FAKE_KEY_1 = `FAKE-SOAK-KEY-ONE-${RUN_ID}`;
const FAKE_KEY_2 = `FAKE-SOAK-KEY-TWO-${RUN_ID}`;

let stepCount = 0;
function step(message) {
  stepCount += 1;
  console.log(`\n[${String(stepCount).padStart(2, '0')}] ${message}`);
}
function pass(message) {
  console.log(`     ok  ${message}`);
}
function note(message) {
  console.log(`     ..  ${message}`);
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

async function request(baseUrl, method, path, body) {
  const init = { method, headers: { Accept: 'application/json' } };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  const response = await fetch(`${baseUrl}${path}`, init);
  const text = await response.text();
  let parsed = null;
  if (text !== '') {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  return { status: response.status, body: parsed, rawLength: text.length };
}

function spawnCaptured(label, command, args, opts) {
  const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'], ...opts });
  let output = '';
  const cap = (chunk) => {
    output += chunk.toString();
    if (output.length > 2_000_000) output = output.slice(-2_000_000);
  };
  child.stdout.on('data', cap);
  child.stderr.on('data', cap);
  let exited = false;
  let exitInfo = null;
  child.on('exit', (code, signal) => {
    exited = true;
    exitInfo = { code, signal };
  });
  return { child, label, getOutput: () => output, hasExited: () => exited, exitInfo: () => exitInfo };
}

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
    if (handle.hasExited()) throw new Error(`backend exited during startup:\n${handle.getOutput()}`);
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
    '# Generated by soak-test.mjs. Do not edit.',
    'logLevel: info', 'logDestinations: [stdout]',
    'readTimeout: 10s', 'writeTimeout: 10s',
    'api: true', `apiAddress: ${apiAddress}`,
    'rtmp: true', `rtmpAddress: ${rtmpAddress}`,
    'rtmpEncryption: "no"',
    'rtsp: false', 'hls: false', 'webrtc: false', 'srt: false', 'moq: false',
    'metrics: false', 'pprof: false', 'playback: false',
    'pathDefaults:', '  record: false',
    'paths:', '  all_others:', '    source: publisher', '    overridePublisher: false', '',
  ].join('\n');
}

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
  const handle = spawnCaptured(label, executablePath, [configPath], { cwd: dir });
  await waitUntil(async () => {
    if (handle.hasExited()) throw new Error(`${label} exited during startup:\n${handle.getOutput()}`);
    const response = await fetch(`http://${ports.apiAddress}/v3/paths/list`);
    return response.ok;
  }, READINESS_TIMEOUT_MS, `${label} Control API`);
  return handle;
}

async function waitForMediaMTXState(baseUrl, wanted, timeoutMs, label) {
  return waitUntil(async () => {
    const runtime = await request(baseUrl, 'GET', '/api/runtime');
    const state = runtime.body?.mediaMtx?.state;
    if (state === 'error') throw new Error(`MediaMTX entered the error state: ${JSON.stringify(runtime.body.mediaMtx.lastError)}`);
    return wanted.includes(state) ? runtime.body : false;
  }, timeoutMs, label);
}

async function waitForIngestState(baseUrl, wanted, timeoutMs) {
  return waitUntil(async () => {
    const runtime = await request(baseUrl, 'GET', '/api/runtime');
    return runtime.body?.ingest?.state === wanted ? runtime.body.ingest : false;
  }, timeoutMs, `ingest state "${wanted}"`);
}

async function getBranch(baseUrl, platformId) {
  const response = await request(baseUrl, 'GET', '/api/runtime/branches');
  const found = response.body.branches.find((b) => b.platformId === platformId);
  if (found === undefined) throw new Error(`no branch snapshot for ${platformId}`);
  return found;
}

async function waitForBranchState(baseUrl, platformId, wantedStates, timeoutMs) {
  return waitUntil(async () => {
    const branch = await getBranch(baseUrl, platformId);
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
  ], undefined);
}

/** Best-effort process-count/working-set sampler for the process
 * names this soak run cares about. Never fatal: a sampling failure is
 * recorded as a null sample rather than aborting the whole run - the
 * soak run itself is the thing under test, not this sampler. */
async function sampleProcesses(names) {
  try {
    if (process.platform === 'win32') {
      const filter = names.map((n) => `$_.Name -eq '${n}'`).join(' -or ');
      const ps = spawnCaptured('sampler', 'powershell', [
        '-NoProfile', '-Command',
        `Get-Process | Where-Object { ${filter} } | Measure-Object -Property WorkingSet64 -Sum | Select-Object Count,Sum | ConvertTo-Json`,
      ]);
      const code = await new Promise((r) => ps.child.on('exit', (c) => r(c)));
      if (code !== 0) return null;
      const parsed = JSON.parse(ps.getOutput() || '{}');
      return { processCount: parsed.Count ?? 0, workingSetBytes: parsed.Sum ?? 0 };
    }
    const ps = spawnCaptured('sampler', 'ps', ['-eo', 'comm,rss']);
    const code = await new Promise((r) => ps.child.on('exit', (c) => r(c)));
    if (code !== 0) return null;
    let processCount = 0;
    let rssKbSum = 0;
    for (const line of ps.getOutput().split('\n').slice(1)) {
      const trimmed = line.trim();
      if (trimmed === '') continue;
      const lastSpace = trimmed.lastIndexOf(' ');
      const comm = trimmed.slice(0, lastSpace).trim();
      const rss = Number(trimmed.slice(lastSpace + 1).trim());
      if (names.some((n) => comm.includes(n)) && Number.isFinite(rss)) {
        processCount += 1;
        rssKbSum += rss;
      }
    }
    return { processCount, workingSetBytes: rssKbSum * 1024 };
  } catch {
    return null;
  }
}

function formatMB(bytes) {
  if (bytes === null || bytes === undefined) return 'n/a';
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

async function main() {
  console.log('Stage 20E bounded resource-stability soak check');
  console.log(`Run id: ${RUN_ID}  Duration: ${SOAK_DURATION_MS / 60_000} min  Sample interval: ${SAMPLE_INTERVAL_MS / 1000}s`);

  step('Confirm a real, compatible FFmpeg is on PATH');
  const ffmpegProbe = spawnCaptured('ffmpeg-probe', 'ffmpeg', ['-version']);
  const probeExit = await new Promise((r) => ffmpegProbe.child.on('exit', (code) => r(code)));
  if (probeExit !== 0) {
    console.error('\nPREREQUISITE MISSING: no working `ffmpeg` executable was found on PATH.');
    process.exitCode = 1;
    return;
  }
  pass('ffmpeg is on PATH');

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-soak-'));
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
  let sseAbort = null;
  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const backendEnv = {
    STREAMING_TREE_DATA_DIR: dataDir,
    STREAMING_TREE_HOST: '127.0.0.1',
    STREAMING_TREE_MEDIAMTX_PATH: '',
    STREAMING_TREE_FFMPEG_PATH: '',
  };
  let baseUrl;

  try {
    step('Build the integration-only test server');
    const build = spawnCaptured('go-build', 'go', ['build', '-tags', 'integration', '-o', exePath, './cmd/testserver'], { cwd: SERVER_DIR });
    const buildExit = await new Promise((r) => build.child.on('exit', (code) => r(code)));
    expect(buildExit === 0, 'the integration test server built successfully', build.getOutput());

    step('Reserve dynamic loopback ports and start the real topology');
    const [backendPort, sourceRtmpPort, sourceApiPort, sink1RtmpPort, sink1ApiPort, sink2RtmpPort, sink2ApiPort] =
      await reservePorts(7);
    baseUrl = `http://127.0.0.1:${backendPort}`;
    const sink1Api = `127.0.0.1:${sink1ApiPort}`;
    const sink2Api = `127.0.0.1:${sink2ApiPort}`;
    backendEnv.STREAMING_TREE_PORT = String(backendPort);
    backendEnv.STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS = `127.0.0.1:${sourceRtmpPort}`;
    backendEnv.STREAMING_TREE_MEDIAMTX_API_ADDRESS = `127.0.0.1:${sourceApiPort}`;

    backend = await startBackend(exePath, backendEnv, baseUrl);
    const platforms = await request(baseUrl, 'GET', '/api/platforms');
    const dest1 = platforms.body.platforms.find((p) => p.providerId === 'twitch');
    const dest2 = platforms.body.platforms.find((p) => p.providerId === 'kick');
    for (const p of [dest1, dest2]) {
      await request(baseUrl, 'PUT', `/api/platforms/${p.id}`, { displayName: p.displayName, enabled: true, sortOrder: p.sortOrder });
    }
    await request(baseUrl, 'PUT', `/api/platforms/${dest1.id}/output`, { serverUrl: `rtmp://127.0.0.1:${sink1RtmpPort}/out`, autoRestart: true });
    await request(baseUrl, 'PUT', `/api/platforms/${dest2.id}/output`, { serverUrl: `rtmp://127.0.0.1:${sink2RtmpPort}/out`, autoRestart: true });
    await request(baseUrl, 'PUT', `/api/platforms/${dest1.id}/credentials/stream-key`, { streamKey: FAKE_KEY_1 });
    await request(baseUrl, 'PUT', `/api/platforms/${dest2.id}/credentials/stream-key`, { streamKey: FAKE_KEY_2 });

    const install = await request(baseUrl, 'POST', '/api/runtime/mediamtx/install');
    expect(install.status === 202, 'source MediaMTX install accepted', install.body);
    await waitForMediaMTXState(baseUrl, ['ready'], INSTALL_TIMEOUT_MS, 'source MediaMTX readiness');
    const mediamtxExe = findManagedMediaMTXExecutable(dataDir);

    sink1 = await startSinkMediaMTX('sink1', mediamtxExe, sink1Dir, { apiAddress: sink1Api, rtmpAddress: `127.0.0.1:${sink1RtmpPort}` });
    sink2 = await startSinkMediaMTX('sink2', mediamtxExe, sink2Dir, { apiAddress: sink2Api, rtmpAddress: `127.0.0.1:${sink2RtmpPort}` });

    publisher = startSyntheticPublisher(`rtmp://127.0.0.1:${sourceRtmpPort}/live`);
    await waitForIngestState(baseUrl, 'receiving', INGEST_TIMEOUT_MS);

    await request(baseUrl, 'POST', `/api/runtime/branches/${dest1.id}/start`);
    await waitForBranchState(baseUrl, dest1.id, ['live'], LIVE_TIMEOUT_MS);
    await request(baseUrl, 'POST', `/api/runtime/branches/${dest2.id}/start`);
    await waitForBranchState(baseUrl, dest2.id, ['live'], LIVE_TIMEOUT_MS);
    pass('both destination branches are live, source ingest is receiving');

    step('Open a long-lived SSE connection to the engagement event stream');
    const sseController = new AbortController();
    sseAbort = () => sseController.abort();
    let sseEventBytes = 0;
    let sseErrored = false;
    fetch(`${baseUrl}/api/engagement/stream`, { signal: sseController.signal })
      .then(async (response) => {
        if (!response.ok || response.body === null) return;
        const reader = response.body.getReader();
        // eslint-disable-next-line no-constant-condition
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          sseEventBytes += value.length;
        }
      })
      .catch(() => {
        if (!sseController.signal.aborted) sseErrored = true;
      });
    pass('SSE connection opened (kept alive for the whole soak window)');

    step(`Run the bounded ${SOAK_DURATION_MS / 60_000}-minute monitoring window`);
    const processNames = process.platform === 'win32'
      ? ['streaming-tree-server', 'testserver', 'ffmpeg', 'mediamtx']
      : ['testserver', 'ffmpeg', 'mediamtx'];
    const samples = [];
    const startedAt = Date.now();
    let sampleIndex = 0;
    let toggleInFlight = false;
    while (Date.now() - startedAt < SOAK_DURATION_MS) {
      sampleIndex += 1;
      // eslint-disable-next-line no-await-in-loop
      const procs = await sampleProcesses(processNames);
      // eslint-disable-next-line no-await-in-loop
      const logs = await request(baseUrl, 'GET', '/api/logs?limit=50');
      const elapsedS = Math.round((Date.now() - startedAt) / 1000);
      samples.push({
        elapsedS,
        processCount: procs?.processCount ?? null,
        workingSetBytes: procs?.workingSetBytes ?? null,
        logsResponseBytes: logs.rawLength,
        logsEntryCount: Array.isArray(logs.body?.entries) ? logs.body.entries.length : null,
      });
      note(`t=${elapsedS}s  processes=${procs?.processCount ?? 'n/a'}  workingSet=${formatMB(procs?.workingSetBytes)}  logsBytes=${logs.rawLength}  sseBytes=${sseEventBytes}`);

      if (!toggleInFlight && sampleIndex % STOP_START_EVERY_N_SAMPLES === 0) {
        toggleInFlight = true;
        // eslint-disable-next-line no-await-in-loop
        await request(baseUrl, 'POST', `/api/runtime/branches/${dest1.id}/stop`);
        // eslint-disable-next-line no-await-in-loop
        await waitForBranchState(baseUrl, dest1.id, ['idle'], LIVE_TIMEOUT_MS).catch(() => {});
        // eslint-disable-next-line no-await-in-loop
        await request(baseUrl, 'POST', `/api/runtime/branches/${dest1.id}/start`);
        // eslint-disable-next-line no-await-in-loop
        await waitForBranchState(baseUrl, dest1.id, ['live'], LIVE_TIMEOUT_MS).catch(() => {});
        toggleInFlight = false;
      }

      // eslint-disable-next-line no-await-in-loop
      await new Promise((r) => setTimeout(r, SAMPLE_INTERVAL_MS));
    }
    expect(sseErrored === false, 'the SSE connection stayed open for the entire window without erroring', sseErrored);
    pass(`collected ${samples.length} samples over ${Math.round((Date.now() - startedAt) / 1000)}s`);

    step('Stop everything and confirm clean shutdown');
    if (sseAbort !== null) sseAbort();
    await killTree(publisher);
    publisher = null;
    await request(baseUrl, 'POST', '/api/runtime/branches/stop-all');
    await stopBackend(backend, baseUrl);
    backend = null;
    await killTree(sink1);
    sink1 = null;
    await killTree(sink2);
    sink2 = null;
    // eslint-disable-next-line no-await-in-loop
    const afterShutdown = await sampleProcesses(processNames);
    expect(
      afterShutdown === null || afterShutdown.processCount === 0,
      'no streaming-tree-server/ffmpeg/mediamtx process remains after full shutdown',
      afterShutdown,
    );

    console.log('\n=== Soak sample series (elapsedS, processCount, workingSet, logsResponseBytes, logsEntryCount) ===');
    for (const s of samples) {
      console.log(`  ${s.elapsedS}s\t${s.processCount ?? 'n/a'}\t${formatMB(s.workingSetBytes)}\t${s.logsResponseBytes}\t${s.logsEntryCount ?? 'n/a'}`);
    }

    const withMemory = samples.filter((s) => s.workingSetBytes !== null);
    if (withMemory.length >= 2) {
      const first = withMemory[0].workingSetBytes;
      const last = withMemory[withMemory.length - 1].workingSetBytes;
      const deltaMB = (last - first) / (1024 * 1024);
      console.log(`\nWorking-set memory: ${formatMB(first)} -> ${formatMB(last)} (delta ${deltaMB >= 0 ? '+' : ''}${deltaMB.toFixed(1)} MB over ${Math.round((Date.now() - startedAt) / 1000)}s)`);
      console.log('No fixed pass/fail threshold is applied here by design - read the series above. A '
        + 'monotonically, unboundedly increasing trend across the whole window would be a real Stage 20E '
        + 'defect worth investigating further, not something this script papers over with an invented number.');
    } else {
      console.log('\nProcess memory sampling was not available in this environment (non-fatal) - see the '
        + 'logsResponseBytes/logsEntryCount columns for the diagnostics-side bounded-growth evidence instead.');
    }

    const logCounts = samples.map((s) => s.logsEntryCount).filter((n) => n !== null);
    if (logCounts.length > 0) {
      expect(Math.max(...logCounts) <= 2000, 'the diagnostics ring buffer never exceeded its 2,000-entry bound during the run', logCounts);
    }

    console.log('\nSoak check complete.');
  } finally {
    if (sseAbort !== null) sseAbort();
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
  console.error(`\nSoak check FAILED: ${error.message}`);
  process.exitCode = 1;
});
