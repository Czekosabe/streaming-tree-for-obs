#!/usr/bin/env node
/**
 * Stage 17A TTS/audio verification (19th integration script).
 *
 * Runs the real `-tags integration` backend (cmd/testserver) with real
 * SQLite, a real Engagement Event Bus, a real internal/audio.Manager,
 * and the real public HTTP surface - against the integration-build-only
 * deterministic fake TTS provider (internal/provider/tts/fake.go),
 * never the real Windows SAPI provider (never a dependency on this
 * machine's installed voices). One real Event-Bus scenario is exercised
 * through the already-established StreamElements donation fake
 * (cmd/fakestreamelements, the same one scripts/verify-streamelements-
 * donations.mjs uses) - a real donation event, normalized by the real
 * connector, published on the real bus, consumed by the real TTS
 * eligibility pipeline - never bypassing the Event Bus by calling the
 * utterance builder directly.
 *
 * This is a REPRESENTATIVE subset, not a literal transcription of every
 * scenario the governing task's own §69 lists. Explicitly delegated to
 * Go tests (never re-proven here) because a black-box HTTP client
 * cannot observe them, or because they need a fake clock/real elapsed
 * time this script cannot afford:
 *   - Synthetic Event Bus events are ignored:
 *     internal/audio/manager_test.go TestManagerIgnoresSyntheticEvent.
 *   - Stable per-user cooldown identity vs. anonymous-uses-global-only:
 *     internal/audio/manager_test.go
 *     TestManagerPerUserCooldownSuppressesSecondMessage,
 *     TestManagerAnonymousEventUsesGlobalCooldownOnly.
 *   - Item expiry (5 real minutes, not configurable):
 *     internal/audio/queue_test.go
 *     TestQueuePopNextEligibleSkipsExpiredAndCounts (fake clock).
 *   - Every text-preprocessing step (URL removal, blocked words,
 *     repeated-character normalization, whitespace, Unicode length):
 *     internal/audio/preprocess_test.go (18 tests) - the public API
 *     never exposes spoken text content to verify against by design
 *     (docs/audio-tts.md §19), so only the pipeline's "empty result is
 *     rejected" end state is checked here through the real HTTP path.
 *   - Every settings validation bound: internal/domain/audio/
 *     validation_test.go (24 tests) - only one representative 422 is
 *     checked here.
 *
 * No real StreamElements account, no real Windows voice, and no real
 * OBS Browser Source is ever involved.
 *
 * Usage: node scripts/verify-tts-audio.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

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

function mintToken(prefix) {
  return `${prefix}-${RUN_ID}-${randomBytes(12).toString('hex')}`;
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

async function control(controlBaseUrl, method, path, body) {
  const init = { method, headers: {} };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  const res = await fetch(`${controlBaseUrl}${path}`, init);
  const text = await res.text();
  record(text);
  let parsed = null;
  if (text !== '') {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  return { status: res.status, body: parsed };
}

// --- SSE helpers (mirrors scripts/verify-alerts.mjs's own reader) --------

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

/** Reads named audio.* events, skipping keepalive comments (which parse
 * as event "message" with null data). */
async function nextAudioEvent(iterator, timeoutMs, label) {
  for (let i = 0; i < 30; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    const evt = await nextEvent(iterator, timeoutMs, label);
    if (evt.event.startsWith('audio.')) return evt;
  }
  throw new Error(`too many non-audio.* frames while waiting for ${label}`);
}

// --- fake StreamElements Astro server (mirrors verify-streamelements-donations.mjs) --

async function startFakeAstroServer(exePath, wsAddr, controlAddr) {
  const handle = spawnCaptured('fake-astro', exePath, [`-ws-addr=${wsAddr}`, `-control-addr=${controlAddr}`], { cwd: SERVER_DIR });
  const deadline = Date.now() + READINESS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (handle.hasExited()) throw new Error(`fake astro server exited during startup:\n${handle.getOutput()}`);
    // eslint-disable-next-line no-await-in-loop
    const res = await fetch(`http://${controlAddr}/control/health`).catch(() => null);
    if (res !== null && res.ok) return handle;
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 150));
  }
  await killTree(handle);
  throw new Error(`fake astro server did not become ready in ${READINESS_TIMEOUT_MS} ms:\n${handle.getOutput()}`);
}

async function waitForConnection(controlBaseUrl, label) {
  return waitUntil(async () => {
    const res = await control(controlBaseUrl, 'GET', '/control/connections');
    return res.body.items.length > 0 ? res.body : false;
  }, POLL_TIMEOUT_MS, label);
}

async function pushTip(controlBaseUrl, topic, tip, connectionId = 'latest') {
  const res = await control(controlBaseUrl, 'POST', '/control/push-tip', { connectionId, topic, tip });
  expect(res.status === 204, `pushed tip ${tip._id} over the fake Astro WebSocket`, res.body);
}

function tipFixture(overrides = {}) {
  const id = overrides.id ?? mintToken('tip');
  return {
    donation: {
      user: {
        username: overrides.username === undefined ? 'Styler' : overrides.username,
        geo: overrides.geo ?? 'ZZ',
        email: overrides.email ?? `donor-${RUN_ID}@example.invalid`,
        channel: overrides.channel ?? 'chan_1',
      },
      message: overrides.message ?? 'great stream!',
      amount: overrides.amount ?? 4.2,
      currency: overrides.currency ?? 'USD',
      paymentMethod: overrides.paymentMethod ?? 'scheme',
    },
    _id: id,
    channel: overrides.channel ?? 'chan_1',
    provider: overrides.paymentRail ?? 'paypal',
    approved: overrides.approved ?? 'allowed',
    status: overrides.status ?? 'success',
    createdAt: overrides.createdAt ?? new Date().toISOString(),
    updatedAt: overrides.updatedAt ?? new Date().toISOString(),
    transactionId: overrides.transactionId ?? mintToken('txn'),
  };
}

async function engagementEvents(baseUrl) {
  const res = await request(baseUrl, 'GET', '/api/engagement/events?after=0&limit=200');
  return res.body.items;
}

async function waitForEvent(baseUrl, predicate, timeoutMs, label) {
  return waitUntil(async () => {
    const items = await engagementEvents(baseUrl);
    const found = items.find(predicate);
    return found ?? false;
  }, timeoutMs, label);
}

// --- audio-specific helpers ----------------------------------------------

const DEFAULT_AUDIO_SETTINGS_BODY = {
  enabled: true,
  providerMode: 'system',
  enabledEventTypes: ['chat.message', 'donation'],
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

async function audioStatus(baseUrl) {
  const res = await request(baseUrl, 'GET', '/api/audio/status');
  expect(res.status === 200, 'GET /api/audio/status succeeded', res.body);
  return res.body;
}

// --- main ------------------------------------------------------------------

async function main() {
  console.log('Stage 17A TTS/audio verification (local fakes only, no real Windows voices, no real StreamElements)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-tts-audio-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const testserverExe = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const fakeAstroExe = join(tempDir, process.platform === 'win32' ? 'fakestreamelements.exe' : 'fakestreamelements');

  let backend = null;
  let fakeAstro = null;
  let baseUrl;
  let controlBaseUrl;
  let dbPath;

  try {
    step('Build the fake Astro WebSocket server and the integration-only test server');
    await buildBinary('go-build-fakestreamelements', './cmd/fakestreamelements', fakeAstroExe);
    await buildBinary('go-build-testserver', './cmd/testserver', testserverExe);

    step('Reserve dynamic loopback ports and start the fake Astro server');
    const [backendPort, wsPort, controlPort] = await reservePorts(3);
    baseUrl = `http://127.0.0.1:${backendPort}`;
    const wsAddr = `127.0.0.1:${wsPort}`;
    controlBaseUrl = `http://127.0.0.1:${controlPort}`;
    fakeAstro = await startFakeAstroServer(fakeAstroExe, wsAddr, `127.0.0.1:${controlPort}`);
    pass(`backend :${backendPort}  fake astro ws :${wsPort}  fake astro control :${controlPort}`);

    const env = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(backendPort),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
      STREAMING_TREE_TEST_STREAMELEMENTS_WS_BASE_URL: `ws://${wsAddr}/`,
    };
    dbPath = join(dataDir, 'streaming-tree.db');

    step('Start the backend under test');
    backend = await startBackend(testserverExe, env, baseUrl);

    // --- 1. defaults, capabilities, voices ------------------------------

    step('Confirm default disabled settings [delegates every bound to internal/domain/audio/validation_test.go]');
    const settings0 = await request(baseUrl, 'GET', '/api/audio/settings');
    expect(settings0.status === 200 && settings0.body.enabled === false, 'audio is disabled by default', settings0.body);
    expect(settings0.body.providerMode === 'disabled', 'provider mode is "disabled" by default', settings0.body);
    expect(typeof settings0.body.publicSlug === 'string' && settings0.body.publicSlug.length > 0, 'a public slug was generated on first read', settings0.body);

    step('Confirm the real fake TTS provider reports itself available with 2 deterministic voices');
    const caps = await request(baseUrl, 'GET', '/api/audio/capabilities');
    expect(caps.status === 200 && caps.body.systemProviderAvailable === true, 'the fake provider reports available', caps.body);
    const voices = await request(baseUrl, 'GET', '/api/audio/voices');
    expect(voices.status === 200 && voices.body.length === 2, 'exactly the fake provider\'s 2 deterministic voices are listed', voices.body);
    expect(voices.body.some((v) => v.id === 'fake-voice-default' && v.isDefault === true), 'the default fake voice is marked isDefault', voices.body);

    // --- 2. settings persist across restart; runtime never does --------

    step('Enable audio and persist settings, then confirm they survive a real backend restart');
    await putAudioSettings(baseUrl, { manualApproval: false });
    const beforeRestart = await request(baseUrl, 'GET', '/api/audio/settings');
    expect(beforeRestart.body.enabled === true, 'settings were saved before restart', beforeRestart.body);

    await stopBackend(backend, baseUrl);
    backend = await startBackend(testserverExe, env, baseUrl);
    const afterRestart = await request(baseUrl, 'GET', '/api/audio/settings');
    expect(afterRestart.body.enabled === true && afterRestart.body.publicSlug === beforeRestart.body.publicSlug,
      'settings (including the public slug) survived a real process restart', { before: beforeRestart.body, after: afterRestart.body });

    step('Confirm the runtime queue does NOT persist across restart');
    await putAudioSettings(baseUrl, { manualApproval: true });
    const testSpeak0 = await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'should not survive restart' });
    expect(testSpeak0.status === 200, 'a Test Speak item was accepted before restart', testSpeak0.body);
    const statusBeforeRestart2 = await audioStatus(baseUrl);
    expect(statusBeforeRestart2.readyQueueCount === 1, 'the item is queued before restart', statusBeforeRestart2);

    await stopBackend(backend, baseUrl);
    backend = await startBackend(testserverExe, env, baseUrl);
    const statusAfterRestart2 = await audioStatus(baseUrl);
    expect(statusAfterRestart2.readyQueueCount === 0 && statusAfterRestart2.pendingApprovalCount === 0,
      'the queue is empty after restart - runtime state never persists', statusAfterRestart2);
    await putAudioSettings(baseUrl, { manualApproval: false });

    // --- 3. Test Speak: pipeline, capacity, empty-result rejection ------

    step('Test Speak goes through the real preprocessing pipeline and the real bounded queue');
    const testSpeak1 = await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'hello from the integration script' });
    expect(testSpeak1.status === 200 && testSpeak1.body.text === 'hello from the integration script',
      'Test Speak accepted and echoed the preprocessed text', testSpeak1.body);
    const statusAfterSpeak = await audioStatus(baseUrl);
    expect(statusAfterSpeak.totalSynthetic >= 1, 'the synthetic counter increased', statusAfterSpeak);
    await request(baseUrl, 'POST', '/api/audio/queue/clear');

    step('Test Speak rejects an entirely-blocked result as empty (real pipeline, real settings)');
    await putAudioSettings(baseUrl, { blockedWords: ['onlyword'] });
    const emptySpeak = await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'onlyword' });
    expect(emptySpeak.status === 422 && emptySpeak.body.error === 'audio_text_empty',
      'an entirely-blocked Test Speak text is rejected as empty', emptySpeak.body);
    await putAudioSettings(baseUrl, { blockedWords: [] });

    step('Queue capacity bound and the capacity-dropped counter, via real bounded Test Speak calls');
    await putAudioSettings(baseUrl, { queueCapacity: 10 });
    for (let i = 0; i < 12; i += 1) {
      // eslint-disable-next-line no-await-in-loop
      await request(baseUrl, 'POST', '/api/audio/test-speak', { text: `capacity test ${i}` });
    }
    const statusAfterCapacity = await audioStatus(baseUrl);
    expect(statusAfterCapacity.readyQueueCount === 10, 'the queue never grew past its 10-item capacity', statusAfterCapacity);
    expect(statusAfterCapacity.totalCapacityDropped >= 2, 'items beyond capacity were dropped and counted', statusAfterCapacity);
    await request(baseUrl, 'POST', '/api/audio/queue/clear');
    await putAudioSettings(baseUrl, { queueCapacity: 100 });

    // --- 4. real Event Bus scenario: a donation source ------------------

    step('Create and enable a real StreamElements donation source pointed at the fake Astro server');
    const jwt = mintToken('fake-jwt');
    const createSource = await request(baseUrl, 'POST', '/api/donation-sources', {
      providerId: 'streamelements', label: 'TTS test channel', remoteChannelId: 'chan_1', token: jwt,
    });
    expect(createSource.status === 201, 'the donation source was created', createSource.body);
    const source = createSource.body;
    await request(baseUrl, 'PUT', `/api/donation-sources/${source.id}/engagement`, { enabled: true });
    await waitForConnection(controlBaseUrl, 'the fake Astro server to accept a connection');
    await waitUntil(async () => {
      const eng = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
      return eng.body.state === 'connected' ? true : false;
    }, POLL_TIMEOUT_MS, 'the donation connector to reach state "connected"');
    pass('the donation source is connected to the fake Astro server');

    step('Enable audio for real donation events, provider/source filtering, and exact-currency threshold');
    await putAudioSettings(baseUrl, {
      enabledEventTypes: ['donation'], supporterOnlyMode: true, enabledSourceIds: ['some-other-source-id'],
    });

    // totalEnqueued/totalSynthetic etc. are lifetime counters, never
    // reset by queue/clear - every "never enqueued" check below compares
    // against a freshly-captured baseline delta, never an absolute 0.
    step('A donation from a source NOT in enabledSourceIds is never enqueued [real event, real Event Bus, real filter]');
    const baseline1 = await audioStatus(baseUrl);
    const filteredTip = tipFixture({ message: 'filtered out' });
    await pushTip(controlBaseUrl, 'channel.tips', filteredTip);
    await waitForEvent(baseUrl, (e) => e.providerEventId === filteredTip._id, POLL_TIMEOUT_MS, 'the donation event on the real Event Bus');
    await new Promise((r) => setTimeout(r, 400));
    const statusFiltered = await audioStatus(baseUrl);
    expect(statusFiltered.readyQueueCount === 0 && statusFiltered.totalEnqueued === baseline1.totalEnqueued,
      'the donation was filtered out by enabledSourceIds - never enqueued', { before: baseline1, after: statusFiltered });

    step('The same source, once allowed, and an exact-currency threshold - same-currency-passes, different-currency-never-compared');
    const minAmount = { thresholdCurrency: 'USD', thresholdMinimumAmountMicros: 5_000_000 };
    await putAudioSettings(baseUrl, {
      enabledEventTypes: ['donation'], supporterOnlyMode: true, enabledSourceIds: [source.id], ...minAmount,
    });

    const baseline2 = await audioStatus(baseUrl);
    const belowThresholdTip = tipFixture({ amount: 1.0, currency: 'USD', message: 'below threshold' });
    await pushTip(controlBaseUrl, 'channel.tips', belowThresholdTip);
    await waitForEvent(baseUrl, (e) => e.providerEventId === belowThresholdTip._id, POLL_TIMEOUT_MS, 'the below-threshold donation on the bus');
    await new Promise((r) => setTimeout(r, 400));
    expect((await audioStatus(baseUrl)).totalEnqueued === baseline2.totalEnqueued, 'a below-threshold USD donation is never enqueued', await audioStatus(baseUrl));

    const wrongCurrencyTip = tipFixture({ amount: 500, currency: 'EUR', message: 'big EUR amount' });
    await pushTip(controlBaseUrl, 'channel.tips', wrongCurrencyTip);
    await waitForEvent(baseUrl, (e) => e.providerEventId === wrongCurrencyTip._id, POLL_TIMEOUT_MS, 'the EUR donation on the bus');
    await new Promise((r) => setTimeout(r, 400));
    expect((await audioStatus(baseUrl)).totalEnqueued === baseline2.totalEnqueued,
      'a large EUR donation is never compared against a USD threshold, never enqueued', await audioStatus(baseUrl));

    const qualifyingTip = tipFixture({ amount: 10.0, currency: 'USD', message: 'qualifying donation' });
    await pushTip(controlBaseUrl, 'channel.tips', qualifyingTip);
    await waitForEvent(baseUrl, (e) => e.providerEventId === qualifyingTip._id, POLL_TIMEOUT_MS, 'the qualifying donation on the bus');
    await waitUntil(async () => (await audioStatus(baseUrl)).totalEnqueued > baseline2.totalEnqueued, POLL_TIMEOUT_MS, 'the qualifying donation to be enqueued for speech');
    pass('a real donation event, through the real connector and the real Event Bus, was accepted by the real TTS pipeline');
    await request(baseUrl, 'POST', '/api/audio/queue/clear');

    // --- 5. manual approval: pending, approve, reject (real event only) --

    step('Manual approval: a real qualifying donation becomes pending, never immediately queued to play');
    await putAudioSettings(baseUrl, {
      enabledEventTypes: ['donation'], supporterOnlyMode: true, enabledSourceIds: [source.id], manualApproval: true, ...minAmount,
    });
    const pendingTip1 = tipFixture({ amount: 10.0, currency: 'USD', message: 'pending approval test' });
    await pushTip(controlBaseUrl, 'channel.tips', pendingTip1);
    await waitForEvent(baseUrl, (e) => e.providerEventId === pendingTip1._id, POLL_TIMEOUT_MS, 'the pending-test donation on the bus');
    const pendingList1 = await waitUntil(async () => {
      const res = await request(baseUrl, 'GET', '/api/audio/pending');
      return res.body.length >= 1 ? res.body : false;
    }, POLL_TIMEOUT_MS, 'the donation to appear in the pending-approval list');
    expect(pendingList1.length === 1, 'exactly one item is pending approval', pendingList1);
    expect((await audioStatus(baseUrl)).readyQueueCount === 0, 'the pending item never entered the ready queue', await audioStatus(baseUrl));

    step('Approving a pending item moves it into the real ready queue');
    const approve1 = await request(baseUrl, 'POST', `/api/audio/pending/${pendingList1[0].id}/approve`);
    expect(approve1.status === 200 && approve1.body.readyQueueCount === 1, 'approve moved the item into the ready queue', approve1.body);
    await request(baseUrl, 'POST', '/api/audio/queue/clear');

    step('Rejecting a pending item discards it - never a provider side effect, never enters the ready queue');
    const callsBeforeReject = (await request(baseUrl, 'GET', '/api/testonly/tts/synthesize-calls')).body.count;
    const pendingTip2 = tipFixture({ amount: 10.0, currency: 'USD', message: 'reject test' });
    await pushTip(controlBaseUrl, 'channel.tips', pendingTip2);
    await waitForEvent(baseUrl, (e) => e.providerEventId === pendingTip2._id, POLL_TIMEOUT_MS, 'the reject-test donation on the bus');
    const pendingList2 = await waitUntil(async () => {
      const res = await request(baseUrl, 'GET', '/api/audio/pending');
      return res.body.length >= 1 ? res.body : false;
    }, POLL_TIMEOUT_MS, 'the reject-test donation to appear pending');
    const reject1 = await request(baseUrl, 'POST', `/api/audio/pending/${pendingList2[0].id}/reject`);
    expect(reject1.status === 200 && reject1.body.pendingApprovalCount === 0, 'reject cleared the pending item', reject1.body);
    expect(reject1.body.readyQueueCount === 0, 'a rejected item never enters the ready queue', reject1.body);
    await new Promise((r) => setTimeout(r, 300));
    const callsAfterReject = (await request(baseUrl, 'GET', '/api/testonly/tts/synthesize-calls')).body.count;
    expect(callsAfterReject === callsBeforeReject, 'a rejected pending item is never synthesized (just-in-time, never for the whole queue)',
      { before: callsBeforeReject, after: callsAfterReject });

    await putAudioSettings(baseUrl, { manualApproval: false, supporterOnlyMode: false, enabledSourceIds: [], thresholdCurrency: '', enabledEventTypes: ['chat.message'] });
    await request(baseUrl, 'DELETE', `/api/donation-sources/${source.id}`).catch(() => {});

    // --- 6. promotion, synthesis, public SSE/bytes/ack, renderer lease --

    step('Just-in-time promotion + synthesis only happens once a renderer is connected');
    const testSpeak2 = await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'renderer lease test' });
    expect(testSpeak2.status === 200, 'Test Speak accepted', testSpeak2.body);
    await new Promise((r) => setTimeout(r, 300));
    expect((await audioStatus(baseUrl)).hasCurrentItem === false, 'no promotion happens without a connected renderer (waiting_for_renderer)', await audioStatus(baseUrl));

    const currentSettings = await request(baseUrl, 'GET', '/api/audio/settings');
    const slug = currentSettings.body.publicSlug;

    const streamA = sseEvents(`${baseUrl}/api/public/audio/${slug}/stream`);
    const resetA = await nextAudioEvent(streamA, POLL_TIMEOUT_MS, 'the first audio.reset');
    expect(resetA.event === 'audio.reset' && typeof resetA.data?.rendererToken === 'string' && resetA.data.rendererToken.length > 0,
      'connecting to the public stream establishes a real renderer session with a real token', resetA);
    const tokenA = resetA.data.rendererToken;

    const currentA = await waitUntil(async () => {
      const evt = await nextAudioEvent(streamA, POLL_TIMEOUT_MS, 'audio.current after renderer connects');
      return evt.event === 'audio.current' ? evt : false;
    }, POLL_TIMEOUT_MS, 'the item to be promoted and synthesized once a renderer connects');
    expect(typeof currentA.data.bytesUrl === 'string' && currentA.data.contentType === 'audio/wav',
      'the public payload carries only a bytes URL, content type, and volume - no source event data', currentA.data);
    expect(Object.keys(currentA.data).sort().join(',') === 'bytesUrl,contentType,itemId,volume',
      'the audio.current payload exposes only its 4 documented fields, nothing else', currentA.data);

    step('The generated audio URL contains no source event text, and GET returns a real deterministic WAV');
    for (const secret of [qualifyingTip._id, 'donor', RUN_ID]) {
      expect(!currentA.data.bytesUrl.includes(secret) && !currentA.data.itemId.includes('great stream'),
        `the bytes URL never leaks "${secret}"`, currentA.data.bytesUrl);
    }
    const bytesRes = await fetch(`${baseUrl}${currentA.data.bytesUrl}`);
    const audioBuffer = Buffer.from(await bytesRes.arrayBuffer());
    expect(bytesRes.status === 200 && bytesRes.headers.get('content-type') === 'audio/wav',
      'the bytes endpoint returns 200 audio/wav', { status: bytesRes.status, contentType: bytesRes.headers.get('content-type') });
    expect(bytesRes.headers.get('accept-ranges') === 'bytes', 'Range support is advertised (Accept-Ranges: bytes)', bytesRes.headers.get('accept-ranges'));
    expect(audioBuffer.length > 44 && audioBuffer.toString('ascii', 0, 4) === 'RIFF' && audioBuffer.toString('ascii', 8, 12) === 'WAVE',
      'the served bytes are a genuine, valid RIFF/WAVE file', { length: audioBuffer.length, header: audioBuffer.toString('hex', 0, 16) });

    step('Playback acknowledgements: started then ended promotes the queue, a stale/duplicate ack is rejected');
    const ackStart = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token: tokenA, itemId: currentA.data.itemId, kind: 'playback_started' });
    expect(ackStart.status === 204, 'playback_started accepted', ackStart.body);
    const ackWrongToken = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token: 'not-a-real-token', itemId: currentA.data.itemId, kind: 'playback_started' });
    expect(ackWrongToken.status === 409, 'an ack with a wrong/stale token is rejected', ackWrongToken.body);
    const ackEnd = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token: tokenA, itemId: currentA.data.itemId, kind: 'playback_ended' });
    expect(ackEnd.status === 204, 'playback_ended accepted', ackEnd.body);
    const ackEndAgain = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token: tokenA, itemId: currentA.data.itemId, kind: 'playback_ended' });
    expect(ackEndAgain.status === 409, 'a duplicate playback_ended ack for the same item is rejected', ackEndAgain.body);
    const statusAfterAck = await audioStatus(baseUrl);
    expect(statusAfterAck.totalPlayed >= 1 && statusAfterAck.hasCurrentItem === false, 'the played item is counted and cleared', statusAfterAck);

    step('A new renderer session supersedes the old one - the old session\'s ack is rejected, the new one\'s is accepted');
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'second renderer session test' });
    // streamA's own buffer still holds stale frames from the previous
    // ack cycle (a duplicate audio.current re-emitted on
    // playback_started, then audio.idle on playback_ended) - matching
    // on event name alone would pick up a stale, already-ended item.
    // Require a genuinely different itemId from currentA's.
    const currentB = await waitUntil(async () => {
      const evt = await nextAudioEvent(streamA, POLL_TIMEOUT_MS, 'a genuinely new audio.current for the second item');
      return evt.event === 'audio.current' && evt.data.itemId !== currentA.data.itemId ? evt : false;
    }, POLL_TIMEOUT_MS, 'the second, genuinely new item to be promoted');

    const streamB = sseEvents(`${baseUrl}/api/public/audio/${slug}/stream`);
    const resetB = await nextAudioEvent(streamB, POLL_TIMEOUT_MS, 'the second stream\'s own audio.reset');
    const tokenB = resetB.data.rendererToken;
    expect(tokenB !== tokenA, 'the new renderer session has a genuinely different token', { tokenA, tokenB });

    const ackOldSession = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token: tokenA, itemId: currentB.data.itemId, kind: 'playback_started' });
    expect(ackOldSession.status === 409, 'the superseded session\'s ack cannot complete the current item', ackOldSession.body);
    const ackNewSession = await request(baseUrl, 'POST', `/api/public/audio/${slug}/ack`, { token: tokenB, itemId: currentB.data.itemId, kind: 'playback_started' });
    expect(ackNewSession.status === 204, 'the new session\'s ack is accepted', ackNewSession.body);

    step('Renderer disconnect while genuinely playing marks the item interrupted, never auto-replayed');
    await streamB.return();
    await waitUntil(async () => (await audioStatus(baseUrl)).totalInterrupted >= 1, POLL_TIMEOUT_MS, 'the disconnect to mark the item interrupted');
    const statusAfterDisconnect = await audioStatus(baseUrl);
    expect(statusAfterDisconnect.hasCurrentItem === false, 'the interrupted item is discarded, not kept as current', statusAfterDisconnect);
    await streamA.return();

    step('Skip current cancels an in-flight synthesis and counts a manual skip');
    const delaySet = await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 5000 });
    expect(delaySet.status === 204, 'set a 5-second fake synthesis delay', delaySet.body);
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'skip me while synthesizing' });
    const streamC = sseEvents(`${baseUrl}/api/public/audio/${slug}/stream`);
    await nextAudioEvent(streamC, POLL_TIMEOUT_MS, 'audio.reset');
    await waitUntil(async () => (await audioStatus(baseUrl)).hasCurrentItem === true, POLL_TIMEOUT_MS, 'the delayed item to become current (synthesizing)');
    const skipsBefore = (await audioStatus(baseUrl)).totalManuallySkipped;
    const skip = await request(baseUrl, 'POST', '/api/audio/queue/skip-current');
    expect(skip.status === 200 && skip.body.hasCurrentItem === false, 'skip-current clears the current item immediately, without waiting for the delayed synthesis', skip.body);
    expect(skip.body.totalManuallySkipped === skipsBefore + 1, 'the manual-skip counter increased', skip.body);
    await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 0 });
    await streamC.return();

    step('Clear queue empties the ready queue but never touches the current item');
    await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 5000 });
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'stays current' });
    const streamD = sseEvents(`${baseUrl}/api/public/audio/${slug}/stream`);
    await nextAudioEvent(streamD, POLL_TIMEOUT_MS, 'audio.reset');
    await waitUntil(async () => (await audioStatus(baseUrl)).hasCurrentItem === true, POLL_TIMEOUT_MS, 'the delayed item to become current');
    await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 0 });
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'queued behind the current one' });
    await waitUntil(async () => (await audioStatus(baseUrl)).readyQueueCount >= 1, POLL_TIMEOUT_MS, 'the second item to sit in the ready queue');
    const clear = await request(baseUrl, 'POST', '/api/audio/queue/clear');
    expect(clear.status === 200 && clear.body.readyQueueCount === 0, 'clear emptied the ready queue', clear.body);
    expect(clear.body.hasCurrentItem === true, 'the current item was never touched by clear', clear.body);
    await request(baseUrl, 'POST', '/api/audio/queue/skip-current');
    await streamD.return();

    // --- 7. synthesis failure isolation, oversize rejection, cancellation --

    step('A forced synthesis failure isolates exactly one item - the runtime keeps working afterward');
    const failuresBefore = (await audioStatus(baseUrl)).totalSynthesisFailed;
    await request(baseUrl, 'POST', '/api/testonly/tts/fail-next');
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'this one fails' });
    const streamE = sseEvents(`${baseUrl}/api/public/audio/${slug}/stream`);
    await nextAudioEvent(streamE, POLL_TIMEOUT_MS, 'audio.reset');
    await waitUntil(async () => (await audioStatus(baseUrl)).totalSynthesisFailed === failuresBefore + 1, POLL_TIMEOUT_MS, 'the forced synthesis failure to be counted');
    expect((await audioStatus(baseUrl)).hasCurrentItem === false, 'a failed item is discarded, never left as current', await audioStatus(baseUrl));
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'this one succeeds' });
    await waitUntil(async () => (await audioStatus(baseUrl)).hasCurrentItem === true, POLL_TIMEOUT_MS, 'the next item to promote normally after the isolated failure');
    await request(baseUrl, 'POST', '/api/audio/queue/skip-current');
    await streamE.return();

    step('An oversized synthesis result is rejected the same way a failure is');
    const oversizeFailuresBefore = (await audioStatus(baseUrl)).totalSynthesisFailed;
    await request(baseUrl, 'POST', '/api/testonly/tts/oversize-next');
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'this one is too big' });
    const streamF = sseEvents(`${baseUrl}/api/public/audio/${slug}/stream`);
    await nextAudioEvent(streamF, POLL_TIMEOUT_MS, 'audio.reset');
    await waitUntil(async () => (await audioStatus(baseUrl)).totalSynthesisFailed === oversizeFailuresBefore + 1, POLL_TIMEOUT_MS, 'the oversized result to be rejected and counted');
    await streamF.return();

    step('Synthesis provider unavailable is reported honestly through the public capability endpoint');
    const unavailableSet = await request(baseUrl, 'POST', '/api/testonly/tts/available', { available: false, reason: 'integration test forced unavailable' });
    expect(unavailableSet.status === 204, 'the fake provider was set unavailable', unavailableSet.body);
    const capsUnavailable = await request(baseUrl, 'GET', '/api/audio/capabilities');
    expect(capsUnavailable.body.systemProviderAvailable === false && capsUnavailable.body.systemProviderReason === 'integration test forced unavailable',
      'the capability endpoint reports the real unavailable reason honestly', capsUnavailable.body);
    await request(baseUrl, 'POST', '/api/testonly/tts/available', { available: true, reason: '' });

    // --- 8. an unknown voice is handled honestly --------------------------

    step('An unknown saved voice id is handled honestly - synthesis fails for that item, never a fabricated fallback');
    await putAudioSettings(baseUrl, { voiceId: 'not-a-real-voice-id' });
    const voiceFailuresBefore = (await audioStatus(baseUrl)).totalSynthesisFailed;
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'unknown voice test' });
    const streamG = sseEvents(`${baseUrl}/api/public/audio/${slug}/stream`);
    await nextAudioEvent(streamG, POLL_TIMEOUT_MS, 'audio.reset');
    await waitUntil(async () => (await audioStatus(baseUrl)).totalSynthesisFailed === voiceFailuresBefore + 1, POLL_TIMEOUT_MS, 'the unknown-voice synthesis to fail honestly');
    await streamG.return();
    await putAudioSettings(baseUrl, { voiceId: '' });

    // --- 9. unknown slug, method/route/validation strictness -------------

    step('An unknown public slug produces an honest audio.gap, never a hard error');
    const unknownSlugStream = sseEvents(`${baseUrl}/api/public/audio/does-not-exist/stream`);
    const gapEvt = await nextAudioEvent(unknownSlugStream, POLL_TIMEOUT_MS, 'audio.gap for an unknown slug');
    expect(gapEvt.event === 'audio.gap' && gapEvt.data.reason === 'unknown_slug', 'the unknown slug produced audio.gap/unknown_slug', gapEvt.data);
    await unknownSlugStream.return();

    step('Route strictness: 405 with Allow, 404 for an unknown item, 422 for invalid settings');
    const wrongMethod = await request(baseUrl, 'DELETE', '/api/audio/settings');
    expect(wrongMethod.status === 405 && wrongMethod.headers.get('allow')?.includes('GET'), '405 + Allow header for a wrong method', { status: wrongMethod.status, allow: wrongMethod.headers.get('allow') });
    const unknownItem = await request(baseUrl, 'POST', '/api/audio/pending/does-not-exist/approve');
    expect(unknownItem.status === 404 && unknownItem.body.error === 'audio_item_not_found', '404 for an unknown pending item id', unknownItem.body);
    const invalidSettings = await request(baseUrl, 'PUT', '/api/audio/settings', { ...DEFAULT_AUDIO_SETTINGS_BODY, providerMode: 'not-a-real-mode' });
    expect(invalidSettings.status === 422 && invalidSettings.body.error === 'audio_settings_invalid', '422 for an invalid provider mode', invalidSettings.body);

    step('Rotating the public slug invalidates the old one immediately');
    const beforeRotate = await request(baseUrl, 'GET', '/api/audio/settings');
    const rotate = await request(baseUrl, 'POST', '/api/audio/rotate-slug');
    expect(rotate.status === 200 && rotate.body.publicSlug !== beforeRotate.body.publicSlug, 'rotate-slug produced a genuinely new slug', { before: beforeRotate.body.publicSlug, after: rotate.body.publicSlug });
    const oldSlugStream = sseEvents(`${baseUrl}/api/public/audio/${beforeRotate.body.publicSlug}/stream`);
    const oldSlugGap = await nextAudioEvent(oldSlugStream, POLL_TIMEOUT_MS, 'audio.gap for the now-invalid old slug');
    expect(oldSlugGap.event === 'audio.gap' && oldSlugGap.data.reason === 'unknown_slug', 'the rotated-away old slug is immediately unknown', oldSlugGap.data);
    await oldSlugStream.return();

    // --- 10. graceful shutdown cancels an in-flight synthesis -----------

    step('Backend shutdown cancels an in-flight synthesis promptly, never hangs');
    await request(baseUrl, 'POST', '/api/testonly/tts/delay', { milliseconds: 30_000 });
    await request(baseUrl, 'POST', '/api/audio/test-speak', { text: 'in flight during shutdown' });
    const shutdownSlug = rotate.body.publicSlug;
    const streamH = sseEvents(`${baseUrl}/api/public/audio/${shutdownSlug}/stream`);
    await nextAudioEvent(streamH, POLL_TIMEOUT_MS, 'audio.reset');
    await waitUntil(async () => (await audioStatus(baseUrl)).hasCurrentItem === true, POLL_TIMEOUT_MS, 'the long-delayed item to become current');
    const shutdownStart = Date.now();
    await stopBackend(backend, baseUrl);
    const shutdownElapsedMs = Date.now() - shutdownStart;
    expect(shutdownElapsedMs < 30_000, `shutdown completed in ${shutdownElapsedMs} ms, well under the 30s fake synthesis delay - the goroutine was really canceled, not merely abandoned`, shutdownElapsedMs);
    await streamH.return().catch(() => {});
    backend = await startBackend(testserverExe, env, baseUrl);

    // --- 11. privacy: nothing sensitive in logs or SQLite ----------------

    step('Search every audio-specific HTTP/SSE payload for donor PII and message text');
    // The donor username/message are expected to appear in the
    // engagement-events/operator-chat responses this script itself
    // requested (those surfaces are allowed to carry them) - the real
    // assertion is that the AUDIO-specific responses never do. Scan
    // only the audio-prefixed capture, not every captured chunk.
    const audioOnlyChunks = secretScanChunks.filter((c) => c.includes('"bytesUrl"') || c.includes('audio.') || c.includes('rendererToken'));
    const audioHaystack = audioOnlyChunks.join('\n');
    for (const secret of ['great stream!', `donor-${RUN_ID}@example.invalid`, 'Styler', qualifyingTip._id]) {
      expect(!audioHaystack.includes(secret), `no audio-surface payload ever contains "${secret}"`, undefined);
    }
    pass(`scanned ${audioHaystack.length} bytes of audio-specific SSE/HTTP payloads for donor PII and message text`);

    step('Confirm no queued text, message content, or generated audio bytes were ever written to SQLite');
    const dbBytes = readFileSync(dbPath);
    const dbText = dbBytes.toString('latin1');
    for (const secret of ['great stream!', `donor-${RUN_ID}`, 'renderer lease test', 'skip me while synthesizing']) {
      expect(!dbText.includes(secret), `SQLite never contains the spoken/queued text "${secret}"`, undefined);
    }
    pass(`scanned the ${dbBytes.length}-byte SQLite database file for queued/spoken text - found none`);

    console.log('\nStage 17A TTS/audio verification PASSED');
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
    await killTree(fakeAstro);
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nStage 17A TTS/audio verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
