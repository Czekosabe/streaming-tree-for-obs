#!/usr/bin/env node
/**
 * Local, no-real-Twitch, no-real-OBS verification of Stage 12B: bounded
 * alert grouping and deterministic mid-alert preemption, layered on top
 * of the Stage 12A queue/playback runtime already covered by
 * scripts/verify-alerts.mjs (which this script does not repeat).
 *
 * Reuses the identical fake OAuth/Helix/EventSub harness (same shapes,
 * copied verbatim - see verify-alerts.mjs and verify-twitch-engagement.mjs
 * for the canonical version of each helper) but a fresh, focused main()
 * covering a representative subset of the Stage 12B task's own ~39-item
 * Part 43 verification list. Every scenario intentionally NOT covered
 * here is named against a specific covering Go/frontend test in
 * docs/progress.md's Stage 12B test-verification entry.
 *
 * Usage: node scripts/verify-alert-advanced-queue.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { createServer as createHttpServer } from 'node:http';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

const READINESS_TIMEOUT_MS = 30_000;
const BUILD_TIMEOUT_MS = 120_000;
const SHUTDOWN_TIMEOUT_MS = 15_000;
const POLL_TIMEOUT_MS = 60_000;
const QUEUE_TIMEOUT_MS = 8_000;

const RUN_ID = randomUUID().slice(0, 8);
const CLIENT_ID = `fake-client-id-${RUN_ID}`;

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
    await new Promise((r) => setTimeout(r, 200));
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

function listen(server, port) {
  return new Promise((resolveListen, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => resolveListen());
  });
}

function close(server) {
  return new Promise((resolveClose) => server.close(() => resolveClose()));
}

async function readBody(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  return Buffer.concat(chunks).toString('utf8');
}

function sendJSON(res, status, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(status, { 'Content-Type': 'application/json', Connection: 'close' });
  res.end(body);
}

function mintToken(prefix) {
  return `${prefix}-${RUN_ID}-${randomBytes(12).toString('hex')}`;
}

// --- fake Twitch OAuth + Helix servers (see verify-twitch-engagement.mjs
// for the identical shapes) --------------------------------------------

function newTwitchFakeState() {
  return {
    devices: new Map(),
    accessTokens: new Map(),
    refreshTokens: new Map(),
    users: new Map(),
    eventsubSubscriptions: [],
  };
}

function issueTokenPair(state, userId, scopes) {
  const accessToken = mintToken('fake-access');
  const refreshToken = mintToken('fake-refresh');
  state.accessTokens.set(accessToken, { valid: true, userId, scopes });
  state.refreshTokens.set(refreshToken, { valid: true, userId, scopes });
  return { accessToken, refreshToken };
}

function createFakeOAuthServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');

      if (req.method === 'POST' && url.pathname === '/device') {
        const form = new URLSearchParams(await readBody(req));
        const deviceCode = mintToken('fake-device-code');
        const userCode = randomBytes(4).toString('hex').toUpperCase().slice(0, 8).replace(/(.{4})(.{4})/, '$1-$2');
        state.devices.set(deviceCode, {
          userCode, pollCount: 0, authorized: false, userId: null,
          scopes: (form.get('scopes') ?? '').split(' ').filter(Boolean),
        });
        sendJSON(res, 200, {
          device_code: deviceCode, user_code: userCode,
          verification_uri: 'https://fake.twitch.example/activate',
          expires_in: 1800, interval: 1,
        });
        return;
      }

      if (req.method === 'POST' && url.pathname === '/token') {
        const form = new URLSearchParams(await readBody(req));
        const grantType = form.get('grant_type');

        if (grantType === 'urn:ietf:params:oauth:grant-type:device_code') {
          const deviceCode = form.get('device_code');
          const device = state.devices.get(deviceCode);
          if (device === undefined) {
            sendJSON(res, 400, { status: 400, message: 'invalid device code' });
            return;
          }
          device.pollCount += 1;
          if (device.pollCount === 1 || !device.authorized) {
            sendJSON(res, 400, { status: 400, message: 'authorization_pending' });
            return;
          }
          state.devices.delete(deviceCode);
          const { accessToken, refreshToken } = issueTokenPair(state, device.userId, device.scopes);
          sendJSON(res, 200, {
            access_token: accessToken, refresh_token: refreshToken,
            scope: device.scopes, token_type: 'bearer', expires_in: 14_400,
          });
          return;
        }

        if (grantType === 'refresh_token') {
          const oldRefresh = form.get('refresh_token');
          const entry = state.refreshTokens.get(oldRefresh);
          if (entry === undefined || !entry.valid) {
            sendJSON(res, 400, { status: 400, message: 'invalid refresh token' });
            return;
          }
          entry.valid = false;
          const { accessToken, refreshToken } = issueTokenPair(state, entry.userId, entry.scopes);
          sendJSON(res, 200, {
            access_token: accessToken, refresh_token: refreshToken,
            scope: entry.scopes, token_type: 'bearer', expires_in: 14_400,
          });
          return;
        }

        sendJSON(res, 400, { status: 400, message: 'unsupported grant type' });
        return;
      }

      if (req.method === 'GET' && url.pathname === '/validate') {
        const auth = req.headers.authorization ?? '';
        const token = auth.startsWith('OAuth ') ? auth.slice('OAuth '.length) : '';
        const entry = state.accessTokens.get(token);
        if (entry === undefined || !entry.valid) {
          sendJSON(res, 401, { status: 401, message: 'invalid access token' });
          return;
        }
        const user = state.users.get(entry.userId);
        sendJSON(res, 200, {
          client_id: CLIENT_ID, login: user?.login ?? '', user_id: entry.userId,
          scopes: entry.scopes, expires_in: 14_400,
        });
        return;
      }

      if (req.method === 'POST' && url.pathname === '/revoke') {
        const form = new URLSearchParams(await readBody(req));
        const token = form.get('token');
        const entry = state.accessTokens.get(token);
        if (entry !== undefined) entry.valid = false;
        res.writeHead(200, { Connection: 'close' });
        res.end();
        return;
      }

      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { status: 500, message: String(error) });
    }
  });
}

function createFakeHelixServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');
      const auth = req.headers.authorization ?? '';
      const token = auth.startsWith('Bearer ') ? auth.slice('Bearer '.length) : '';
      const entry = state.accessTokens.get(token);
      const rateLimitHeaders = { Connection: 'close' };

      if (entry === undefined || !entry.valid) {
        res.writeHead(401, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({ error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' }));
        return;
      }

      if (req.method === 'GET' && url.pathname === '/users') {
        const user = state.users.get(entry.userId);
        res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({ data: [{ id: user.id, login: user.login, display_name: user.displayName }] }));
        return;
      }

      if (req.method === 'POST' && url.pathname === '/eventsub/subscriptions') {
        const raw = await readBody(req);
        record(raw);
        const parsed = JSON.parse(raw);
        state.eventsubSubscriptions.push({
          type: parsed.type, version: parsed.version, condition: parsed.condition,
          sessionId: parsed.transport?.session_id,
        });
        res.writeHead(202, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({ data: [{ id: mintToken('fake-sub'), status: 'enabled' }] }));
        return;
      }

      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { status: 500, message: String(error) });
    }
  });
}

// --- fake EventSub WebSocket server (identical minimal RFC 6455 subset
// to verify-twitch-engagement.mjs) --------------------------------------

function computeAcceptKey(key) {
  return createHash('sha1').update(key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
}

function encodeTextFrame(payloadStr) {
  const payload = Buffer.from(payloadStr, 'utf8');
  const len = payload.length;
  let header;
  if (len < 126) {
    header = Buffer.from([0x81, len]);
  } else if (len < 65536) {
    header = Buffer.alloc(4);
    header[0] = 0x81;
    header[1] = 126;
    header.writeUInt16BE(len, 2);
  } else {
    header = Buffer.alloc(10);
    header[0] = 0x81;
    header[1] = 127;
    header.writeBigUInt64BE(BigInt(len), 2);
  }
  return Buffer.concat([header, payload]);
}

function sendWS(socket, envelope) {
  if (socket === undefined || socket === null || socket.destroyed) return;
  socket.write(encodeTextFrame(JSON.stringify(envelope)));
}

function newEventSubServerState() {
  return { connections: [], pendingResolvers: [] };
}

function nextConnection(state, timeoutMs = 10_000) {
  return new Promise((resolveConn, reject) => {
    const timer = setTimeout(() => reject(new Error('timed out waiting for a WebSocket connection')), timeoutMs);
    state.pendingResolvers.push((socket) => {
      clearTimeout(timer);
      resolveConn(socket);
    });
  });
}

function createFakeEventSubServer(state) {
  const server = createHttpServer((_req, res) => {
    res.writeHead(404);
    res.end();
  });
  server.on('upgrade', (req, socket) => {
    const key = req.headers['sec-websocket-key'];
    socket.write(
      'HTTP/1.1 101 Switching Protocols\r\n' +
        'Upgrade: websocket\r\n' +
        'Connection: Upgrade\r\n' +
        `Sec-WebSocket-Accept: ${computeAcceptKey(key)}\r\n\r\n`,
    );
    socket.on('data', () => {});
    socket.on('error', () => {});
    state.connections.push(socket);
    const resolver = state.pendingResolvers.shift();
    if (resolver !== undefined) resolver(socket);
  });
  return server;
}

function welcomeEnvelope(sessionId, keepaliveSeconds) {
  return {
    metadata: { message_id: mintToken('wsmsg'), message_type: 'session_welcome', message_timestamp: new Date().toISOString() },
    payload: { session: { id: sessionId, status: 'connected', keepalive_timeout_seconds: keepaliveSeconds, reconnect_url: null } },
  };
}

function notificationEnvelope(subType, event, messageId) {
  return {
    metadata: {
      message_id: messageId ?? mintToken('notif'), message_type: 'notification',
      message_timestamp: new Date().toISOString(), subscription_type: subType, subscription_version: '1',
    },
    payload: { subscription: { id: 'sub_x', status: 'enabled', type: subType, version: '1' }, event },
  };
}

// --- EventSub notification builders needed for Stage 12B: cheer (Bits
// grouping), channel-point redemption (reward-id grouping), follow and
// raid (preemption priority pairs) ---------------------------------------

let msgSeq = 0;
function nextMsgId() {
  msgSeq += 1;
  return `msg_${RUN_ID}_${msgSeq}`;
}

function followEvent(userId, login, name) {
  return notificationEnvelope('channel.follow', { user_id: userId, user_login: login, user_name: name, followed_at: new Date().toISOString() }, nextMsgId());
}

function cheerEvent(userId, login, name, bits, { isAnonymous = false, message = 'Cheer!' } = {}) {
  return notificationEnvelope('channel.cheer', { user_id: userId, user_login: login, user_name: name, is_anonymous: isAnonymous, message, bits }, nextMsgId());
}

function raidEvent(fromUserId, fromLogin, fromName, viewers) {
  return notificationEnvelope('channel.raid', { from_broadcaster_user_id: fromUserId, from_broadcaster_user_login: fromLogin, from_broadcaster_user_name: fromName, viewers }, nextMsgId());
}

/** Unlike verify-alerts.mjs's own redemptionEvent, this takes an explicit
 * rewardId so two redemptions of the *same* reusable reward (Stage 12B's
 * own grouping subject) can be produced for the grouping-by-subject test. */
function redemptionEventWithReward(userId, login, name, rewardId, rewardTitle, userInput) {
  return notificationEnvelope('channel.channel_points_custom_reward_redemption.add', {
    id: mintToken('redeem'), user_id: userId, user_login: login, user_name: name, user_input: userInput,
    reward: { id: rewardId, title: rewardTitle, cost: 100 }, redeemed_at: new Date().toISOString(),
  }, nextMsgId());
}

// --- SSE helpers (mirrors scripts/verify-chat-overlay.mjs's own reader) --

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

// --- alert queue helpers --------------------------------------------------

async function queueStatus(baseUrl, profileId) {
  const res = await request(baseUrl, 'GET', `/api/alert-profiles/${profileId}/queue`);
  return res.body;
}

async function waitForQueue(baseUrl, profileId, predicate, timeoutMs, label) {
  return waitUntil(async () => {
    const st = await queueStatus(baseUrl, profileId);
    return predicate(st) ? st : false;
  }, timeoutMs, label);
}

function findAlert(status, substring) {
  const items = [status.current, ...(status.nextQueued ?? [])].filter(Boolean);
  return items.find((a) => a.renderedText.includes(substring));
}

/** Extends verify-alerts.mjs's own ruleBody with Stage 12B's 4 new
 * fields, defaulted to the documented Stage-12A-preserving safe values
 * (allowGrouping=false, groupWindowMs=DEFAULT, interruptMode='never',
 * interruptible=true) exactly like the rule editor's own emptyDraft. */
function ruleBody(overrides) {
  return {
    name: overrides.name,
    enabled: overrides.enabled ?? true,
    eventType: overrides.eventType,
    priority: overrides.priority ?? 50,
    durationMs: overrides.durationMs ?? 1000,
    minimumQuantity: overrides.minimumQuantity ?? null,
    maximumQuantity: overrides.maximumQuantity ?? null,
    requiredRole: 'everyone',
    showPlatform: true,
    showUsername: overrides.showUsername ?? true,
    showMessage: overrides.showMessage ?? false,
    showQuantity: overrides.showQuantity ?? false,
    textTemplate: overrides.textTemplate,
    entryAnimation: 'fade',
    exitAnimation: 'fade',
    animationDurationMs: 400,
    providers: overrides.providers ?? [],
    accounts: overrides.accounts ?? [],
    allowGrouping: overrides.allowGrouping ?? false,
    groupWindowMs: overrides.groupWindowMs ?? 5000,
    interruptMode: overrides.interruptMode ?? 'never',
    interruptible: overrides.interruptible ?? true,
  };
}

function profileBody(p, overrides = {}) {
  return {
    name: overrides.name ?? p.name,
    enabled: overrides.enabled ?? p.enabled,
    language: overrides.language ?? p.language,
    theme: overrides.theme ?? p.theme,
    position: overrides.position ?? p.position,
    textAlign: overrides.textAlign ?? p.textAlign,
    maxQueueItems: overrides.maxQueueItems ?? p.maxQueueItems,
    maximumQueueAgeSeconds: overrides.maximumQueueAgeSeconds ?? p.maximumQueueAgeSeconds,
  };
}

// --- Twitch account connection (device-flow only) -------------------------

async function connectMetadataAccount(baseUrl, twitchState, suffix) {
  const start = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
  const attemptId = start.body.attemptId;
  await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
    return snap.body.state === 'polling' ? snap.body : false;
  }, POLL_TIMEOUT_MS, `the "${suffix}" device-flow attempt to reach "polling"`);

  const fakeUserId = `u_${RUN_ID}_${suffix}`;
  twitchState.users.set(fakeUserId, { id: fakeUserId, login: `streamer_${RUN_ID}_${suffix}`, displayName: `Streamer ${suffix}` });
  const device = [...twitchState.devices.values()].find((d) => d.userCode === start.body.userCode);
  device.userId = fakeUserId;
  device.authorized = true;

  const authorized = await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
    if (snap.body.state === 'error') throw new Error(`attempt error: ${snap.body.errorCode}`);
    return snap.body.state === 'authorized' ? snap.body : false;
  }, POLL_TIMEOUT_MS, `the "${suffix}" device-flow attempt to reach "authorized"`);

  return { accountId: authorized.connectedAccountId, fakeUserId };
}

async function enableEngagement(baseUrl, twitchState, wsState, accountId, fakeUserId) {
  const upgrade = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/engagement/authorize`);
  expect(upgrade.status === 202, 'the engagement permission-upgrade attempt starts', upgrade.body);
  await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${upgrade.body.attemptId}`);
    return snap.body.state === 'polling' ? snap.body : false;
  }, POLL_TIMEOUT_MS, 'the upgrade attempt to reach "polling"');
  const upgradeDevice = [...twitchState.devices.values()].find((d) => d.userCode === upgrade.body.userCode);
  upgradeDevice.userId = fakeUserId;
  upgradeDevice.authorized = true;
  await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${upgrade.body.attemptId}`);
    if (snap.body.state === 'error') throw new Error(`upgrade attempt error: ${snap.body.errorCode}`);
    return snap.body.state === 'authorized' ? snap.body : false;
  }, POLL_TIMEOUT_MS, 'the upgrade attempt to reach "authorized"');

  const connPromise = nextConnection(wsState);
  const enableResp = await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: true });
  expect(enableResp.status === 200 && enableResp.body.enabled === true, 'engagement enabled for the account', enableResp.body);
  const socket = await connPromise;
  sendWS(socket, welcomeEnvelope(`sess_${RUN_ID}`, 30));
  await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    return snap.body.state === 'connected' ? snap.body : false;
  }, POLL_TIMEOUT_MS, 'the connector to reach "connected"');
  return socket;
}

async function main() {
  console.log('Stage 12B advanced alert queue verification (grouping + preemption; local fakes only)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-alert-advqueue-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const twitchState = newTwitchFakeState();
  const wsState = newEventSubServerState();
  const oauthServer = createFakeOAuthServer(twitchState);
  const helixServer = createFakeHelixServer(twitchState);
  const eventSubServer = createFakeEventSubServer(wsState);

  let backend = null;
  let baseUrl;
  let env;

  try {
    step('Build the integration-only test server (go build -tags integration ./cmd/testserver)');
    const build = spawnCaptured('go-build', 'go', ['build', '-tags', 'integration', '-o', exePath, './cmd/testserver'], { cwd: SERVER_DIR });
    const buildExit = await new Promise((r) => {
      const timer = setTimeout(() => r(-1), BUILD_TIMEOUT_MS);
      build.child.on('exit', (code) => { clearTimeout(timer); r(code); });
    });
    expect(buildExit === 0, 'the integration test server built successfully', build.getOutput());

    step('Reserve dynamic loopback ports and start the fake Twitch OAuth, Helix and EventSub servers');
    const [backendPort, oauthPort, helixPort, eventSubPort] = await reservePorts(4);
    baseUrl = `http://127.0.0.1:${backendPort}`;
    await listen(oauthServer, oauthPort);
    await listen(helixServer, helixPort);
    await listen(eventSubServer, eventSubPort);
    pass(`backend :${backendPort}  fake oauth :${oauthPort}  fake helix :${helixPort}  fake eventsub :${eventSubPort}`);

    env = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(backendPort),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
      STREAMING_TREE_TEST_TWITCH_OAUTH_BASE_URL: `http://127.0.0.1:${oauthPort}`,
      STREAMING_TREE_TEST_TWITCH_API_BASE_URL: `http://127.0.0.1:${helixPort}`,
      STREAMING_TREE_TEST_TWITCH_EVENTSUB_BASE_URL: `http://127.0.0.1:${eventSubPort}`,
      STREAMING_TREE_TEST_TWITCH_EVENTSUB_RECONNECT_HOST: '127.0.0.1',
    };

    step('Start the backend');
    backend = await startBackend(exePath, env, baseUrl);

    step('Create an alert profile and connect + enable one Twitch account for real events');
    const createProfile = await request(baseUrl, 'POST', '/api/alert-profiles', { name: 'Advanced Queue' });
    expect(createProfile.status === 201, 'the profile was created', createProfile.body);
    let profile = createProfile.body;
    await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    const account = await connectMetadataAccount(baseUrl, twitchState, 'a');
    const socket = await enableEngagement(baseUrl, twitchState, wsState, account.accountId, account.fakeUserId);

    // --- Part 8 rejection matrix: grouping is only ever offered where the
    // real, closed capability matrix says it is safe -----------------------

    step('Reject AllowGrouping=true on a non-groupable event type (follow)');
    const badGroupType = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad group type', eventType: 'follow', allowGrouping: true, textTemplate: '{username} followed!',
    }));
    expect(badGroupType.status === 422, 'grouping on a non-groupable event type is rejected (422)', badGroupType.body);

    step('Reject AllowGrouping=true combined with ShowMessage=true on a RequiresNoMessage type (bits)');
    const badGroupMessage = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad group message', eventType: 'bits', allowGrouping: true, showMessage: true, textTemplate: '{username} cheered!',
    }));
    expect(badGroupMessage.status === 422, 'grouping with ShowMessage=true on Bits is rejected (422)', badGroupMessage.body);

    step('Reject AllowGrouping=true with a template referencing {message} on a RequiresNoMessage type (bits)');
    const badGroupTemplate = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad group template', eventType: 'bits', allowGrouping: true, textTemplate: '{username} said {message}',
    }));
    expect(badGroupTemplate.status === 422, 'grouping with a {message} placeholder on Bits is rejected (422)', badGroupTemplate.body);

    step('Reject AllowGrouping=true on gifted_subscription (structurally proves it can never merge with subscription_gift_batch)');
    const badGiftedSub = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad gifted sub group', eventType: 'gifted_subscription', allowGrouping: true, textTemplate: '{username} received a gift sub!',
    }));
    expect(badGiftedSub.status === 422, 'grouping on gifted_subscription is rejected (422) - it can never be Groupable', badGiftedSub.body);

    step('Reject a GroupWindowMS outside [1000, 30000], even with AllowGrouping=false (unconditional bound)');
    const badWindow = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad window', eventType: 'follow', groupWindowMs: 500, textTemplate: '{username} followed!',
    }));
    expect(badWindow.status === 422, 'an out-of-bounds groupWindowMs is rejected regardless of allowGrouping', badWindow.body);

    step('Reject an unrecognized interruptMode value');
    const badInterruptMode = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bad interrupt mode', eventType: 'follow', interruptMode: 'sometimes', textTemplate: '{username} followed!',
    }));
    expect(badInterruptMode.status === 422, 'an unrecognized interruptMode is rejected (422)', badInterruptMode.body);

    step('Confirm the event-type capability list correctly reports groupable + groupingRequiresHiddenMessage');
    const capabilities = await request(baseUrl, 'GET', '/api/alert-event-types');
    const bitsCap = capabilities.body.find((c) => c.eventType === 'bits');
    const followCap = capabilities.body.find((c) => c.eventType === 'follow');
    const redemptionCap = capabilities.body.find((c) => c.eventType === 'channel_point_redemption');
    expect(bitsCap.groupable === true && bitsCap.groupingRequiresHiddenMessage === true, 'bits is groupable and requires message hidden', bitsCap);
    expect(followCap.groupable === false, 'follow is not groupable', followCap);
    expect(redemptionCap.groupable === true && redemptionCap.groupingRequiresHiddenMessage === true, 'channel_point_redemption is groupable and requires message hidden', redemptionCap);

    // --- grouping rules actually used below --------------------------------

    step('Create a groupable Bits rule (same-actor quantity sum) and a groupable channel-point rule (same-actor same-subject count)');
    const bitsGroupRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bits group', eventType: 'bits', priority: 50, allowGrouping: true, groupWindowMs: 3000,
      showQuantity: true, textTemplate: '{username} cheered {quantity} bits x{groupCount} (bits-group)!',
    }))).body;
    const redemptionGroupRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Redemption group', eventType: 'channel_point_redemption', priority: 50, allowGrouping: true, groupWindowMs: 3000,
      textTemplate: '{username} redeemed x{groupCount} (redemption-group)!',
    }))).body;

    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);

    // --- real-event grouping: same actor within window merges truthfully -

    step('Two real Bits cheers from the same actor within the window merge into one queued alert with a truthfully summed quantity');
    const groupedBefore = await queueStatus(baseUrl, profile.id);
    sendWS(socket, cheerEvent('u_cheer_group_1', 'cheergroup1', 'CheerGroup1', 50));
    await waitForQueue(baseUrl, profile.id, (s) => findAlert(s, 'bits-group') !== undefined, QUEUE_TIMEOUT_MS, 'the first Bits cheer to be queued');
    sendWS(socket, cheerEvent('u_cheer_group_1', 'cheergroup1', 'CheerGroup1', 30));
    await waitForQueue(baseUrl, profile.id, (s) => findAlert(s, 'bits-group')?.quantity === 80, QUEUE_TIMEOUT_MS, 'the second cheer from the same actor to merge (quantity 50+30=80)');
    let st = await queueStatus(baseUrl, profile.id);
    const grouped = findAlert(st, 'bits-group');
    expect(grouped.groupCount === 2, 'the merged alert reports groupCount=2', grouped);
    expect(grouped.renderedText.includes('x2'), 'the re-rendered text reflects the updated groupCount via {groupCount}', grouped.renderedText);
    expect(st.totalGroupedMembers === groupedBefore.totalGroupedMembers + 1, 'totalGroupedMembers increments exactly once for the merged (2nd) member', st.totalGroupedMembers);
    expect(st.totalGroupsCreated === groupedBefore.totalGroupsCreated + 1, 'totalGroupsCreated increments exactly once, the moment groupCount first grows past 1', st.totalGroupsCreated);
    expect(st.totalEnqueued === groupedBefore.totalEnqueued + 1, 'a merged member never occupies a new queue slot / is never separately counted as enqueued', st.totalEnqueued);

    step('A third Bits cheer from a DIFFERENT actor never merges into the existing group');
    sendWS(socket, cheerEvent('u_cheer_group_2', 'cheergroup2', 'CheerGroup2', 999));
    await waitForQueue(baseUrl, profile.id, (s) => (s.nextQueued ?? []).filter((a) => a.renderedText.includes('bits-group')).length === 2,
      QUEUE_TIMEOUT_MS, 'a second, distinct Bits-group alert to appear (different actor)');
    st = await queueStatus(baseUrl, profile.id);
    const stillGrouped = findAlert(st, 'x2 (bits-group)') ?? st.nextQueued.find((a) => a.groupCount === 2);
    expect(stillGrouped !== undefined && stillGrouped.quantity === 80, 'the original group is untouched by the different actor\'s cheer', stillGrouped);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    step('Two real channel-point redemptions of the SAME reusable reward, same actor, merge by subject (no quantity involved)');
    const rewardId = mintToken('reward-group');
    sendWS(socket, redemptionEventWithReward('u_redeem_group_1', 'redeemgroup1', 'RedeemGroup1', rewardId, 'Group Reward', 'first'));
    await waitForQueue(baseUrl, profile.id, (s) => findAlert(s, 'redemption-group') !== undefined, QUEUE_TIMEOUT_MS, 'the first redemption to be queued');
    sendWS(socket, redemptionEventWithReward('u_redeem_group_1', 'redeemgroup1', 'RedeemGroup1', rewardId, 'Group Reward', 'second'));
    await waitForQueue(baseUrl, profile.id, (s) => findAlert(s, 'redemption-group')?.groupCount === 2, QUEUE_TIMEOUT_MS, 'the second same-reward redemption to merge');
    st = await queueStatus(baseUrl, profile.id);
    expect(findAlert(st, 'redemption-group').message === '' || findAlert(st, 'redemption-group').message === undefined,
      'the grouped redemption never carries a real user-input message (RequiresNoMessage)', findAlert(st, 'redemption-group'));

    step('A redemption of a DIFFERENT reward id from the same actor never merges into the reward-scoped group');
    const otherRewardId = mintToken('reward-other');
    sendWS(socket, redemptionEventWithReward('u_redeem_group_1', 'redeemgroup1', 'RedeemGroup1', otherRewardId, 'Other Reward', 'third'));
    await waitForQueue(baseUrl, profile.id, (s) => (s.nextQueued ?? []).filter((a) => a.renderedText.includes('redemption-group')).length === 2,
      QUEUE_TIMEOUT_MS, 'a second, distinct redemption-group alert to appear (different reward id)');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    // --- preemption rules ---------------------------------------------

    step('Create a low-priority interruptible rule, a strictly-higher-priority interrupting rule, and a protected (non-interruptible) rule');
    const lowInterruptibleRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Low interruptible', eventType: 'follow', priority: 40, durationMs: 6000, interruptible: true,
      textTemplate: '{username} followed (low-interruptible)!',
    }))).body;
    const lowProtectedRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Low protected', eventType: 'follow', priority: 40, durationMs: 6000, interruptible: false,
      textTemplate: '{username} followed (low-protected)!',
    }))).body;
    const highInterruptingBody = {
      name: 'High interrupting', eventType: 'raid', priority: 90, durationMs: 3000, showQuantity: true,
      interruptMode: 'lower_priority', textTemplate: '{username} raided with {quantity} (high-interrupting)!',
    };
    const highInterruptingRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody(highInterruptingBody))).body;

    // --- real-event preemption end-to-end, with the public SSE hide-then-
    // show protocol asserted directly ------------------------------------

    step('Subscribe to the public stream BEFORE any alert plays, then confirm real-event preemption: hide(reason=preempted) then show, no prior content leaked');
    const iterPre = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iterPre, 10_000, 'the initial reset');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);

    const preemptedBefore = (await queueStatus(baseUrl, profile.id)).totalPreempted;
    sendWS(socket, followEvent('u_follow_preempt_1', 'followpreempt1', 'FollowPreempt1'));
    const showLow = await waitUntil(async () => {
      const ev = await nextEvent(iterPre, 3000, 'the low-priority alert to show').catch(() => null);
      if (ev !== null && ev.event === 'alert.show' && ev.data.alert?.renderedText?.includes('low-interruptible')) return ev;
      return false;
    }, QUEUE_TIMEOUT_MS, 'the low-priority interruptible alert to become current on the public stream');
    const lowAlertId = showLow.data.alert.alertId;

    sendWS(socket, raidEvent('u_raid_preempt_1', 'raidpreempt1', 'RaidPreempt1', 7));
    const hideEv = await waitUntil(async () => {
      const ev = await nextEvent(iterPre, 3000, 'a hide event').catch(() => null);
      if (ev !== null && ev.event === 'alert.hide') return ev;
      return false;
    }, QUEUE_TIMEOUT_MS, 'the low-priority alert to be hidden with reason=preempted');
    expect(hideEv.data.alertId === lowAlertId, 'the hide payload identifies exactly the preempted alert', hideEv.data);
    expect(hideEv.data.reason === 'preempted', 'the hide reason is the closed value "preempted"', hideEv.data);
    expect(Object.keys(hideEv.data).sort().join(',') === 'alertId,paused,reason', 'the hide payload carries only paused/alertId/reason, never prior rendered content', hideEv.data);
    expect(!hideEv.raw.includes('low-interruptible') && !hideEv.raw.includes('FollowPreempt1'), 'the raw hide SSE chunk never repeats the outgoing alert\'s text or username', hideEv.raw);

    const showHigh = await waitUntil(async () => {
      const ev = await nextEvent(iterPre, 3000, 'the urgent raid alert to show').catch(() => null);
      if (ev !== null && ev.event === 'alert.show' && ev.data.alert?.renderedText?.includes('high-interrupting')) return ev;
      return false;
    }, QUEUE_TIMEOUT_MS, 'the interrupting raid alert to become current immediately (no exit-animation delay)');
    expect(showHigh.data.alert.alertId !== lowAlertId, 'the urgent alert has its own distinct alert id, not a resumed remainder of the old one', showHigh.data.alert);
    await iterPre.return();

    st = await queueStatus(baseUrl, profile.id);
    expect(st.totalPreempted === preemptedBefore + 1, 'totalPreempted incremented exactly once for the real preemption', st.totalPreempted);
    expect(st.current.renderedText.includes('high-interrupting'), 'the management queue status agrees: the interrupting raid alert is current', st.current);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 3200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    step('Equal priority never preempts (strictly-greater-only rule)');
    // highInterruptingRule is also a "raid" rule with no quantity bounds,
    // so it would match the very same raid event as equalPriorityRule
    // below - disabled for the duration of this test so only the
    // equal-priority candidate itself is under test.
    await request(baseUrl, 'PUT', `/api/alert-rules/${highInterruptingRule.id}`, ruleBody({ ...highInterruptingBody, enabled: false }));
    const equalPriorityRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Equal priority interrupting', eventType: 'raid', priority: 40, durationMs: 3000,
      interruptMode: 'lower_priority', showQuantity: true, textTemplate: '{username} raided (equal-priority)!',
    }))).body;
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket, followEvent('u_follow_equal_1', 'followequal1', 'FollowEqual1'));
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.renderedText?.includes('low-interruptible'), QUEUE_TIMEOUT_MS, 'the low-priority alert to become current');
    const equalPreemptedBefore = (await queueStatus(baseUrl, profile.id)).totalPreempted;
    sendWS(socket, raidEvent('u_raid_equal_1', 'raidequal1', 'RaidEqual1', 3));
    await new Promise((r) => setTimeout(r, 900));
    st = await queueStatus(baseUrl, profile.id);
    expect(st.totalPreempted === equalPreemptedBefore, 'an equal-priority candidate never preempts', st.totalPreempted);
    expect(st.current.renderedText.includes('low-interruptible'), 'the original low-priority alert is still current', st.current);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 5300));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);
    const deleteEqualPriority = await fetch(`${baseUrl}/api/alert-rules/${equalPriorityRule.id}`, { method: 'DELETE' });
    expect(deleteEqualPriority.status === 204, 'the equal-priority test rule was removed', deleteEqualPriority.status);
    await request(baseUrl, 'PUT', `/api/alert-rules/${highInterruptingRule.id}`, ruleBody(highInterruptingBody));

    step('A non-interruptible current alert is protected: a strictly-higher-priority candidate is queued instead of preempting');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket, followEvent('u_follow_protected_1', 'followprotected1', 'FollowProtected1'));
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.renderedText?.includes('low-protected'), QUEUE_TIMEOUT_MS, 'the protected low-priority alert to become current');
    const protectedPreemptedBefore = (await queueStatus(baseUrl, profile.id)).totalPreempted;
    sendWS(socket, raidEvent('u_raid_protected_1', 'raidprotected1', 'RaidProtected1', 9));
    await waitForQueue(baseUrl, profile.id, (s) => findAlert(s, 'high-interrupting') !== undefined, QUEUE_TIMEOUT_MS, 'the higher-priority raid to be queued behind the protected current alert, not preempt it');
    st = await queueStatus(baseUrl, profile.id);
    expect(st.totalPreempted === protectedPreemptedBefore, 'totalPreempted did not increase - the protected current alert was never interrupted', st.totalPreempted);
    expect(st.current.renderedText.includes('low-protected'), 'the protected alert is still current', st.current);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 6300));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    step('Pausing the queue disables preemption (a paused queue only ever grows the queue, never interrupts the current alert)');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket, followEvent('u_follow_pause_1', 'followpause1', 'FollowPause1'));
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.renderedText?.includes('low-interruptible'), QUEUE_TIMEOUT_MS, 'the low-priority alert to become current');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    const pausedPreemptedBefore = (await queueStatus(baseUrl, profile.id)).totalPreempted;
    sendWS(socket, raidEvent('u_raid_pause_1', 'raidpause1', 'RaidPause1', 11));
    await waitForQueue(baseUrl, profile.id, (s) => findAlert(s, 'high-interrupting') !== undefined, QUEUE_TIMEOUT_MS, 'the higher-priority raid to be queued, not preempt, while paused');
    st = await queueStatus(baseUrl, profile.id);
    expect(st.totalPreempted === pausedPreemptedBefore, 'totalPreempted did not increase while the queue was paused', st.totalPreempted);
    expect(st.current.renderedText.includes('low-interruptible'), 'the current alert kept playing undisturbed while paused', st.current);
    await new Promise((r) => setTimeout(r, 6300));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    // --- synthetic (Test Rule) alerts and preemption -----------------------

    step('A synthetic Test Rule alert never preempts a real current alert (queued instead)');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket, followEvent('u_follow_synth_1', 'followsynth1', 'FollowSynth1'));
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.renderedText?.includes('low-interruptible'), QUEUE_TIMEOUT_MS, 'a real low-priority alert to become current');
    const synthVsRealBefore = (await queueStatus(baseUrl, profile.id)).totalPreempted;
    const synthTest = await request(baseUrl, 'POST', `/api/alert-rules/${highInterruptingRule.id}/test`, {});
    expect(synthTest.status === 200 && synthTest.body.synthetic === true, 'the synthetic test alert was created', synthTest.body);
    await new Promise((r) => setTimeout(r, 500));
    st = await queueStatus(baseUrl, profile.id);
    expect(st.totalPreempted === synthVsRealBefore, 'a synthetic candidate never preempts a real current alert', st.totalPreempted);
    expect(st.current.renderedText.includes('low-interruptible') && st.current.synthetic === false, 'the real alert is still current, untouched', st.current);
    expect(findAlert(st, 'high-interrupting')?.synthetic === true, 'the synthetic candidate instead waits in the queue like a normal higher-priority item', st);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 5600));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    step('A synthetic Test Rule alert MAY preempt another synthetic current alert');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    const firstSynth = await request(baseUrl, 'POST', `/api/alert-rules/${lowInterruptibleRule.id}/test`, {});
    expect(firstSynth.status === 200, 'the first synthetic (low-priority, interruptible) alert was created', firstSynth.body);
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.alertId === firstSynth.body.alertId, QUEUE_TIMEOUT_MS, 'the first synthetic alert to become current');
    const synthVsSynthBefore = (await queueStatus(baseUrl, profile.id)).totalPreempted;
    const secondSynth = await request(baseUrl, 'POST', `/api/alert-rules/${highInterruptingRule.id}/test`, {});
    expect(secondSynth.status === 200, 'the second, higher-priority synthetic alert was created', secondSynth.body);
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.alertId === secondSynth.body.alertId, QUEUE_TIMEOUT_MS, 'the second synthetic alert to preempt the first synthetic current');
    st = await queueStatus(baseUrl, profile.id);
    expect(st.totalPreempted === synthVsSynthBefore + 1, 'totalPreempted increments for a synthetic-preempts-synthetic transition too', st.totalPreempted);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 3200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    // --- replay never preempts ---------------------------------------

    step('Replay Previous never preempts the current alert - it waits until the current alert finishes naturally');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket, followEvent('u_follow_replay_src', 'followreplaysrc', 'FollowReplaySrc'));
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.renderedText?.includes('low-interruptible'), QUEUE_TIMEOUT_MS, 'a real alert to become current and finish, becoming the replay snapshot');
    await new Promise((r) => setTimeout(r, 6200));
    await waitForQueue(baseUrl, profile.id, (s) => s.replayAvailable === true, QUEUE_TIMEOUT_MS, 'a replay-eligible snapshot to exist');

    sendWS(socket, followEvent('u_follow_replay_block', 'followreplayblock', 'FollowReplayBlock'));
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.renderedText?.includes('low-interruptible'), QUEUE_TIMEOUT_MS, 'a fresh low-priority alert to become current, blocking the queue');
    const replayPreemptedBefore = (await queueStatus(baseUrl, profile.id)).totalPreempted;
    const replayResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/replay-previous`);
    expect(replayResp.status === 200, 'replay-previous was accepted', replayResp.body);
    await new Promise((r) => setTimeout(r, 800));
    st = await queueStatus(baseUrl, profile.id);
    expect(st.totalPreempted === replayPreemptedBefore, 'queuing a replay never increments totalPreempted', st.totalPreempted);
    expect(st.current.replayed === false, 'the still-playing current alert is unaffected - the replay is waiting, not preempting', st.current);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 5600));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    await waitForQueue(baseUrl, profile.id, (s) => s.current?.replayed === true, QUEUE_TIMEOUT_MS, 'the replay to finally play once the queue is free, never having preempted anything');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 1200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);

    // --- restart: grouping/interruption rule fields persist, runtime
    // counters reset, no replay ------------------------------------------

    step('Restart the backend: allowGrouping/groupWindowMs/interruptMode/interruptible persist on every rule, but grouping/preemption counters reset');
    socket.destroy();
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);

    const rulesAfterRestart = await request(baseUrl, 'GET', `/api/alert-profiles/${profile.id}/rules`);
    const bitsGroupAfterRestart = rulesAfterRestart.body.rules.find((r) => r.id === bitsGroupRule.id);
    expect(bitsGroupAfterRestart.allowGrouping === true && bitsGroupAfterRestart.groupWindowMs === 3000,
      'the Bits grouping rule\'s allowGrouping/groupWindowMs survive a restart', bitsGroupAfterRestart);
    const redemptionGroupAfterRestart = rulesAfterRestart.body.rules.find((r) => r.id === redemptionGroupRule.id);
    expect(redemptionGroupAfterRestart.allowGrouping === true, 'the redemption grouping rule\'s allowGrouping survives a restart', redemptionGroupAfterRestart);
    const highInterruptingAfterRestart = rulesAfterRestart.body.rules.find((r) => r.id === highInterruptingRule.id);
    expect(highInterruptingAfterRestart.interruptMode === 'lower_priority', 'the interrupting rule\'s interruptMode survives a restart', highInterruptingAfterRestart);
    const lowProtectedAfterRestart = rulesAfterRestart.body.rules.find((r) => r.id === lowProtectedRule.id);
    expect(lowProtectedAfterRestart.interruptible === false, 'the protected rule\'s interruptible=false survives a restart', lowProtectedAfterRestart);

    const statusAfterRestart = await queueStatus(baseUrl, profile.id);
    expect(statusAfterRestart.totalGroupedMembers === 0 && statusAfterRestart.totalGroupsCreated === 0 && statusAfterRestart.totalPreempted === 0,
      'every Stage 12B runtime counter resets to 0 on restart - nothing is replayed', statusAfterRestart);
    expect(statusAfterRestart.current == null && statusAfterRestart.queuedCount === 0, 'the queue itself is empty after restart', statusAfterRestart);

    step('Confirm the pipeline still groups and preempts correctly after a restart (via a freshly re-authorized account)');
    const accountAfterRestart = await connectMetadataAccount(baseUrl, twitchState, 'b');
    const socketAfterRestart = await enableEngagement(baseUrl, twitchState, wsState, accountAfterRestart.accountId, accountAfterRestart.fakeUserId);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    sendWS(socketAfterRestart, cheerEvent('u_cheer_postrestart', 'cheerpostrestart', 'CheerPostRestart', 15));
    sendWS(socketAfterRestart, cheerEvent('u_cheer_postrestart', 'cheerpostrestart', 'CheerPostRestart', 5));
    await waitForQueue(baseUrl, profile.id, (s) => findAlert(s, 'bits-group')?.groupCount === 2, QUEUE_TIMEOUT_MS, 'grouping to still work correctly post-restart');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);
    socketAfterRestart.destroy();

    // --- public/private boundary: grouped-member identities are never
    // exposed publicly ----------------------------------------------------

    step('Confirm the public stream never leaks per-member identities of a grouped alert (only the aggregate groupCount/quantity)');
    const accountAfterRestart2 = await connectMetadataAccount(baseUrl, twitchState, 'c');
    const socket2 = await enableEngagement(baseUrl, twitchState, wsState, accountAfterRestart2.accountId, accountAfterRestart2.fakeUserId);
    const iterPub = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iterPub, 10_000, 'a fresh reset');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket2, cheerEvent('u_cheer_leak_1', 'cheerleak1', 'CheerLeak1', 20));
    sendWS(socket2, cheerEvent('u_cheer_leak_1', 'cheerleak1', 'CheerLeak1', 10));
    const groupedShow = await waitUntil(async () => {
      const ev = await nextEvent(iterPub, 3000, 'the grouped alert to show').catch(() => null);
      if (ev !== null && ev.event === 'alert.show' && ev.data.alert?.groupCount === 2) return ev;
      return false;
    }, QUEUE_TIMEOUT_MS, 'the grouped Bits alert to appear on the public stream with groupCount=2');
    expect(groupedShow.data.alert.quantity === 30, 'the public payload carries only the truthful aggregate quantity', groupedShow.data.alert);
    expect(!('members' in groupedShow.data.alert) && !('memberIds' in groupedShow.data.alert) && !('usernames' in groupedShow.data.alert),
      'the public payload never carries a member list of any kind', groupedShow.data.alert);
    await iterPub.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 3200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);
    socket2.destroy();

    // --- logs never leak secrets or real alert content --------------

    step('Confirm the backend\'s own stdout/stderr never leaks tokens or real (non-synthetic) alert content introduced in this run');
    const backendOutput = backend.getOutput();
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!backendOutput.includes(entry), 'no access token appears in backend logs', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!backendOutput.includes(entry), 'no refresh token appears in backend logs', undefined);
    }
    expect(!backendOutput.includes('CheerLeak1') && !backendOutput.includes('FollowPreempt1'), 'real alert usernames never appear in backend logs', undefined);

    step('Search every captured HTTP/SSE response body for real secret material');
    const haystack = secretScanChunks.join('\n');
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!haystack.includes(entry), 'access token is never present in captured HTTP/SSE bodies', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!haystack.includes(entry), 'refresh token is never present in captured HTTP/SSE bodies', undefined);
    }
    pass(`scanned ${haystack.length} bytes of HTTP/SSE bodies and ${backendOutput.length} bytes of backend output`);

    console.log('\nAll steps passed.');
  } catch (error) {
    if (backend !== null && process.env.STREAMING_TREE_VERIFY_DEBUG === '1') {
      console.error('\n--- backend output (debug) ---');
      console.error(backend.getOutput());
      console.error('--- end backend output ---\n');
    }
    throw error;
  } finally {
    await stopBackend(backend, baseUrl ?? '');
    for (const s of wsState.connections) {
      if (!s.destroyed) s.destroy();
    }
    await close(oauthServer);
    await close(helixServer);
    await close(eventSubServer);
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nAdvanced alert queue verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
