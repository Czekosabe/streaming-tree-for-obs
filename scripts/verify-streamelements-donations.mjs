#!/usr/bin/env node
/**
 * Local, no-real-StreamElements verification of the Stage 16A external-
 * donation foundation and StreamElements Astro WebSocket connector.
 *
 * This script never contacts real StreamElements. It runs the real
 * backend under test (`go build -tags integration ./cmd/testserver`)
 * against a real local WebSocket server
 * (`go build -tags integration ./cmd/fakestreamelements`) that speaks the
 * genuine Astro wire protocol - the real backend dials it exactly the way
 * it would dial `wss://astro.streamelements.com/` in production, over
 * `STREAMING_TREE_TEST_STREAMELEMENTS_WS_BASE_URL`, an env var that exists
 * only in the `-tags integration` binary. The fake is driven entirely over
 * its own small loopback HTTP control API (`/control/*`) - this script
 * never speaks the Astro protocol itself.
 *
 * This is a representative subset of the stage task's full ~45-scenario
 * enumeration, not a literal one-assertion-per-item transcription -
 * mirrors this project's own established convention (see
 * scripts/verify-twitch-engagement.mjs and
 * scripts/verify-youtube-engagement.mjs's own doc comments). Covered
 * here: donation-source CRUD and credential handling, the real Astro
 * connect/subscribe/welcome lifecycle, exact-money conversion end to end,
 * moderation pending/allowed/rejected semantics and dedup, sensitive-
 * field privacy discard (email/geo/paymentMethod/transactionId/payment
 * rail), operator-chat and alert-matcher integration including a real
 * monetary threshold and currency mismatch, a synthetic Test Rule that
 * never touches the fake provider, graceful reconnect (including a
 * rejected/invalid token falling back to a fresh connect), an unexpected
 * disconnect's possible-gap signal, disable/credential-replacement/
 * delete/backend-restart lifecycle behavior, and a privacy-leak scan of
 * every response body, SSE payload, and backend log line captured over
 * the whole run. Deliberately deferred to the Go unit/HTTP test suites
 * already covering them directly: connector.go's own backoff-timing unit
 * tests, chatautomation's own "donations never trigger a command" test,
 * and the outbound-chat target-selector's own provider allow-list test.
 *
 * Every JWT, tip id, and identifier used here is an obviously-fake string
 * generated for this run only. No real StreamElements account, PayPal
 * transaction, or network request to StreamElements is ever involved, and
 * no real OBS Browser Source is ever opened.
 *
 * Usage: node scripts/verify-streamelements-donations.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
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
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error(`timed out waiting for ${label}${lastError ? `: ${lastError.message}` : ''}`);
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

async function startFakeAstroServer(exePath, wsAddr, controlAddr) {
  const handle = spawnCaptured('fake-astro', exePath, [`-ws-addr=${wsAddr}`, `-control-addr=${controlAddr}`], { cwd: SERVER_DIR });
  await waitUntil(async () => {
    if (handle.hasExited()) throw new Error(`fake Astro server exited during startup:\n${handle.getOutput()}`);
    const res = await fetch(`http://${controlAddr}/control/health`).catch(() => null);
    return res !== null && res.ok ? true : false;
  }, READINESS_TIMEOUT_MS, 'the fake Astro server to become ready');
  return handle;
}

// --- fake Astro control-plane helpers -------------------------------------

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

// An optional predicate lets a caller wait for a specific connection state
// (e.g. "the subscribe frame has actually been processed: room/hasToken/
// subscribedTopics are populated") rather than merely "a WebSocket
// connection has been accepted" - the fake server registers a connection
// entry at accept time, before its subscribe message is parsed, so reading
// room/hasToken/subscribedTopics immediately after the bare
// items.length > 0 condition is a genuine read-before-write race, not a
// StreamElements product-connector defect. On timeout, the last observed
// connection snapshots are attached to the error for diagnostics - safe to
// include since hasToken is only ever a boolean, never the raw credential.
async function waitForConnection(controlBaseUrl, label, predicate) {
  let lastObserved = null;
  try {
    return await waitUntil(async () => {
      const res = await control(controlBaseUrl, 'GET', '/control/connections');
      lastObserved = res.body.items;
      if (res.body.items.length === 0) return false;
      if (!predicate) return res.body;
      return res.body.items.some(predicate) ? res.body : false;
    }, POLL_TIMEOUT_MS, label);
  } catch (error) {
    if (predicate && lastObserved) {
      const safe = lastObserved.map((item) => ({
        id: item.id, room: item.room, hasToken: item.hasToken, subscribedTopics: item.subscribedTopics,
      }));
      throw new Error(`${error.message} - last observed connections: ${JSON.stringify(safe)}`);
    }
    throw error;
  }
}

async function waitForNoConnection(controlBaseUrl, label) {
  return waitUntil(async () => {
    const res = await control(controlBaseUrl, 'GET', '/control/connections');
    return res.body.items.length === 0 ? true : false;
  }, POLL_TIMEOUT_MS, label);
}

async function connections(controlBaseUrl) {
  const res = await control(controlBaseUrl, 'GET', '/control/connections');
  return res.body;
}

async function pushTip(controlBaseUrl, topic, tip, connectionId = 'latest') {
  const res = await control(controlBaseUrl, 'POST', '/control/push-tip', { connectionId, topic, tip });
  expect(res.status === 204, `pushed a ${topic} tip (${tip._id})`, res.body);
}

// --- tip fixtures (verbatim Astro shape - internal/provider/streamelements/tip.go) ---

function tipFixture(overrides = {}) {
  const id = overrides.id ?? mintToken('tip');
  return {
    donation: {
      user: {
        username: overrides.username ?? 'Styler',
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

// --- Engagement Event Bus / operator-chat helpers -------------------------

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

async function operatorChatItems(baseUrl) {
  const res = await request(baseUrl, 'GET', '/api/operator-chat/items?after=0&limit=200');
  return res.body.items;
}

async function ruleBody(overrides) {
  return {
    name: overrides.name,
    enabled: overrides.enabled ?? true,
    eventType: overrides.eventType,
    priority: overrides.priority ?? 50,
    durationMs: overrides.durationMs ?? 1000,
    minimumQuantity: null,
    maximumQuantity: null,
    requiredRole: 'everyone',
    showPlatform: true,
    showUsername: true,
    showMessage: overrides.showMessage ?? true,
    showQuantity: false,
    textTemplate: overrides.textTemplate,
    entryAnimation: 'fade',
    exitAnimation: 'fade',
    animationDurationMs: 400,
    providers: overrides.providers ?? [],
    accounts: overrides.accounts ?? [],
    currency: overrides.currency ?? '',
    minimumAmountMicros: overrides.minimumAmountMicros ?? null,
    maximumAmountMicros: overrides.maximumAmountMicros ?? null,
    showAmount: overrides.showAmount ?? true,
  };
}

async function queueStatus(baseUrl, profileId) {
  const res = await request(baseUrl, 'GET', `/api/alert-profiles/${profileId}/queue`);
  return res.body;
}

function findAlert(status, substring) {
  const items = [status.current, ...(status.nextQueued ?? [])].filter(Boolean);
  return items.find((a) => a.renderedText?.includes(substring));
}

// --- SSE helpers (mirrors scripts/verify-alerts.mjs's own reader) --------

function parseSSEChunk(chunk) {
  let eventName = 'message';
  let data = '';
  for (const line of chunk.split('\n')) {
    if (line.startsWith('event:')) eventName = line.slice('event:'.length).trim();
    else if (line.startsWith('data:')) data += line.slice('data:'.length).trim();
  }
  let parsed = null;
  if (data !== '') {
    try {
      parsed = JSON.parse(data);
    } catch {
      parsed = data;
    }
  }
  return { event: eventName, data: parsed, raw: chunk };
}

async function* sseEvents(url) {
  const response = await fetch(url, { headers: { Accept: 'text/event-stream' } });
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  try {
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

// --- privacy scan ----------------------------------------------------------

const SENSITIVE_SUBSTRINGS = ['@example.invalid', 'geo', 'paymentMethod', 'transactionId', 'scheme'];

function scanForSensitiveFields(payload, label) {
  const rendered = JSON.stringify(payload);
  expect(!rendered.includes('@example.invalid'), `${label} never contains the donor email`, rendered);
  expect(!/"geo"\s*:/i.test(rendered), `${label} never contains a geo field`, rendered);
  expect(!/"paymentMethod"\s*:/i.test(rendered), `${label} never contains a paymentMethod field`, rendered);
  expect(!/"transactionId"\s*:/i.test(rendered), `${label} never contains a transactionId field`, rendered);
  expect(!rendered.includes('paypal'), `${label} never contains the payment-rail value "paypal"`, rendered);
}

// --- main ------------------------------------------------------------------

async function main() {
  console.log('StreamElements donations (Stage 16A) verification (local fakes only, no real StreamElements)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-streamelements-donations-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const testserverExe = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const fakeAstroExe = join(tempDir, process.platform === 'win32' ? 'fakestreamelements.exe' : 'fakestreamelements');

  let backend = null;
  let fakeAstro = null;
  let baseUrl;
  let controlBaseUrl;

  const jwt = mintToken('fake-jwt');
  const rotatedJwt = mintToken('fake-jwt-rotated');

  try {
    step('Build the fake Astro WebSocket server (go build -tags integration ./cmd/fakestreamelements)');
    await buildBinary('go-build-fakestreamelements', './cmd/fakestreamelements', fakeAstroExe);

    step('Build the integration-only test server (go build -tags integration ./cmd/testserver)');
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

    step('Start the backend under test with no donation sources configured');
    backend = await startBackend(testserverExe, env, baseUrl);
    const list0 = await request(baseUrl, 'GET', '/api/donation-sources');
    expect(list0.status === 200 && list0.body.items.length === 0, 'the donation-source list starts empty', list0.body);

    // --- 1. donation-source CRUD + credential handling ------------------

    step('Create a StreamElements donation source [assertions: CRUD persists safe metadata, credential never returned]');
    const create = await request(baseUrl, 'POST', '/api/donation-sources', {
      providerId: 'streamelements', label: 'Main channel', remoteChannelId: 'chan_1', token: jwt,
    });
    expect(create.status === 201, 'the source was created', create.body);
    const source = create.body;
    expect(source.enabled === false, 'a freshly created source starts disabled', source);
    expect(source.credentialConfigured === true, 'credentialConfigured is true immediately after create', source);
    expect(!('token' in source) && !JSON.stringify(source).includes(jwt), 'the create response never echoes the credential', source);

    const getSource = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}`);
    expect(getSource.status === 200 && getSource.body.label === 'Main channel', 'GET round-trips the created source', getSource.body);

    const listAfterCreate = await request(baseUrl, 'GET', '/api/donation-sources');
    expect(listAfterCreate.body.items.some((s) => s.id === source.id), 'the created source is listed', listAfterCreate.body);

    const engagement0 = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
    expect(engagement0.status === 200 && engagement0.body.state === 'disabled' && engagement0.body.enabled === false,
      'the connector reports disabled before ever being enabled', engagement0.body);

    // --- 2. enable, real Astro connect/welcome/subscribe lifecycle ------

    step('Enable the source [assertions: opens a real WebSocket, welcome handled, subscribes to channel.tips + channel.tips.moderation, credential present but never exposed]');
    const enable = await request(baseUrl, 'PUT', `/api/donation-sources/${source.id}/engagement`, { enabled: true });
    expect(enable.status === 200 && enable.body.enabled === true, 'the enable request succeeds', enable.body);

    const conn1 = await waitForConnection(
      controlBaseUrl,
      'the fake Astro server to observe a fully subscribed connection (room=chan_1, credential present, both tip topics)',
      (item) => item.room === 'chan_1' && item.hasToken === true
        && Array.isArray(item.subscribedTopics)
        && item.subscribedTopics.includes('channel.tips')
        && item.subscribedTopics.includes('channel.tips.moderation'),
    );
    const info1 = conn1.items[0];
    expect(info1.room === 'chan_1', 'the subscribe request carried the source\'s own remoteChannelId as room', info1);
    expect(info1.hasToken === true, 'a credential was supplied on subscribe', info1);
    expect(
      info1.subscribedTopics.includes('channel.tips') && info1.subscribedTopics.includes('channel.tips.moderation'),
      'subscribed to both channel.tips and channel.tips.moderation', info1,
    );
    expect(!info1.subscribedTopics.includes('channel.activities'), 'never subscribed to channel.activities as a duplicate tip path', info1);

    await waitUntil(async () => {
      const eng = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
      return eng.body.state === 'connected' ? true : false;
    }, POLL_TIMEOUT_MS, 'the connector to reach state "connected"');
    pass('the connector reached state "connected"');

    // --- 3. exact money conversion + normalization end to end -----------

    step('Push an allowed tip and verify exact money conversion, normalization, and privacy discard on the Engagement Event Bus');
    const tip1 = tipFixture({ amount: 4.2, currency: 'usd' });
    await pushTip(controlBaseUrl, 'channel.tips', tip1);
    const event1 = await waitForEvent(baseUrl, (e) => e.providerEventId === tip1._id, POLL_TIMEOUT_MS, 'the donation event to appear on the Event Bus');
    expect(event1.type === 'donation', 'the event type is "donation"', event1.type);
    expect(event1.providerId === 'streamelements', 'the event providerId is "streamelements", never the payment rail', event1.providerId);
    expect(event1.connectedAccountId === source.id, 'the event carries the donation source\'s own id', event1.connectedAccountId);
    expect(event1.amountMicros === 4_200_000, 'amount 4.20 converts to exactly 4200000 micros (no float rounding)', event1.amountMicros);
    expect(event1.currency === 'USD', 'a lowercase "usd" currency is normalized to uppercase', event1.currency);
    expect(event1.user?.displayName === 'Styler', 'the donor display name is preserved', event1.user);
    expect(event1.message?.text === 'great stream!', 'the donation message is preserved as plain text', event1.message);
    scanForSensitiveFields(event1, 'the public engagement-event payload');

    step('Confirm the donation appears in operator chat with its amount, and without sensitive fields');
    const chatItem1 = await waitUntil(async () => {
      const items = await operatorChatItems(baseUrl);
      return items.find((i) => i.sourceEventId === event1.id || (i.kind === 'activity' && i.activity?.activityType === 'donation' && i.providerId === 'streamelements')) ?? false;
    }, POLL_TIMEOUT_MS, 'the donation to appear as an operator-chat activity');
    expect(chatItem1.activity?.activityType === 'donation', 'the operator-chat activity type is "donation"', chatItem1.activity);
    expect(chatItem1.activity?.displayAmount !== undefined && chatItem1.activity.displayAmount !== '', 'the operator-chat activity carries a display amount', chatItem1.activity);
    scanForSensitiveFields(chatItem1, 'the operator-chat activity payload');

    // --- 4. moderation semantics + dedup ---------------------------------

    step('Confirm a pending tip produces no donation event, and rejected never does either');
    const beforePending = await engagementEvents(baseUrl);
    const pendingTip = tipFixture({ approved: 'pending' });
    await pushTip(controlBaseUrl, 'channel.tips', pendingTip);
    await new Promise((r) => setTimeout(r, 400));
    const afterPending = await engagementEvents(baseUrl);
    expect(afterPending.length === beforePending.length, 'a pending tip produces no side effect', { before: beforePending.length, after: afterPending.length });

    const rejectedTip = tipFixture({ approved: 'rejected' });
    await pushTip(controlBaseUrl, 'channel.tips', rejectedTip);
    await new Promise((r) => setTimeout(r, 400));
    const afterRejected = await engagementEvents(baseUrl);
    expect(afterRejected.length === beforePending.length, 'a rejected tip produces no side effect', { before: beforePending.length, after: afterRejected.length });

    step('Confirm a later moderation "allowed" update for the same tip id publishes exactly one donation');
    const modTip = tipFixture({ approved: 'allowed' });
    // The pending state for this exact tip id was never published (per the
    // scenario above) - the moderation-topic "allowed" update is this tip
    // id's first-ever publishable state transition.
    await pushTip(controlBaseUrl, 'channel.tips.moderation', modTip);
    const modEvent = await waitForEvent(baseUrl, (e) => e.providerEventId === modTip._id, POLL_TIMEOUT_MS, 'the moderation-approved donation to publish');
    expect(modEvent.type === 'donation', 'the moderation-topic tip publishes as a real donation once allowed', modEvent);

    step('Confirm a duplicate allowed update for the same tip id never duplicates the event');
    const beforeDuplicate = (await engagementEvents(baseUrl)).length;
    await pushTip(controlBaseUrl, 'channel.tips.moderation', modTip);
    await new Promise((r) => setTimeout(r, 400));
    const afterDuplicate = await engagementEvents(baseUrl);
    expect(afterDuplicate.length === beforeDuplicate, 'a repeated allowed update for the same tip id produces exactly one real event', {
      before: beforeDuplicate, after: afterDuplicate.length,
    });

    step('Confirm a different tip id with the same amount/name produces a distinct event');
    const tip2 = tipFixture({ amount: 4.2, username: 'Styler' });
    expect(tip2._id !== tip1._id, 'the two fixtures have distinct tip ids', { a: tip1._id, b: tip2._id });
    await pushTip(controlBaseUrl, 'channel.tips', tip2);
    const event2 = await waitForEvent(baseUrl, (e) => e.providerEventId === tip2._id, POLL_TIMEOUT_MS, 'the second, distinct donation to publish');
    expect(event2.id !== event1.id, 'the second donation is a genuinely distinct event', { first: event1.id, second: event2.id });

    step('Confirm the payment rail "paypal" never becomes the engagement providerId, and a malformed amount is rejected without a crash');
    expect(event2.providerId === 'streamelements', 'providerId stays "streamelements" regardless of the tip\'s own payment rail', event2.providerId);
    const beforeMalformed = (await engagementEvents(baseUrl)).length;
    const malformedTip = tipFixture({ amount: 'not-a-number' });
    await pushTip(controlBaseUrl, 'channel.tips', malformedTip);
    await new Promise((r) => setTimeout(r, 400));
    const afterMalformed = await engagementEvents(baseUrl);
    expect(afterMalformed.length === beforeMalformed, 'a malformed amount is rejected safely - no event published', {
      before: beforeMalformed, after: afterMalformed.length,
    });
    const healthAfterMalformed = await fetch(`${baseUrl}/api/health`);
    expect(healthAfterMalformed.ok, 'the backend is still healthy after a malformed tip payload', healthAfterMalformed.status);

    // --- 5. alerts: real monetary threshold + currency mismatch ---------

    step('Create an alert profile and a donation rule with a monetary threshold, then verify matching, non-matching, and currency-mismatch behavior');
    const profileResp = await request(baseUrl, 'POST', '/api/alert-profiles', { name: 'Donations' });
    expect(profileResp.status === 201, 'the alert profile was created', profileResp.body);
    const profile = profileResp.body;

    const ruleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, await ruleBody({
      name: 'Donation threshold', eventType: 'donation', providers: ['streamelements'], accounts: [source.id],
      currency: 'USD', minimumAmountMicros: 1_000_000, maximumAmountMicros: null,
      textTemplate: '{username} donated {amount} (donation-alert)!',
    }));
    expect(ruleResp.status === 201, 'the donation alert rule was created', ruleResp.body);
    const rule = ruleResp.body;

    const belowThresholdTip = tipFixture({ amount: 0.5, currency: 'USD' });
    await pushTip(controlBaseUrl, 'channel.tips', belowThresholdTip);
    await waitForEvent(baseUrl, (e) => e.providerEventId === belowThresholdTip._id, POLL_TIMEOUT_MS, 'the below-threshold donation to reach the Event Bus');
    await new Promise((r) => setTimeout(r, 400));
    let statusBelow = await queueStatus(baseUrl, profile.id);
    expect(findAlert(statusBelow, 'donation-alert') === undefined, 'a donation below the minimum threshold does not trigger the alert', statusBelow);

    const mismatchCurrencyTip = tipFixture({ amount: 5, currency: 'EUR' });
    await pushTip(controlBaseUrl, 'channel.tips', mismatchCurrencyTip);
    await waitForEvent(baseUrl, (e) => e.providerEventId === mismatchCurrencyTip._id, POLL_TIMEOUT_MS, 'the mismatched-currency donation to reach the Event Bus');
    await new Promise((r) => setTimeout(r, 400));
    let statusMismatch = await queueStatus(baseUrl, profile.id);
    expect(findAlert(statusMismatch, 'donation-alert') === undefined, 'a donation in a different currency never matches (amounts are never compared across currencies)', statusMismatch);

    const matchingTip = tipFixture({ amount: 10, currency: 'USD', username: 'BigDonor' });
    await pushTip(controlBaseUrl, 'channel.tips', matchingTip);
    const matchedStatus = await waitUntil(async () => {
      const st = await queueStatus(baseUrl, profile.id);
      return findAlert(st, 'donation-alert') ?? false;
    }, POLL_TIMEOUT_MS, 'the matching donation to trigger the alert');
    expect(matchedStatus.renderedText.includes('BigDonor'), 'the triggered alert carries the real donor name', matchedStatus);
    scanForSensitiveFields(matchedStatus, 'the alert queue payload');

    step('Confirm a synthetic Test Rule alert never contacts the fake provider');
    const connectionsBeforeTest = (await connections(controlBaseUrl)).items.length;
    const testResp = await request(baseUrl, 'POST', `/api/alert-rules/${rule.id}/test`, {});
    expect(testResp.status === 200 || testResp.status === 201 || testResp.status === 202, 'the Test Rule request succeeds', testResp);
    await new Promise((r) => setTimeout(r, 300));
    const connectionsAfterTest = (await connections(controlBaseUrl)).items.length;
    expect(connectionsAfterTest === connectionsBeforeTest, 'Test Rule never opens or closes a connection to the fake provider', {
      before: connectionsBeforeTest, after: connectionsAfterTest,
    });

    step('Confirm the public alert stream never leaks a sensitive field for a real donation alert');
    const iter = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter, POLL_TIMEOUT_MS, 'the public stream\'s initial snapshot');
    const publicTip = tipFixture({ amount: 12, currency: 'USD', username: 'PublicDonor' });
    await pushTip(controlBaseUrl, 'channel.tips', publicTip);
    let publicShow = null;
    for (let i = 0; i < 20 && publicShow === null; i += 1) {
      // eslint-disable-next-line no-await-in-loop
      const evt = await nextEvent(iter, POLL_TIMEOUT_MS, 'a public alert.show event');
      const alert = evt.data?.alert;
      if (evt.event === 'alert.show' && typeof alert?.renderedText === 'string' && alert.renderedText.includes('PublicDonor')) {
        publicShow = alert;
      }
    }
    expect(publicShow !== null, 'the public stream delivered the real donation alert', publicShow);
    scanForSensitiveFields(publicShow, 'the public alert-stream payload');

    // --- 6. graceful reconnect + unexpected disconnect -------------------

    step('Push a graceful reconnect: the connector resumes with the reconnect_token and never shows a possible gap');
    const beforeReconnect = await connections(controlBaseUrl);
    const graceToken = mintToken('grace');
    const reconnectPush = await control(controlBaseUrl, 'POST', '/control/push-reconnect', {
      connectionId: 'latest', token: graceToken,
    });
    expect(reconnectPush.status === 204, 'pushed a graceful reconnect envelope', reconnectPush.body);

    await waitUntil(async () => {
      const conns = await connections(controlBaseUrl);
      return conns.items.some((c) => c.resumedWithToken === true && c.id > beforeReconnect.latestId) ? conns : false;
    }, POLL_TIMEOUT_MS, 'the connector to resume with the reconnect_token on a new connection');
    const engAfterGraceful = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
    expect(engAfterGraceful.body.state === 'connected', 'a graceful reconnect never shows a possible gap', engAfterGraceful.body);
    expect(engAfterGraceful.body.possibleGapCount === 0, 'possibleGapCount stays 0 through a graceful reconnect', engAfterGraceful.body);

    step('An unexpected disconnect marks a possible gap, then a real tip clears it back to connected');
    await control(controlBaseUrl, 'POST', '/control/disconnect', { connectionId: 'latest' });
    await waitUntil(async () => {
      const eng = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
      return eng.body.state === 'possible_gap' ? eng.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to report a possible gap after an unexpected disconnect');
    pass('the connector honestly reported a possible gap');

    await waitForConnection(controlBaseUrl, 'a fresh connection after the unexpected disconnect');
    const tipAfterGap = tipFixture({ amount: 1, username: 'AfterGap' });
    await pushTip(controlBaseUrl, 'channel.tips', tipAfterGap);
    await waitForEvent(baseUrl, (e) => e.providerEventId === tipAfterGap._id, POLL_TIMEOUT_MS, 'the post-gap donation to publish');
    await waitUntil(async () => {
      const eng = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
      return eng.body.state === 'connected' ? true : false;
    }, POLL_TIMEOUT_MS, 'the connector to return to "connected" after a real tip arrives');
    pass('a real tip arriving cleared the possible-gap display');

    step('An invalid/expired reconnect token falls back to an ordinary fresh connection, never loops forever');
    const beforeStaleToken = await connections(controlBaseUrl);
    const staleToken = mintToken('stale');
    await control(controlBaseUrl, 'POST', '/control/push-reconnect', { connectionId: 'latest', token: staleToken, markValid: false });
    // The first reconnect attempt (using the now-rejected staleToken) fails
    // outright - the client's own Connect() cannot distinguish "token
    // rejected" from any other transient connect failure (see
    // docs/provider-integrations/external-donations.md §31), so the
    // connector falls back to an ordinary fresh connect (no resume token)
    // on its next bounded-backoff attempt, landing on state possible_gap
    // (an honest signal, not "connected" yet - a resumed connection is the
    // only path that skips that display) until a real tip clears it.
    await waitUntil(async () => {
      const conns = await connections(controlBaseUrl);
      return conns.items.some((c) => c.resumedWithToken === false && c.id > beforeStaleToken.latestId) ? conns : false;
    }, POLL_TIMEOUT_MS, 'the connector to fall back to an ordinary fresh connection after the stale reconnect token was rejected');
    pass('the connector fell back to an ordinary fresh connection rather than looping forever on the rejected token');

    const tipAfterStaleToken = tipFixture({ amount: 1, username: 'AfterStaleToken' });
    await pushTip(controlBaseUrl, 'channel.tips', tipAfterStaleToken);
    await waitForEvent(baseUrl, (e) => e.providerEventId === tipAfterStaleToken._id, POLL_TIMEOUT_MS, 'the post-fallback donation to publish');
    await waitUntil(async () => {
      const eng = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
      return eng.body.state === 'connected' ? true : false;
    }, POLL_TIMEOUT_MS, 'the connector to fully recover to "connected" after the fallback');
    pass('the connector fully recovered to "connected" after falling back from the rejected reconnect token');

    // --- 7. disable / credential replacement / delete / restart ---------

    step('Disable the source: the connection closes');
    const disable = await request(baseUrl, 'PUT', `/api/donation-sources/${source.id}/engagement`, { enabled: false });
    expect(disable.status === 200 && disable.body.enabled === false, 'the disable request succeeds', disable.body);
    await waitForNoConnection(controlBaseUrl, 'the fake Astro server to have no active connections after disable');
    pass('disable closed the WebSocket connection');

    step('Replace the credential, then restart: the connector reconnects cleanly using the NEW token');
    await control(controlBaseUrl, 'POST', '/control/reset');
    const replace = await request(baseUrl, 'PUT', `/api/donation-sources/${source.id}/credential`, { token: rotatedJwt });
    expect(replace.status === 200 && replace.body.configured === true, 'the credential was replaced', replace.body);
    expect(!JSON.stringify(replace.body).includes(rotatedJwt), 'the credential-replace response never echoes the new token', replace.body);

    await control(controlBaseUrl, 'POST', '/control/require-token', { token: rotatedJwt });
    const reEnable = await request(baseUrl, 'PUT', `/api/donation-sources/${source.id}/engagement`, { enabled: true });
    expect(reEnable.status === 200, 're-enabled after credential replacement', reEnable.body);
    await waitUntil(async () => {
      const eng = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}/engagement`);
      return eng.body.state === 'connected' ? true : false;
    }, POLL_TIMEOUT_MS, 'the connector to connect cleanly with the rotated credential');
    pass('the connector reconnected successfully using the rotated credential');
    await control(controlBaseUrl, 'POST', '/control/require-token', { token: '' });

    step('Delete the source: the connection closes and the credential is removed');
    const del = await request(baseUrl, 'DELETE', `/api/donation-sources/${source.id}`);
    expect(del.status === 204, 'the source was deleted', del.status);
    await waitForNoConnection(controlBaseUrl, 'the fake Astro server to have no active connections after delete');
    const getDeleted = await request(baseUrl, 'GET', `/api/donation-sources/${source.id}`);
    expect(getDeleted.status === 404, 'the deleted source is no longer gettable', getDeleted.body);

    step('A backend restart preserves the enabled flag, restarts the connector automatically, and never replays donation history');
    const create2 = await request(baseUrl, 'POST', '/api/donation-sources', {
      providerId: 'streamelements', label: 'Restart source', remoteChannelId: 'chan_2', token: mintToken('restart-jwt'),
    });
    const source2 = create2.body;
    await request(baseUrl, 'PUT', `/api/donation-sources/${source2.id}/engagement`, { enabled: true });
    await waitForConnection(controlBaseUrl, 'the restart-test source to connect before restart');
    const eventsBeforeRestart = (await engagementEvents(baseUrl)).length;

    await stopBackend(backend, baseUrl);
    backend = await startBackend(testserverExe, env, baseUrl);

    const sourceAfterRestart = await request(baseUrl, 'GET', `/api/donation-sources/${source2.id}`);
    expect(sourceAfterRestart.body.enabled === true, 'the enabled flag persisted across a full backend restart', sourceAfterRestart.body);

    // The `-tags integration` testserver's own credential store
    // (internal/secrets/secretstest) is a deliberately in-memory-only fake
    // (see cmd/testserver/main.go's own doc comment) - it has no real
    // credential left to reconnect with after a process restart, so this
    // asserts what IS actually provable in this harness: Start() attempted
    // to reconcile the still-enabled source automatically (reaching a real
    // active connector state, not "disabled"), exactly mirroring
    // scripts/verify-youtube-engagement.mjs's own restart assertion, which
    // for the identical reason checks the persisted enabled flag rather
    // than a live reconnect.
    const engAfterRestart = await waitUntil(async () => {
      const eng = await request(baseUrl, 'GET', `/api/donation-sources/${source2.id}/engagement`);
      return eng.body.state !== 'disabled' ? eng.body : false;
    }, POLL_TIMEOUT_MS, 'Start() to reconcile the still-enabled source automatically after a backend restart');
    pass(`the connector was reconciled automatically after restart (state: ${engAfterRestart.state})`);

    const eventsAfterRestart = await engagementEvents(baseUrl);
    expect(eventsAfterRestart.length === 0, 'a backend restart never replays donation history - the in-memory Event Bus starts empty', {
      beforeRestartCount: eventsBeforeRestart, afterRestartCount: eventsAfterRestart.length,
    });

    // --- 8. final secret scan ---------------------------------------------

    step('Search every captured HTTP body, SSE payload, and backend log line for real secret material');
    const haystack = secretScanChunks.join('\n');
    for (const secret of [jwt, rotatedJwt]) {
      const index = haystack.indexOf(secret);
      expect(index === -1, `the credential ${secret.slice(0, 12)}... never appears in any captured response or log line`,
        index === -1 ? undefined : haystack.slice(Math.max(0, index - 200), index + 200));
    }
    pass(`scanned ${haystack.length} bytes of backend stdout/stderr, HTTP response bodies, and SSE payloads for 2 issued credentials`);

    console.log('\nStage 16A StreamElements donations verification PASSED');
  } finally {
    if (backend !== null && baseUrl !== undefined) {
      try {
        await stopBackend(backend, baseUrl);
      } catch {
        // Already reporting a failure if we get here.
      }
    }
    await killTree(fakeAstro);
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nStage 16A StreamElements donations verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
