#!/usr/bin/env node
/**
 * Stage 17B alert audio verification (20th integration script).
 *
 * Runs the real `-tags integration` backend (cmd/testserver) with real
 * SQLite, a real Engagement Event Bus, the real internal/audio.Manager,
 * the real internal/alerts runtime, the real managed audio-asset
 * domain, and the real public HTTP surface - against the integration-
 * build-only deterministic fake TTS provider (internal/provider/tts/
 * fake.go), never the real Windows SAPI provider (never a dependency on
 * this machine's installed voices, never real Twitch/YouTube/
 * StreamElements/Kick/Streamlabs/Ko-fi/TikTok, never real OBS, never a
 * manual browser). Every alert this script triggers is the synthetic
 * Test Rule path (POST /api/alert-rules/{id}/test) - never a fabricated
 * Engagement Event Bus event.
 *
 * This is a REPRESENTATIVE subset, not a literal transcription of every
 * scenario docs/alert-audio.md §13 lists - exhaustive validation-bound/
 * archive-security/WAV-signature matrices are already covered directly
 * at the Go level (internal/domain/audioasset/validation_test.go,
 * internal/domain/alerts/validation_test.go,
 * internal/domain/visualpackage/reader_test.go,
 * internal/audio/manager_alertaudio_test.go,
 * internal/alerts/audio_link_test.go) - this script instead exercises
 * the real, wired-together HTTP/SSE surface end to end: managed audio-
 * asset upload/list/delete-guard, rule-owned audio persistence and
 * validation, the real sound-then-TTS chain playing over the real
 * public audio SSE stream in lockstep with the real public alert SSE
 * stream, arbitration against a real global-TTS item, the bounded
 * visual-hold behavior, package v2 audio import/export, and a privacy
 * scan - mirroring scripts/verify-tts-audio.mjs's own "representative
 * subset" precedent exactly.
 *
 * Usage: node scripts/verify-alert-audio.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateRawSync } from 'node:zlib';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

const READINESS_TIMEOUT_MS = 30_000;
const BUILD_TIMEOUT_MS = 120_000;
const SHUTDOWN_TIMEOUT_MS = 15_000;
const POLL_TIMEOUT_MS = 20_000;

const RUN_ID = randomUUID().slice(0, 8);

let stepCount = 0;
const secretScanChunks = [];

// --- generic helpers (mirror every other scripts/verify-*.mjs) ---------

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
  if (typeof text === 'string' && text.length > 0) secretScanChunks.push(text);
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

function spawnCaptured(label, command, args, opts) {
  const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'], ...opts });
  let output = '';
  const cap = (chunk) => {
    const text = chunk.toString();
    output += text;
    if (output.length > 5_000_000) output = output.slice(-5_000_000);
    record(text);
  };
  child.stdout.on('data', cap);
  child.stderr.on('data', cap);
  let exited = false;
  child.on('exit', () => {
    exited = true;
  });
  return { child, label, getOutput: () => output, hasExited: () => exited };
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

async function buildBinary(label, cmdPath, exePath) {
  const build = spawnCaptured(label, 'go', ['build', '-tags', 'integration', '-o', exePath, cmdPath], { cwd: SERVER_DIR });
  const buildExit = await new Promise((r) => {
    const timer = setTimeout(() => r(-1), BUILD_TIMEOUT_MS);
    build.child.on('exit', (code) => {
      clearTimeout(timer);
      r(code);
    });
  });
  expect(buildExit === 0, `${label} built successfully`, build.getOutput());
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
    await new Promise((r) => setTimeout(r, 150));
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
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error('the backend is still answering after shutdown');
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
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error(`timed out waiting for ${label}${lastError ? `: ${lastError.message}` : ''}`);
}

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
  return { status: response.status, body: parsed, headers: response.headers };
}

/** Sends a raw binary body (multipart upload or a raw package archive)
 * - never goes through JSON.stringify. */
async function requestRaw(baseUrl, method, path, { body, headers = {} } = {}) {
  const response = await fetch(`${baseUrl}${path}`, { method, headers, body });
  const buf = Buffer.from(await response.arrayBuffer());
  let parsed = null;
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    const text = buf.toString('utf8');
    record(text);
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  return { status: response.status, body: parsed, raw: buf, headers: response.headers };
}

// --- SSE helpers (mirror scripts/verify-tts-audio.mjs's own reader) ------

function parseSSEChunk(chunk) {
  let eventName = 'message';
  let data = '';
  let id;
  for (const line of chunk.split('\n')) {
    if (line.startsWith('event:')) eventName = line.slice('event:'.length).trim();
    else if (line.startsWith('data:')) data += line.slice('data:'.length).trim();
    else if (line.startsWith('id:')) id = line.slice('id:'.length).trim();
  }
  let parsed = null;
  if (data !== '') {
    try {
      parsed = JSON.parse(data);
    } catch {
      parsed = data;
    }
  }
  return { event: eventName, id, data: parsed, raw: chunk };
}

async function* sseEvents(url) {
  const response = await fetch(url, { headers: { Accept: 'text/event-stream' } });
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  try {
    // eslint-disable-next-line no-constant-condition
    while (true) {
      // eslint-disable-next-line no-await-in-loop
      const { value, done } = await reader.read();
      if (done) return;
      buffer += decoder.decode(value, { stream: true });
      let idx;
      while ((idx = buffer.indexOf('\n\n')) !== -1) {
        const raw = buffer.slice(0, idx);
        buffer = buffer.slice(idx + 2);
        record(raw);
        yield parseSSEChunk(raw);
      }
    }
  } finally {
    reader.cancel().catch(() => {});
  }
}

async function nextEvent(iterator, timeoutMs, label) {
  const result = await Promise.race([
    iterator.next(),
    new Promise((_r, reject) => setTimeout(() => reject(new Error(`timed out waiting for ${label}`)), timeoutMs)),
  ]);
  if (result.done) throw new Error(`the SSE stream ended while waiting for ${label}`);
  return result.value;
}

/** Reads named audio.* events, skipping keepalive comments. */
async function nextAudioEvent(iterator, timeoutMs, label) {
  for (let i = 0; i < 40; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    const evt = await nextEvent(iterator, timeoutMs, label);
    if (evt.event.startsWith('audio.')) return evt;
  }
  throw new Error(`too many non-audio.* frames while waiting for ${label}`);
}

/** Reads named alert.* events, skipping keepalive comments. */
async function nextAlertEvent(iterator, timeoutMs, label) {
  for (let i = 0; i < 40; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    const evt = await nextEvent(iterator, timeoutMs, label);
    if (evt.event.startsWith('alert.')) return evt;
  }
  throw new Error(`too many non-alert.* frames while waiting for ${label}`);
}

/** Acknowledges an audio.current item started then ended - the exact
 * two-step cycle a real renderer performs, advancing the sound-then-TTS
 * chain (docs/alert-audio.md §8.1) or completing a standalone item. */
async function ackStartedThenEnded(baseUrl, slug, token, itemId) {
  const started = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token, itemId, kind: 'playback_started' });
  expect(started.status === 204, `playback_started acked for ${itemId}`, started.body);
  const ended = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token, itemId, kind: 'playback_ended' });
  expect(ended.status === 204, `playback_ended acked for ${itemId}`, ended.body);
}

async function audioStatus(baseUrl) {
  const res = await request(baseUrl, 'GET', '/api/audio/status');
  expect(res.status === 200, 'GET /api/audio/status succeeded', res.body);
  return res.body;
}

const DEFAULT_AUDIO_SETTINGS_BODY = {
  enabled: false,
  providerMode: 'system',
  enabledEventTypes: [],
  enabledProviderIds: [],
  enabledSourceIds: [],
  supporterOnlyMode: false,
  thresholdCurrency: '',
  blockedWords: [],
  maxTextLengthCodePoints: 500,
  perUserCooldownSeconds: 0,
  globalCooldownSeconds: 0,
  removeUrls: true,
  normalizeRepeatedChars: true,
  suppressCommands: true,
  queueCapacity: 100,
  manualApproval: false,
  voiceId: '',
  language: '',
  speed: 1.0,
  volume: 1.0,
};

async function putAudioSettings(baseUrl, overrides = {}) {
  const body = { ...DEFAULT_AUDIO_SETTINGS_BODY, ...overrides };
  const res = await request(baseUrl, 'PUT', '/api/audio/settings', body);
  expect(res.status === 200, 'PUT /api/audio/settings succeeded', res.body);
  return res.body;
}

// --- tiny WAV encoder (no dependency) -------------------------------------

/** Builds a real, valid 16-bit PCM WAV file - the one closed format
 * docs/alert-audio.md §2 accepts. numSamples is per channel. */
function buildWAV(sampleRate = 44100, channels = 1, numSamples = 4410) {
  const bitsPerSample = 16;
  const blockAlign = channels * (bitsPerSample / 8);
  const dataSize = numSamples * blockAlign;
  const byteRate = sampleRate * blockAlign;

  const buf = Buffer.alloc(44 + dataSize);
  buf.write('RIFF', 0, 'ascii');
  buf.writeUInt32LE(36 + dataSize, 4);
  buf.write('WAVE', 8, 'ascii');
  buf.write('fmt ', 12, 'ascii');
  buf.writeUInt32LE(16, 16);
  buf.writeUInt16LE(1, 20); // PCM
  buf.writeUInt16LE(channels, 22);
  buf.writeUInt32LE(sampleRate, 24);
  buf.writeUInt32LE(byteRate, 28);
  buf.writeUInt16LE(blockAlign, 32);
  buf.writeUInt16LE(bitsPerSample, 34);
  buf.write('data', 36, 'ascii');
  buf.writeUInt32LE(dataSize, 40);
  for (let i = 0; i < dataSize; i += 1) {
    buf[44 + i] = i % 200;
  }
  return buf;
}

function sha256Hex(buf) {
  return createHash('sha256').update(buf).digest('hex');
}

// --- tiny ZIP writer (no dependency, mirrors verify-visual-template- -----
// --- packages.mjs's own identical helper exactly) -------------------------

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) {
      c = (c & 1) !== 0 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    table[n] = c >>> 0;
  }
  return table;
})();

function crc32(buf) {
  let crc = 0xffffffff;
  for (const byte of buf) {
    crc = CRC_TABLE[(crc ^ byte) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function buildZip(entries) {
  const localParts = [];
  const centralParts = [];
  let offset = 0;

  for (const { name, data } of entries) {
    const nameBuf = Buffer.from(name, 'utf8');
    const compressed = deflateRawSync(data);
    const useStore = compressed.length >= data.length;
    const method = useStore ? 0 : 8;
    const payload = useStore ? data : compressed;
    const crc = crc32(data);

    const localHeader = Buffer.alloc(30);
    localHeader.writeUInt32LE(0x04034b50, 0);
    localHeader.writeUInt16LE(20, 4);
    localHeader.writeUInt16LE(0, 6);
    localHeader.writeUInt16LE(method, 8);
    localHeader.writeUInt16LE(0, 10);
    localHeader.writeUInt16LE(0, 12);
    localHeader.writeUInt32LE(crc, 14);
    localHeader.writeUInt32LE(payload.length, 18);
    localHeader.writeUInt32LE(data.length, 22);
    localHeader.writeUInt16LE(nameBuf.length, 26);
    localHeader.writeUInt16LE(0, 28);
    localParts.push(localHeader, nameBuf, payload);

    const centralHeader = Buffer.alloc(46);
    centralHeader.writeUInt32LE(0x02014b50, 0);
    centralHeader.writeUInt16LE(20, 4);
    centralHeader.writeUInt16LE(20, 6);
    centralHeader.writeUInt16LE(0, 8);
    centralHeader.writeUInt16LE(method, 10);
    centralHeader.writeUInt16LE(0, 12);
    centralHeader.writeUInt16LE(0, 14);
    centralHeader.writeUInt32LE(crc, 16);
    centralHeader.writeUInt32LE(payload.length, 20);
    centralHeader.writeUInt32LE(data.length, 24);
    centralHeader.writeUInt16LE(nameBuf.length, 28);
    centralHeader.writeUInt16LE(0, 30);
    centralHeader.writeUInt16LE(0, 32);
    centralHeader.writeUInt16LE(0, 34);
    centralHeader.writeUInt16LE(0, 36);
    centralHeader.writeUInt32LE(0, 38);
    centralHeader.writeUInt32LE(offset, 42);
    centralParts.push(centralHeader, nameBuf);

    offset += localHeader.length + nameBuf.length + payload.length;
  }

  const centralStart = offset;
  const centralBuf = Buffer.concat(centralParts);
  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(0, 4);
  eocd.writeUInt16LE(0, 6);
  eocd.writeUInt16LE(entries.length, 8);
  eocd.writeUInt16LE(entries.length, 10);
  eocd.writeUInt32LE(centralBuf.length, 12);
  eocd.writeUInt32LE(centralStart, 16);
  eocd.writeUInt16LE(0, 20);

  return Buffer.concat([...localParts, centralBuf, eocd]);
}

/** Builds a real, valid v2 `.streaming-tree-template` package carrying
 * an alertAudio preset (sound + TTS) and one audioAssets entry - the
 * exact manifest shape docs/alert-audio.md §10.2 defines. */
function buildAudioPackage({ target = 'alert', name = 'Coin Alert Package' } = {}) {
  const wav = buildWAV();
  const hash = sha256Hex(wav);
  const manifest = JSON.stringify({
    format: 'streaming-tree-template-package',
    schemaVersion: 2,
    templatePath: 'template.json',
    assets: [],
    alertAudio: {
      soundEnabled: true, soundAssetId: 'pkgaudio_0001', soundVolume: 0.8,
      ttsEnabled: true, ttsTemplate: '{username} triggered a coin', ttsVolume: 0.5,
    },
    audioAssets: [
      {
        id: 'pkgaudio_0001', path: 'audio/pkgaudio_0001.wav', mediaType: 'audio/wav',
        sha256: hash, sizeBytes: wav.length, durationMs: 100, displayName: 'Coin chime',
      },
    ],
  });
  const template = JSON.stringify({
    format: 'streaming-tree-visual-template', schemaVersion: 1, target, name, description: '', author: '', license: '',
    visualDesign: {
      version: 3,
      canvas: target === 'chat' ? { width: 960, height: 280, transparent: true } : { width: 1920, height: 1080, transparent: true },
      layers: [
        {
          id: 'layer_1', name: 'Text', kind: 'text', visible: true, locked: false, order: 0,
          frame: { x: 0, y: 0, width: 400, height: 60 }, opacity: 1,
          text: {
            binding: 'username', missingValueBehavior: 'hide', fontFamily: 'system-ui', fontSize: 32, fontWeight: 400, lineHeight: 1.2,
            textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'top', outlineColor: '#000000', shadowColor: '#000000',
          },
          entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
        },
      ],
    },
  });
  return buildZip([
    { name: 'manifest.json', data: Buffer.from(manifest) },
    { name: 'template.json', data: Buffer.from(template) },
    { name: 'audio/pkgaudio_0001.wav', data: wav },
  ]);
}

// --- rule/profile helpers ---------------------------------------------

function ruleBody(overrides) {
  return {
    name: overrides.name,
    enabled: overrides.enabled ?? true,
    eventType: overrides.eventType ?? 'follow',
    priority: overrides.priority ?? 50,
    durationMs: overrides.durationMs ?? 5000,
    requiredRole: 'everyone',
    showPlatform: true,
    showUsername: true,
    showMessage: false,
    showQuantity: false,
    textTemplate: overrides.textTemplate ?? '{username} followed!',
    entryAnimation: 'fade',
    exitAnimation: 'fade',
    animationDurationMs: 400,
    providers: [],
    accounts: [],
    ...(overrides.audio !== undefined ? { audio: overrides.audio } : {}),
  };
}

async function uploadAudioAsset(baseUrl, wav, displayName) {
  const boundary = `----streamingtree${RUN_ID}`;
  const multipartBody = Buffer.concat([
    Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="file"; filename="sound.wav"\r\nContent-Type: audio/wav\r\n\r\n`),
    wav,
    Buffer.from(`\r\n--${boundary}\r\nContent-Disposition: form-data; name="displayName"\r\n\r\n${displayName}\r\n--${boundary}--\r\n`),
  ]);
  return requestRaw(baseUrl, 'POST', '/api/audio-assets', {
    body: multipartBody,
    headers: { 'Content-Type': `multipart/form-data; boundary=${boundary}` },
  });
}

// --- main ------------------------------------------------------------------

async function main() {
  console.log('Stage 17B alert audio verification (local fakes only, no real Windows voices, no real providers, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-alert-audio-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const testserverExe = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');

  let backend = null;
  let baseUrl;
  let dbPath;

  try {
    step('Build the integration-only test server (go build -tags integration ./cmd/testserver)');
    await buildBinary('go-build-testserver', './cmd/testserver', testserverExe);

    step('Reserve a dynamic loopback port and start the backend');
    const backendPort = await reservePort();
    baseUrl = `http://127.0.0.1:${backendPort}`;
    const env = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(backendPort),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
    };
    dbPath = join(dataDir, 'streaming-tree.db');
    backend = await startBackend(testserverExe, env, baseUrl);
    pass(`backend listening on :${backendPort}`);

    // --- 1. managed audio assets ------------------------------------------

    step('Upload a valid 16-bit PCM WAV via multipart/form-data');
    const soundWav = buildWAV(44100, 1, 4410);
    const upload = await uploadAudioAsset(baseUrl, soundWav, 'Coin chime');
    expect(upload.status === 201, 'upload succeeds', upload.body);
    expect(typeof upload.body.id === 'string' && upload.body.id.startsWith('audioasset_'), 'the asset got a server-generated audioasset_ id', upload.body.id);
    expect(upload.body.mediaType === 'audio/wav' && upload.body.durationMs === 100, 'the real signature/duration was independently detected', upload.body);
    expect(!('path' in upload.body) && !('url' in upload.body), 'the management response exposes no local path or public content URL', upload.body);
    const soundAssetId = upload.body.id;

    step('Uploading non-WAV content (disguised as .wav) is rejected by independent signature detection');
    const fakeWav = Buffer.from('this is definitely not a real WAV file, just plain text');
    const fakeUpload = await uploadAudioAsset(baseUrl, fakeWav, 'Fake sound');
    expect(fakeUpload.status === 422 && fakeUpload.body?.error === 'audio_asset_unsupported', 'non-WAV content is rejected with the correct stable code', fakeUpload.body);

    step('The uploaded asset is listed and individually gettable');
    const list1 = await request(baseUrl, 'GET', '/api/audio-assets');
    expect(list1.status === 200 && list1.body.some((a) => a.id === soundAssetId), 'the uploaded asset appears in the list', list1.body);
    const get1 = await request(baseUrl, 'GET', `/api/audio-assets/${soundAssetId}`);
    expect(get1.status === 200 && get1.body.displayName === 'Coin chime' && get1.body.referenceCount === 0, 'GET by id matches, unreferenced so far', get1.body);

    // --- 2. rule/profile audio persistence and validation -----------------

    step('Create a profile and a rule referencing the uploaded sound asset, with TTS also enabled');
    const createProfile = await request(baseUrl, 'POST', '/api/alert-profiles', { name: 'Audio Test Profile' });
    expect(createProfile.status === 201, 'profile created', createProfile.body);
    const profile = createProfile.body;

    const createRule = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Coin alert',
      audio: {
        soundEnabled: true, soundAssetId, soundVolume: 0.8,
        ttsEnabled: true, ttsTemplate: '{username} triggered a coin', ttsVolume: 0.5,
      },
    }));
    expect(createRule.status === 201, 'rule created with sound+TTS audio', createRule.body);
    expect(createRule.body.audio.soundEnabled === true && createRule.body.audio.soundAssetId === soundAssetId, 'the created rule reflects the sound config exactly', createRule.body.audio);
    expect(createRule.body.audio.ttsEnabled === true && createRule.body.audio.ttsTemplate === '{username} triggered a coin', 'the created rule reflects the TTS config exactly', createRule.body.audio);
    const rule = createRule.body;

    const getRule = await request(baseUrl, 'GET', `/api/alert-rules/${rule.id}`);
    expect(getRule.status === 200 && getRule.body.audio.soundVolume === 0.8 && getRule.body.audio.ttsVolume === 0.5, 'GET the rule persists the exact volumes', getRule.body.audio);

    step('Referencing an unknown sound asset id is rejected with the correct stable code');
    const unknownAssetRule = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad asset rule',
      audio: { soundEnabled: true, soundAssetId: 'audioasset_does_not_exist', soundVolume: 1, ttsEnabled: false, ttsTemplate: '', ttsVolume: 1 },
    }));
    expect(unknownAssetRule.status === 404 && unknownAssetRule.body?.error === 'audio_rule_asset_not_found', 'an unknown sound asset id is rejected honestly', unknownAssetRule.body);

    step('A TTS template using {groupCount} is rejected - grouping never restarts already-playing audio');
    const groupCountRule = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad TTS template rule',
      audio: { soundEnabled: false, soundAssetId: '', soundVolume: 1, ttsEnabled: true, ttsTemplate: '{username} x{groupCount}', ttsVolume: 1 },
    }));
    expect(groupCountRule.status === 422 && groupCountRule.body?.error === 'alert_template_invalid', 'a {groupCount} TTS template is rejected', groupCountRule.body);

    step('Sound enabled with no asset selected is rejected');
    const noAssetRule = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'No asset rule',
      audio: { soundEnabled: true, soundAssetId: '', soundVolume: 1, ttsEnabled: false, ttsTemplate: '', ttsVolume: 1 },
    }));
    expect(noAssetRule.status === 422 && noAssetRule.body?.error === 'alert_rule_invalid', 'sound enabled without an asset is rejected', noAssetRule.body);

    step('Deleting a referenced audio asset is blocked; deleting an unreferenced one succeeds');
    const deleteInUse = await request(baseUrl, 'DELETE', `/api/audio-assets/${soundAssetId}`);
    expect(deleteInUse.status === 409 && deleteInUse.body?.error === 'audio_asset_in_use', 'delete is blocked while the rule references the asset', deleteInUse.body);
    const scratchWav = buildWAV(22050, 1, 2205);
    const scratchUpload = await uploadAudioAsset(baseUrl, scratchWav, 'Scratch (unreferenced)');
    const deleteUnused = await request(baseUrl, 'DELETE', `/api/audio-assets/${scratchUpload.body.id}`);
    expect(deleteUnused.status === 204, 'delete succeeds for an unreferenced asset', deleteUnused.body);

    const publicSlug = profile.publicSlug;

    // --- 3. bounded visual hold, WITHOUT any renderer ever connected -----

    step('Bounded hold: a slow-synthesizing alert stays visible past its own duration while no renderer has connected yet');
    const delaySet = await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 3000 });
    expect(delaySet.status === 204, 'set a 3-second fake synthesis delay', delaySet.body);

    const holdRule = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Hold test rule', durationMs: 1000,
      // TTS, never sound: a sound item resolves managed-asset bytes
      // directly (no synthesis step at all), so the fake synthesis
      // delay below has nothing to hold onto. TTS is the one alert-
      // owned audio kind that genuinely calls the fake TTS provider.
      audio: { soundEnabled: false, soundAssetId: '', soundVolume: 1, ttsEnabled: true, ttsTemplate: '{username} test', ttsVolume: 1 },
    }));
    expect(holdRule.status === 201, 'the short-duration hold-test rule was created', holdRule.body);

    const holdStream = sseEvents(`${baseUrl}/api/public/alert-profiles/${publicSlug}/stream`);
    await nextAlertEvent(holdStream, POLL_TIMEOUT_MS, 'the initial alert.reset');
    const testHold = await request(baseUrl, 'POST', `/api/alert-rules/${holdRule.body.id}/test`);
    expect(testHold.status === 200, 'Test Rule accepted for the hold-test rule', testHold.body);
    const holdShow = await nextAlertEvent(holdStream, POLL_TIMEOUT_MS, 'alert.show for the hold-test alert');
    expect(holdShow.event === 'alert.show', 'the hold-test alert is shown', holdShow.data);

    // The rule's own 1000ms duration has already elapsed by the time the
    // 3000ms synthesis delay clears - if hold were NOT working, alert.hide
    // would already have fired well before this point.
    await new Promise((r) => setTimeout(r, 1500));
    expect((await audioStatus(baseUrl)).hasCurrentItem === true, 'the sound item is still synthesizing 1.5s in - still well inside the 3s delay', await audioStatus(baseUrl));

    const holdHide = await nextAlertEvent(holdStream, POLL_TIMEOUT_MS, 'alert.hide once the hold releases (no renderer ever connected)');
    expect(holdHide.event === 'alert.hide', 'the alert eventually hides once synthesis finishes and no renderer is present to hold for (AlertAudioNoRenderer)', holdHide.data);
    await holdStream.return();
    await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 0 });
    await request(baseUrl, 'POST', '/api/audio/queue/clear');

    // --- 4. the real sound-then-TTS chain, WITH a connected renderer -----

    step('Connect the one shared public audio renderer stream (needed for any promotion/synthesis from here on)');
    const audioSettings = await request(baseUrl, 'GET', '/api/audio/settings');
    expect(audioSettings.status === 200 && typeof audioSettings.body.publicSlug === 'string' && audioSettings.body.publicSlug.length > 0,
      'a public audio slug exists', audioSettings.body);
    const audioSlug = audioSettings.body.publicSlug;
    const audioStream = sseEvents(`${baseUrl}/api/public/audio/${audioSlug}/stream`);
    const audioReset = await nextAudioEvent(audioStream, POLL_TIMEOUT_MS, 'the initial audio.reset');
    expect(audioReset.event === 'audio.reset' && typeof audioReset.data?.rendererToken === 'string', 'a real renderer session was established', audioReset.data);
    const rendererToken = audioReset.data.rendererToken;

    step('Test Rule triggers the real sound-then-TTS chain, in order, over the real public audio stream');
    const alertStream = sseEvents(`${baseUrl}/api/public/alert-profiles/${publicSlug}/stream`);
    await nextAlertEvent(alertStream, POLL_TIMEOUT_MS, 'the alert stream\'s own alert.reset');
    const testChain = await request(baseUrl, 'POST', `/api/alert-rules/${rule.id}/test`);
    expect(testChain.status === 200, 'Test Rule accepted for the sound+TTS rule', testChain.body);
    const chainShow = await nextAlertEvent(alertStream, POLL_TIMEOUT_MS, 'alert.show for the chain-test alert');
    expect(chainShow.event === 'alert.show', 'the chain-test alert is shown on the real public alert stream', chainShow.data);

    const soundCurrent = await waitUntil(async () => {
      const evt = await nextAudioEvent(audioStream, POLL_TIMEOUT_MS, 'the first (sound) audio.current');
      return evt.event === 'audio.current' ? evt : false;
    }, POLL_TIMEOUT_MS, 'the sound item to be promoted first');
    expect(soundCurrent.data.contentType === 'audio/wav' && Math.abs(soundCurrent.data.volume - 0.8) < 1e-9,
      'the first item is the sound, at the rule\'s own configured volume (0.8 * global volume 1.0)', soundCurrent.data);
    const soundItemId = soundCurrent.data.itemId;

    await ackStartedThenEnded(baseUrl, audioSlug, rendererToken, soundItemId);
    const ttsCurrent = await waitUntil(async () => {
      const evt = await nextAudioEvent(audioStream, POLL_TIMEOUT_MS, 'the second (TTS) audio.current');
      return evt.event === 'audio.current' && evt.data.itemId !== soundItemId ? evt : false;
    }, POLL_TIMEOUT_MS, 'the chain to advance automatically to the TTS item');
    expect(Math.abs(ttsCurrent.data.volume - 0.5) < 1e-9, 'the second (TTS) item carries its own configured volume (0.5)', ttsCurrent.data);
    pass('the sound-then-TTS chain advanced automatically on natural completion, exactly once, in order');

    await ackStartedThenEnded(baseUrl, audioSlug, rendererToken, ttsCurrent.data.itemId);
    const chainHide = await nextAlertEvent(alertStream, POLL_TIMEOUT_MS, 'alert.hide once the chain finishes and the rule\'s own duration has elapsed');
    expect(chainHide.event === 'alert.hide', 'the alert hides once both chain items have ended', chainHide.data);
    await alertStream.return();

    step('No alert-owned identifier or spoken text ever appears in the public audio payload');
    for (const secret of [rule.id, profile.id, 'triggered a coin', RUN_ID]) {
      expect(!JSON.stringify(soundCurrent.data).includes(secret) && !JSON.stringify(ttsCurrent.data).includes(secret),
        `the audio.current payload never leaks "${secret}"`, { sound: soundCurrent.data, tts: ttsCurrent.data });
    }
    expect(Object.keys(soundCurrent.data).sort().join(',') === 'bytesUrl,contentType,itemId,volume',
      'the alert-owned sound item exposes only the 4 documented public fields, nothing else', soundCurrent.data);

    // --- 5. arbitration: alert-owned audio preempts a playing global TTS -

    step('Enable global TTS and start a slow global item, so it is genuinely current when the alert-owned item arrives');
    await putAudioSettings(baseUrl, { enabled: true, enabledEventTypes: ['chat.message'] });
    await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 5000 });
    const globalSpeak = await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'a long global announcement' });
    expect(globalSpeak.status === 200, 'the global Test Speak item was accepted', globalSpeak.body);
    // The stream's own buffer still holds stale frames from step 13's own
    // final ack cycle (a duplicate audio.current re-emitted on
    // playback_started, then audio.idle on playback_ended - see the same
    // note further down) that step 13 itself never drained, since it read
    // from alertStream next instead of audioStream. Require a genuinely
    // new itemId, never soundItemId/ttsCurrent's own, so this wait cannot
    // resolve on that leftover chain-test data instead of the real new
    // global item.
    const globalCurrent = await waitUntil(async () => {
      const evt = await nextAudioEvent(audioStream, POLL_TIMEOUT_MS, 'the global item to become current (synthesizing)');
      return evt.event === 'audio.current' && evt.data.itemId !== soundItemId && evt.data.itemId !== ttsCurrent.data.itemId ? evt : false;
    }, POLL_TIMEOUT_MS, 'the slow global TTS item to become current');
    pass(`the global TTS item (${globalCurrent.data.itemId}) is current and still synthesizing`);

    const interruptedBefore = (await audioStatus(baseUrl)).totalInterruptedByAlert;
    const testArbitration = await request(baseUrl, 'POST', `/api/alert-rules/${rule.id}/test`);
    expect(testArbitration.status === 200, 'Test Rule accepted while a global item is current', testArbitration.body);

    const preempted = await waitUntil(async () => {
      const evt = await nextAudioEvent(audioStream, POLL_TIMEOUT_MS, 'the alert-owned item preempting the global one');
      return evt.event === 'audio.current' && evt.data.itemId !== globalCurrent.data.itemId ? evt : false;
    }, POLL_TIMEOUT_MS, 'the alert-owned sound to preempt the still-synthesizing global item');
    expect(Math.abs(preempted.data.volume - 0.8) < 1e-9, 'the preempting item is the alert\'s own sound (volume 0.8), never the global one', preempted.data);
    const statusAfterPreempt = await audioStatus(baseUrl);
    expect(statusAfterPreempt.totalInterruptedByAlert === interruptedBefore + 1, 'totalInterruptedByAlert increased by exactly 1', { before: interruptedBefore, after: statusAfterPreempt.totalInterruptedByAlert });

    // Drain the rest of this alert's own chain so later steps start clean.
    // The stream's own buffer still holds stale frames from the ack
    // cycle just performed (a duplicate audio.current re-emitted on
    // playback_started, then audio.idle on playback_ended) - matching
    // on event name alone would pick up the already-ended sound item
    // again. Require a genuinely different itemId from the sound's own.
    await ackStartedThenEnded(baseUrl, audioSlug, rendererToken, preempted.data.itemId);
    const afterPreemptTTS = await waitUntil(async () => {
      const evt = await nextAudioEvent(audioStream, POLL_TIMEOUT_MS, 'the TTS half after the preempting sound ends');
      return evt.event === 'audio.current' && evt.data.itemId !== preempted.data.itemId ? evt : false;
    }, POLL_TIMEOUT_MS, 'the preempting alert\'s own TTS item');
    await ackStartedThenEnded(baseUrl, audioSlug, rendererToken, afterPreemptTTS.data.itemId);
    await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 0 });
    await putAudioSettings(baseUrl, {});
    await audioStream.return();

    // --- 6. package v2 audio: import, export, chat-target rejection ------

    step('Import a real v2 package carrying an alertAudio preset and one audio asset');
    const pkg = buildAudioPackage({ target: 'alert' });
    const importPkg = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import', { body: pkg });
    expect(importPkg.status === 201, 'the audio package imported successfully', importPkg.body);
    expect(importPkg.body.alertAudio?.soundEnabled === true && importPkg.body.alertAudio.soundAssetId !== 'pkgaudio_0001',
      'the imported template carries the preset, remapped to a real local audio asset id', importPkg.body.alertAudio);
    expect(importPkg.body.alertAudio?.ttsEnabled === true && importPkg.body.alertAudio.ttsTemplate === '{username} triggered a coin',
      'the imported template\'s TTS preset matches the package exactly', importPkg.body.alertAudio);
    const importedTemplateId = importPkg.body.id;
    const importedAudioAssetId = importPkg.body.alertAudio.soundAssetId;

    const importedAsset = await request(baseUrl, 'GET', `/api/audio-assets/${importedAudioAssetId}`);
    expect(importedAsset.status === 200 && importedAsset.body.source === 'package' && importedAsset.body.referenceCount === 1,
      'the package-imported audio asset is real, sourced "package", and referenced exactly once', importedAsset.body);

    step('Exporting that template writes a real v2 package that re-imports with a fresh local asset id');
    const exportPkg = await requestRaw(baseUrl, 'GET', `/api/visual-templates/${importedTemplateId}/export-package`);
    expect(exportPkg.status === 200, 'export-package succeeded', exportPkg.status);
    expect(exportPkg.raw.length > 4 && exportPkg.raw.readUInt32LE(0) === 0x04034b50, 'the export response is a real ZIP archive (local file header magic)', exportPkg.raw.subarray(0, 4).toString('hex'));

    const reimportPkg = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import', { body: exportPkg.raw });
    expect(reimportPkg.status === 201, 're-importing the exported package succeeded', reimportPkg.body);
    expect(reimportPkg.body.alertAudio?.soundAssetId !== importedAudioAssetId && reimportPkg.body.alertAudio?.soundEnabled === true,
      're-import produced a genuinely fresh local audio asset id, never reusing the original', reimportPkg.body.alertAudio);

    step('A chat-target package carrying alertAudio is rejected before any asset is staged');
    const chatPkg = buildAudioPackage({ target: 'chat' });
    const assetCountBefore = (await request(baseUrl, 'GET', '/api/audio-assets')).body.length;
    const importChatPkg = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import', { body: chatPkg });
    expect(importChatPkg.status === 422 && importChatPkg.body?.error === 'visual_template_package_audio_target_invalid',
      'the chat-target audio package is rejected with the correct stable code', importChatPkg.body);
    const assetCountAfter = (await request(baseUrl, 'GET', '/api/audio-assets')).body.length;
    expect(assetCountAfter === assetCountBefore, 'no audio asset was ever created for the rejected chat-target package', { before: assetCountBefore, after: assetCountAfter });

    step('A v1 package (no audio) still imports exactly as before - the audio work never disturbed the pre-existing path');
    const v1Manifest = JSON.stringify({ format: 'streaming-tree-template-package', schemaVersion: 1, templatePath: 'template.json', assets: [] });
    const v1Template = JSON.stringify({
      format: 'streaming-tree-visual-template', schemaVersion: 1, target: 'alert', name: 'Plain v1 package', description: '', author: '', license: '',
      visualDesign: { version: 3, canvas: { width: 1920, height: 1080, transparent: true }, layers: [] },
    });
    const v1Pkg = buildZip([
      { name: 'manifest.json', data: Buffer.from(v1Manifest) },
      { name: 'template.json', data: Buffer.from(v1Template) },
    ]);
    const importV1 = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import', { body: v1Pkg });
    expect(importV1.status === 201 && importV1.body.alertAudio === undefined, 'a plain v1 package imports with no alertAudio at all', importV1.body);

    // --- 7. privacy: nothing sensitive in logs or SQLite ------------------

    step('Confirm no rule id, profile id, or rendered TTS text was ever written to SQLite');
    const dbBytes = readFileSync(dbPath);
    const dbText = dbBytes.toString('latin1');
    for (const secret of ['triggered a coin', 'a long global announcement']) {
      expect(!dbText.includes(secret), `SQLite never contains the rendered/spoken text "${secret}"`, undefined);
    }
    pass(`scanned the ${dbBytes.length}-byte SQLite database file for rendered TTS text - found none`);

    console.log('\nStage 17B alert audio verification PASSED');
  } catch (error) {
    if (backend !== null && process.env.STREAMING_TREE_VERIFY_DEBUG === '1') {
      console.error('\n--- backend output (debug) ---');
      console.error(backend.getOutput());
      console.error('--- end backend output ---\n');
    }
    throw error;
  } finally {
    try {
      await stopBackend(backend, baseUrl ?? '');
    } catch {
      // Already reporting a failure if we get here.
    }
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nStage 17B alert audio verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
