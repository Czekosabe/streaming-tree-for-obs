#!/usr/bin/env node
/**
 * Local, no-real-Twitch, no-real-OBS verification of the Stage 12A alert
 * subsystem: persisted alert profiles/rules, the provider-independent
 * matching engine, the bounded in-memory queue/playback runtime, the
 * management HTTP API, and the public Browser Source API (config + SSE).
 *
 * Mirrors scripts/verify-twitch-engagement.mjs's own fake-server harness
 * (fake OAuth/Helix/EventSub servers reproducing only the shapes this
 * application actually parses) but simplified: alerts only ever CONSUME
 * inbound EventSub notifications, so the outbound-chat-authorize step used
 * by scripts/verify-chat-automation.mjs's connectFullAccount is never
 * needed here.
 *
 * This is a representative subset of the Stage 12A task's own ~44-item
 * verification list (Part 55), not the complete enumeration - see
 * docs/progress.md's Stage 12A test-verification entry for exactly which
 * scenarios are covered here versus by a specific named Go test instead
 * (internal/alerts/{matcher,queue,playback,manager}_test.go,
 * internal/domain/alerts/{service,validation}_test.go,
 * internal/httpapi/alerts_test.go).
 *
 * Every token, device code, user code and client ID used here is an
 * obviously-fake string generated for this run only. No real Twitch
 * account, application, or network request to Twitch is ever involved, and
 * no OBS Browser Source is ever opened - only this script's own HTTP/SSE
 * client.
 *
 * Usage: node scripts/verify-alerts.mjs
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
          // A restarted backend re-derives its access token from the
          // persisted refresh token before reconnecting an already-enabled
          // engagement connector - without this grant, a restart-then-
          // reconnect scenario would hang forever waiting for "connected".
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

// --- EventSub notification builders for the 8 alert-capable event types
// (field names taken directly from apps/server/internal/provider/twitch/
// eventsub_normalize.go's own wire* structs) -----------------------------

let msgSeq = 0;
function nextMsgId() {
  msgSeq += 1;
  return `msg_${RUN_ID}_${msgSeq}`;
}

function followEvent(userId, login, name) {
  return notificationEnvelope('channel.follow', { user_id: userId, user_login: login, user_name: name, followed_at: new Date().toISOString() }, nextMsgId());
}

function subscribeEvent(userId, login, name, { isGift = false, tier = '1000' } = {}) {
  return notificationEnvelope('channel.subscribe', { user_id: userId, user_login: login, user_name: name, tier, is_gift: isGift }, nextMsgId());
}

function subscriptionGiftEvent(userId, login, name, total, { isAnonymous = false, tier = '1000' } = {}) {
  return notificationEnvelope('channel.subscription.gift', { user_id: userId, user_login: login, user_name: name, total, tier, is_anonymous: isAnonymous }, nextMsgId());
}

function subscriptionMessageEvent(userId, login, name, text, { tier = '1000', cumulativeMonths = 6, durationMonths = 1 } = {}) {
  return notificationEnvelope('channel.subscription.message', {
    user_id: userId, user_login: login, user_name: name, tier,
    message: { text }, cumulative_months: cumulativeMonths, duration_months: durationMonths,
  }, nextMsgId());
}

function cheerEvent(userId, login, name, bits, { isAnonymous = false, message = 'Cheer!' } = {}) {
  return notificationEnvelope('channel.cheer', { user_id: userId, user_login: login, user_name: name, is_anonymous: isAnonymous, message, bits }, nextMsgId());
}

function raidEvent(fromUserId, fromLogin, fromName, viewers) {
  return notificationEnvelope('channel.raid', { from_broadcaster_user_id: fromUserId, from_broadcaster_user_login: fromLogin, from_broadcaster_user_name: fromName, viewers }, nextMsgId());
}

function redemptionEvent(userId, login, name, rewardTitle, userInput) {
  return notificationEnvelope('channel.channel_points_custom_reward_redemption.add', {
    id: mintToken('redeem'), user_id: userId, user_login: login, user_name: name, user_input: userInput,
    reward: { id: mintToken('reward'), title: rewardTitle, cost: 100 }, redeemed_at: new Date().toISOString(),
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

// --- Twitch account connection (device-flow only - alerts never send
// outbound chat, so the outbound-chat-authorize step is never needed) ----

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
  console.log('Stage 12A alerts verification (local fakes only, no real Twitch, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-alerts-'));
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

    step('Start the backend and confirm the alert subsystem starts empty');
    backend = await startBackend(exePath, env, baseUrl);
    const emptyList = await request(baseUrl, 'GET', '/api/alert-profiles');
    expect(emptyList.status === 200 && Array.isArray(emptyList.body) && emptyList.body.length === 0, 'no alert profiles exist yet', emptyList.body);

    // --- profile creation, defaults, and public config -------------------

    step('Create an alert profile and confirm its safe defaults persist');
    const createProfile = await request(baseUrl, 'POST', '/api/alert-profiles', { name: 'Main Alerts' });
    expect(createProfile.status === 201, 'the profile was created', createProfile.body);
    let profile1 = createProfile.body;
    expect(profile1.enabled === true && profile1.theme === 'minimal' && profile1.position === 'bottom' &&
      profile1.textAlign === 'center' && profile1.language === 'en' && profile1.maxQueueItems === 100 &&
      profile1.maximumQueueAgeSeconds === 120, 'the new profile has the documented safe defaults', profile1);
    expect(typeof profile1.publicSlug === 'string' && profile1.publicSlug.length >= 16, 'the profile has a high-entropy public slug', profile1.publicSlug);

    const listAfterCreate = await request(baseUrl, 'GET', '/api/alert-profiles');
    expect(listAfterCreate.body.some((p) => p.id === profile1.id), 'the created profile is persisted and listed', listAfterCreate.body);

    step('Confirm an unknown public slug answers with safe defaults, not a hard error or profile data');
    const unknownConfig = await request(baseUrl, 'GET', '/api/public/alert-profiles/does-not-exist/config');
    expect(unknownConfig.status === 200, 'an unknown slug still answers 200 (never leaks existence via a 404)', unknownConfig.status);
    expect(Object.keys(unknownConfig.body).sort().join(',') === 'language,position,schemaVersion,textAlign,theme',
      'the public config exposes only its 5 documented fields, nothing else', unknownConfig.body);

    step('Confirm the real public config reflects this profile\'s own presentation fields');
    const realConfig = await request(baseUrl, 'GET', `/api/public/alert-profiles/${profile1.publicSlug}/config`);
    expect(realConfig.status === 200 && realConfig.body.theme === 'minimal' && realConfig.body.language === 'en', 'the public config matches the profile', realConfig.body);

    step('Confirm a fresh public SSE connection\'s first event is a complete reset with no historical/queue content');
    const iter1 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile1.publicSlug}/stream`);
    const firstEvent = await nextEvent(iter1, 10_000, 'the first SSE event');
    expect(firstEvent.event === 'alert.reset', 'the first event is alert.reset', firstEvent);
    expect(firstEvent.data.alert === null && firstEvent.data.paused === false, 'the reset shows no current alert and paused=false', firstEvent.data);
    expect(!('nextQueued' in firstEvent.data) && !('queuedCount' in firstEvent.data), 'the public reset never carries queue-management fields', firstEvent.data);
    await iter1.return();

    // --- rule persistence + capability-driven validation + overlap -------

    step('Create a Follow rule and confirm it persists with no overlap warnings');
    const followRuleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Follow', eventType: 'follow', priority: 50, textTemplate: '{username} just followed!',
    }));
    expect(followRuleResp.status === 201, 'the follow rule was created', followRuleResp.body);
    const followRule = followRuleResp.body;

    step('Reject a condition the event type\'s capability does not support (minimumQuantity on follow)');
    const badQuantity = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Bad', eventType: 'follow', minimumQuantity: 5, textTemplate: '{username} followed!',
    }));
    expect(badQuantity.status === 422, 'a quantity threshold on a follow rule is rejected (422)', badQuantity.body);

    step('Create two non-overlapping Bits quantity tiers and confirm no overlap warning');
    const bitsLowResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Bits low', eventType: 'bits', priority: 50, minimumQuantity: 1, maximumQuantity: 99,
      showQuantity: true, textTemplate: '{username} cheered {quantity} bits (bits-low)!',
    }));
    const bitsHighResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Bits high', eventType: 'bits', priority: 50, minimumQuantity: 100, maximumQuantity: null,
      showQuantity: true, textTemplate: '{username} cheered {quantity} bits (bits-high)!',
    }));
    const bitsLowRule = bitsLowResp.body;
    const bitsHighRule = bitsHighResp.body;
    const rulesAfterTiers = await request(baseUrl, 'GET', `/api/alert-profiles/${profile1.id}/rules`);
    expect(rulesAfterTiers.body.overlapWarnings.length === 0, 'two non-overlapping Bits tiers produce no overlap warning', rulesAfterTiers.body.overlapWarnings);

    step('Create a third, overlapping Bits rule and confirm both sides of the overlap are warned, then remove it');
    const bitsOverlapResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Bits overlap', eventType: 'bits', priority: 50, minimumQuantity: 50, maximumQuantity: 150,
      showQuantity: true, textTemplate: '{username} cheered {quantity} bits (bits-overlap)!',
    }));
    const rulesWithOverlap = await request(baseUrl, 'GET', `/api/alert-profiles/${profile1.id}/rules`);
    expect(rulesWithOverlap.body.overlapWarnings.length >= 2, 'the overlapping rule produces a warning naming both sides', rulesWithOverlap.body.overlapWarnings);
    const deleteOverlap = await fetch(`${baseUrl}/api/alert-rules/${bitsOverlapResp.body.id}`, { method: 'DELETE' });
    expect(deleteOverlap.status === 204, 'the overlapping rule was deleted', deleteOverlap.status);
    const rulesAfterCleanup = await request(baseUrl, 'GET', `/api/alert-profiles/${profile1.id}/rules`);
    expect(rulesAfterCleanup.body.overlapWarnings.length === 0, 'the overlap warning clears once the overlapping rule is gone', rulesAfterCleanup.body.overlapWarnings);

    step('Create Raid (high priority), Redemption (low priority), and the 4 subscription-family rules');
    const raidRuleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Raid', eventType: 'raid', priority: 100, showQuantity: true, textTemplate: '{username} raided with {quantity} viewers (raid)!',
    }));
    const redemptionRuleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Redemption', eventType: 'channel_point_redemption', priority: 10, showMessage: true,
      textTemplate: '{username} redeemed {rewardTitle} (redemption)!',
    }));
    const subscriptionRuleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Subscription', eventType: 'subscription', textTemplate: '{username} subscribed (sub-new)!',
    }));
    const resubscriptionRuleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Resubscription', eventType: 'resubscription', showMessage: true, textTemplate: '{username} resubscribed (resub)!',
    }));
    const giftedSubRuleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Gifted sub recipient', eventType: 'gifted_subscription', textTemplate: '{username} received a gift sub (gift-recipient)!',
    }));
    const giftBatchRuleResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Gift batch', eventType: 'subscription_gift_batch', showQuantity: true, textTemplate: '{username} gifted {quantity} subs (gift-batch)!',
    }));
    for (const r of [raidRuleResp, redemptionRuleResp, subscriptionRuleResp, resubscriptionRuleResp, giftedSubRuleResp, giftBatchRuleResp]) {
      expect(r.status === 201, `rule "${r.body?.name}" was created`, r.body);
    }
    const raidRule = raidRuleResp.body;
    const redemptionRule = redemptionRuleResp.body;

    // --- connect two Twitch accounts (metadata only) + enable engagement
    // on the first ------------------------------------------------------

    step('Configure the Twitch Client ID and connect two accounts (metadata scope only)');
    await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    const accountA = await connectMetadataAccount(baseUrl, twitchState, 'a');
    const accountB = await connectMetadataAccount(baseUrl, twitchState, 'b');
    expect(accountA.accountId !== accountB.accountId, 'the two connected accounts have distinct ids', [accountA.accountId, accountB.accountId]);

    step('Upgrade account A to engagement scopes and enable its connector (dials the fake EventSub server)');
    const socket = await enableEngagement(baseUrl, twitchState, wsState, accountA.accountId, accountA.fakeUserId);

    // --- a real event becomes a real alert on the public stream ----------

    step('Pause the queue, send a real follow notification, and confirm it becomes a queued alert');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);
    sendWS(socket, followEvent('u_follower_1', 'a_follower_1', 'AFollower1'));
    await waitForQueue(baseUrl, profile1.id, (st) => findAlert(st, 'just followed') !== undefined, QUEUE_TIMEOUT_MS, 'the follow alert to be queued');
    let st = await queueStatus(baseUrl, profile1.id);
    expect(st.totalEnqueued >= 1, 'totalEnqueued increased for the real follow event', st.totalEnqueued);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    step('Confirm the same alert reaches the public SSE stream as a "show" revision');
    const iter2 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile1.publicSlug}/stream`);
    await nextEvent(iter2, 10_000, 'the reconnect reset'); // fresh connection's own synthetic reset
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/resume`);
    sendWS(socket, followEvent('u_follower_2', 'a_follower_2', 'AFollower2'));
    const shown = await waitUntil(async () => {
      const ev = await nextEvent(iter2, 3000, 'a show event').catch(() => null);
      if (ev !== null && ev.event === 'alert.show' && ev.data.alert?.renderedText?.includes('just followed')) return ev;
      return false;
    }, 10_000, 'the follow alert to appear on the public stream as alert.show');
    expect(shown.data.alert.synthetic === false, 'the real alert is not marked synthetic', shown.data.alert);
    await iter2.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 1200)); // let the short-duration alert finish naturally
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    // --- account filtering -------------------------------------------

    step('Create a Follow rule filtered to account B only, and confirm account A\'s events never match it');
    const acctBOnlyResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/rules`, ruleBody({
      name: 'Follow (account B only)', eventType: 'follow', accounts: [accountB.accountId],
      textTemplate: '{username} followed (acct-b-only)!',
    }));
    expect(acctBOnlyResp.status === 201, 'the account-filtered rule was created', acctBOnlyResp.body);
    const acctBOnlyRule = acctBOnlyResp.body;

    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);
    sendWS(socket, followEvent('u_follower_3', 'a_follower_3', 'AFollower3'));
    await waitForQueue(baseUrl, profile1.id, (s) => findAlert(s, 'just followed') !== undefined, QUEUE_TIMEOUT_MS, 'the general follow rule to match');
    st = await queueStatus(baseUrl, profile1.id);
    expect(findAlert(st, 'acct-b-only') === undefined, 'the account-B-only rule never matched account A\'s event', st);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    step('Widen the rule to include account A, and confirm it now matches');
    const acctBWidened = await request(baseUrl, 'PUT', `/api/alert-rules/${acctBOnlyRule.id}`, ruleBody({
      name: 'Follow (account B only)', eventType: 'follow', accounts: [accountB.accountId, accountA.accountId],
      textTemplate: '{username} followed (acct-b-only)!',
    }));
    expect(acctBWidened.status === 200, 'the rule was updated', acctBWidened.body);
    sendWS(socket, followEvent('u_follower_4', 'a_follower_4', 'AFollower4'));
    await waitForQueue(baseUrl, profile1.id, (s) => findAlert(s, 'acct-b-only') !== undefined, QUEUE_TIMEOUT_MS, 'the widened rule to now match account A');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);
    const deleteAcctBOnly = await fetch(`${baseUrl}/api/alert-rules/${acctBOnlyRule.id}`, { method: 'DELETE' });
    expect(deleteAcctBOnly.status === 204, 'the account-filter test rule was removed', deleteAcctBOnly.status);

    // --- Bits tier selection (no cross-tier triggering) -------------------

    step('Confirm a Bits cheer only triggers its own quantity tier, never both');
    sendWS(socket, cheerEvent('u_cheer_1', 'cheerer1', 'Cheerer1', 50));
    await waitForQueue(baseUrl, profile1.id, (s) => findAlert(s, 'bits-low') !== undefined, QUEUE_TIMEOUT_MS, 'the low Bits tier to match 50 bits');
    st = await queueStatus(baseUrl, profile1.id);
    expect(findAlert(st, 'bits-high') === undefined, '50 bits never also triggers the high tier', st);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    sendWS(socket, cheerEvent('u_cheer_2', 'cheerer2', 'Cheerer2', 500));
    await waitForQueue(baseUrl, profile1.id, (s) => findAlert(s, 'bits-high') !== undefined, QUEUE_TIMEOUT_MS, 'the high Bits tier to match 500 bits');
    st = await queueStatus(baseUrl, profile1.id);
    expect(findAlert(st, 'bits-low') === undefined, '500 bits never also triggers the low tier', st);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    // --- raid + redemption reach an alert ---------------------------------

    step('Confirm raid and channel-point-redemption notifications become alerts');
    sendWS(socket, raidEvent('u_raider_1', 'raider1', 'Raider1', 42));
    sendWS(socket, redemptionEvent('u_redeemer_1', 'redeemer1', 'Redeemer1', 'Test Reward', 'hello'));
    await waitForQueue(baseUrl, profile1.id, (s) => findAlert(s, '(raid)') !== undefined && findAlert(s, '(redemption)') !== undefined,
      QUEUE_TIMEOUT_MS, 'both the raid and redemption alerts to be queued');
    pass('both the raid and channel-point-redemption alerts were queued');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    // --- subscription / resubscription / gifted sub / gift batch stay
    // distinct ------------------------------------------------------------

    step('Confirm subscription, resubscription, gift-recipient, and gift-batch events stay distinct alerts');
    sendWS(socket, subscribeEvent('u_sub_1', 'subscriber1', 'Subscriber1', { isGift: false }));
    sendWS(socket, subscriptionMessageEvent('u_resub_1', 'resubscriber1', 'Resubscriber1', 'been here a while'));
    sendWS(socket, subscriptionGiftEvent('u_gifter_1', 'gifter1', 'Gifter1', 3));
    sendWS(socket, subscribeEvent('u_giftrecip_1', 'giftrecipient1', 'GiftRecipient1', { isGift: true }));
    await waitForQueue(baseUrl, profile1.id, (s) =>
      findAlert(s, 'sub-new') !== undefined && findAlert(s, 'resub') !== undefined &&
      findAlert(s, 'gift-batch') !== undefined && findAlert(s, 'gift-recipient') !== undefined,
      QUEUE_TIMEOUT_MS, 'all four subscription-family alerts to be queued distinctly');
    st = await queueStatus(baseUrl, profile1.id);
    expect(findAlert(st, 'gift-batch').renderedText.includes('3'), 'the gift batch preserves its total gifted count', findAlert(st, 'gift-batch'));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    // --- Test Rule: synthetic alert through the real queue ---------------

    step('POST /api/alert-rules/{id}/test creates one synthetic alert through the real queue');
    const beforeEvents = await request(baseUrl, 'GET', '/api/engagement/events?limit=200');
    const testResp = await request(baseUrl, 'POST', `/api/alert-rules/${followRule.id}/test`, {});
    expect(testResp.status === 200 && testResp.body.synthetic === true, 'the test rule endpoint returns a synthetic alert summary', testResp.body);
    st = await queueStatus(baseUrl, profile1.id);
    expect(st.totalSynthetic >= 1, 'the synthetic test alert is counted separately (totalSynthetic)', st.totalSynthetic);
    const afterEvents = await request(baseUrl, 'GET', '/api/engagement/events?limit=200');
    expect(afterEvents.body.items.length === beforeEvents.body.items.length, 'testing a rule never publishes a real Engagement Event Bus event', {
      before: beforeEvents.body.items.length, after: afterEvents.body.items.length,
    });
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    // --- priority ordering + FIFO, independent of call/insertion order ---

    step('Confirm the queue orders strictly by priority (desc), regardless of the order alerts were enqueued');
    await request(baseUrl, 'POST', `/api/alert-rules/${redemptionRule.id}/test`, {}); // priority 10
    await request(baseUrl, 'POST', `/api/alert-rules/${followRule.id}/test`, {}); // priority 50
    await request(baseUrl, 'POST', `/api/alert-rules/${raidRule.id}/test`, {}); // priority 100
    st = await waitForQueue(baseUrl, profile1.id, (s) => (s.nextQueued?.length ?? 0) >= 3, QUEUE_TIMEOUT_MS, 'all three test alerts to be queued (paused)');
    expect(st.nextQueued[0].renderedText.includes('(raid)'), 'the highest-priority alert (raid, 100) sorts first', st.nextQueued);
    expect(st.nextQueued[1].renderedText.includes('just followed'), 'the middle-priority alert (follow, 50) sorts second', st.nextQueued);
    expect(st.nextQueued[2].renderedText.includes('(redemption)'), 'the lowest-priority alert (redemption, 10) sorts last', st.nextQueued);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    // --- expiration --------------------------------------------------

    step('Confirm a queued alert older than maximumQueueAgeSeconds is discarded, never played, and never blocks the queue forever');
    profile1 = (await request(baseUrl, 'PUT', `/api/alert-profiles/${profile1.id}`, profileBody(profile1, { maximumQueueAgeSeconds: 5 }))).body;
    await request(baseUrl, 'POST', `/api/alert-rules/${followRule.id}/test`, {});
    await new Promise((r) => setTimeout(r, 5600));
    const expiredBefore = (await queueStatus(baseUrl, profile1.id)).totalExpired;
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/resume`);
    await waitUntil(async () => {
      const s = await queueStatus(baseUrl, profile1.id);
      return s.totalExpired > expiredBefore ? s : false;
    }, QUEUE_TIMEOUT_MS, 'the stale queued alert to be discarded as expired');
    st = await queueStatus(baseUrl, profile1.id);
    expect(st.current == null && st.queuedCount === 0, 'the expired alert was never promoted to "current"', st);
    profile1 = (await request(baseUrl, 'PUT', `/api/alert-profiles/${profile1.id}`, profileBody(profile1, { maximumQueueAgeSeconds: 30 }))).body;
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);

    // --- pause policy: the current alert always finishes normally, but
    // the queue does not advance again until resume -----------------------

    step('Confirm the documented pause policy: the current alert finishes normally, but the queue does not advance while paused');
    await request(baseUrl, 'POST', `/api/alert-rules/${raidRule.id}/test`, {});
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/resume`);
    await waitForQueue(baseUrl, profile1.id, (s) => s.current != null, QUEUE_TIMEOUT_MS, 'the raid test alert to become current');
    await request(baseUrl, 'POST', `/api/alert-rules/${followRule.id}/test`, {}); // queued behind it
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);
    await waitForQueue(baseUrl, profile1.id, (s) => s.current == null, QUEUE_TIMEOUT_MS, 'the current alert to finish naturally even while paused');
    await new Promise((r) => setTimeout(r, 500));
    st = await queueStatus(baseUrl, profile1.id);
    expect(st.current == null && st.queuedCount === 1, 'the queue does not advance to the next item while paused', st);

    step('Resuming immediately promotes the next queued item');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/resume`);
    await waitForQueue(baseUrl, profile1.id, (s) => s.current != null, QUEUE_TIMEOUT_MS, 'the queue to advance again after resume');

    // --- skip current --------------------------------------------------

    step('Skip Current removes the playing alert immediately, counts it as manually skipped, and advances');
    const skippedBefore = (await queueStatus(baseUrl, profile1.id)).totalManuallySkipped;
    const skipResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/skip-current`);
    expect(skipResp.status === 200, 'skip-current succeeded', skipResp.body);
    expect(skipResp.body.totalManuallySkipped === skippedBefore + 1, 'the skip is counted as manually skipped, not played', skipResp.body);

    // --- replay previous: never recreates a real Engagement Event -------

    step('Replay Previous re-shows the last completed alert without creating a new Engagement Event Bus event');
    await waitForQueue(baseUrl, profile1.id, (s) => s.replayAvailable === true, QUEUE_TIMEOUT_MS, 'a replay-eligible snapshot to exist');
    const eventsBeforeReplay = await request(baseUrl, 'GET', '/api/engagement/events?limit=200');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);
    const replayResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/replay-previous`);
    expect(replayResp.status === 200, 'replay-previous succeeded', replayResp.body);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/resume`);
    await waitForQueue(baseUrl, profile1.id, (s) => s.current?.replayed === true, QUEUE_TIMEOUT_MS, 'the replayed alert to become current, marked replayed');
    const eventsAfterReplay = await request(baseUrl, 'GET', '/api/engagement/events?limit=200');
    expect(eventsAfterReplay.body.items.length === eventsBeforeReplay.body.items.length, 'replay never creates a new Engagement Event Bus event', {
      before: eventsBeforeReplay.body.items.length, after: eventsAfterReplay.body.items.length,
    });
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 1200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);

    // --- clear queue: only queued items, never the current alert ---------

    step('Clear Queue removes only queued items, is a separate action from Skip Current, and never counts as played');
    await request(baseUrl, 'POST', `/api/alert-rules/${followRule.id}/test`, {});
    await request(baseUrl, 'POST', `/api/alert-rules/${raidRule.id}/test`, {});
    const playedBeforeClear = (await queueStatus(baseUrl, profile1.id)).totalPlayed;
    const clearResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);
    expect(clearResp.status === 200 && clearResp.body.queuedCount === 0, 'the queue is empty after clear', clearResp.body);
    expect(clearResp.body.totalPlayed === playedBeforeClear, 'cleared items are never counted as played', clearResp.body);

    // --- capacity policy ---------------------------------------------

    step('Confirm the deterministic capacity policy: reject an equal/lower priority candidate, evict for a strictly higher one');
    profile1 = (await request(baseUrl, 'PUT', `/api/alert-profiles/${profile1.id}`, profileBody(profile1, { maxQueueItems: 1 }))).body;
    const first = await request(baseUrl, 'POST', `/api/alert-rules/${followRule.id}/test`, {}); // priority 50, accepted
    expect(first.status === 200, 'the first item is accepted below/at capacity', first.body);
    const second = await request(baseUrl, 'POST', `/api/alert-rules/${bitsLowRule.id}/test`, {}); // priority 50, at capacity, not strictly higher -> rejected
    expect(second.status === 429, 'an equal-priority candidate is rejected once the queue is full', second.body);
    const droppedBefore = (await queueStatus(baseUrl, profile1.id)).totalCapacityDropped;
    const third = await request(baseUrl, 'POST', `/api/alert-rules/${raidRule.id}/test`, {}); // priority 100, strictly higher -> evicts
    expect(third.status === 200, 'a strictly-higher-priority candidate is accepted, evicting the worst queued item', third.body);
    st = await queueStatus(baseUrl, profile1.id);
    expect(st.totalCapacityDropped === droppedBefore + 1, 'the evicted item is counted as capacity-dropped', st.totalCapacityDropped);
    expect(st.nextQueued.length === 1 && st.nextQueued[0].renderedText.includes('(raid)'), 'only the higher-priority raid item remains queued', st.nextQueued);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);
    profile1 = (await request(baseUrl, 'PUT', `/api/alert-profiles/${profile1.id}`, profileBody(profile1, { maxQueueItems: 100 }))).body;

    // --- profile disable/enable isolation -----------------------------

    step('Disabling a profile hides its current alert, empties its queue, and stops accepting new alerts');
    profile1 = (await request(baseUrl, 'PUT', `/api/alert-profiles/${profile1.id}`, profileBody(profile1, { enabled: false }))).body;
    st = await queueStatus(baseUrl, profile1.id);
    expect(st.enabled === false && st.current == null && st.queuedCount === 0, 'the disabled profile\'s queue is empty and nothing is current', st);
    const disabledConfig = await request(baseUrl, 'GET', `/api/public/alert-profiles/${profile1.publicSlug}/config`);
    expect(disabledConfig.body.theme === 'minimal' && disabledConfig.status === 200, 'a disabled profile\'s public config falls back to safe defaults', disabledConfig.body);
    const enqueuedBeforeDisabled = st.totalEnqueued;
    sendWS(socket, followEvent('u_follower_disabled', 'a_follower_disabled', 'ADisabled'));
    await new Promise((r) => setTimeout(r, 800));
    st = await queueStatus(baseUrl, profile1.id);
    expect(st.totalEnqueued === enqueuedBeforeDisabled && st.queuedCount === 0, 'an event that arrives while disabled is never enqueued', st);

    step('Re-enabling a profile begins empty - events that arrived while disabled are never replayed');
    profile1 = (await request(baseUrl, 'PUT', `/api/alert-profiles/${profile1.id}`, profileBody(profile1, { enabled: true }))).body;
    await new Promise((r) => setTimeout(r, 800));
    st = await queueStatus(baseUrl, profile1.id);
    expect(st.queuedCount === 0 && st.current == null, 're-enabling never replays the event that arrived while disabled', st);

    // --- two profiles are isolated ------------------------------------

    step('Two alert profiles are fully isolated: one real event reaches both queues independently');
    const profile2Resp = await request(baseUrl, 'POST', '/api/alert-profiles', { name: 'Secondary' });
    let profile2 = profile2Resp.body;
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile2.id}/rules`, ruleBody({
      name: 'Follow', eventType: 'follow', textTemplate: '{username} followed (profile-2)!',
    }));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/pause`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile2.id}/queue/pause`);
    sendWS(socket, followEvent('u_follower_iso', 'a_follower_iso', 'AIso'));
    await waitForQueue(baseUrl, profile1.id, (s) => findAlert(s, 'just followed') !== undefined, QUEUE_TIMEOUT_MS, 'profile 1 to receive its own alert');
    await waitForQueue(baseUrl, profile2.id, (s) => findAlert(s, 'profile-2') !== undefined, QUEUE_TIMEOUT_MS, 'profile 2 to independently receive its own alert');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile2.id}/queue/clear`);
    const p1AfterP2Clear = await queueStatus(baseUrl, profile1.id);
    expect(p1AfterP2Clear.queuedCount === 1, 'clearing profile 2\'s queue never affects profile 1\'s own queue', p1AfterP2Clear);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/resume`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile2.id}/queue/resume`);

    // --- public slug rotation invalidates the old URL ---------------------

    step('Rotating a profile\'s public slug immediately invalidates the old one');
    profile2 = (await request(baseUrl, 'PUT', `/api/alert-profiles/${profile2.id}`, profileBody(profile2, { theme: 'large' }))).body;
    const oldSlug = profile2.publicSlug;
    const rotateResp = await request(baseUrl, 'POST', `/api/alert-profiles/${profile2.id}/rotate-public-slug`);
    expect(rotateResp.status === 200 && rotateResp.body.publicSlug !== oldSlug, 'rotation produces a new, different public slug', rotateResp.body);
    const newSlug = rotateResp.body.publicSlug;
    const oldSlugConfig = await request(baseUrl, 'GET', `/api/public/alert-profiles/${oldSlug}/config`);
    expect(oldSlugConfig.body.theme === 'minimal', 'the old slug no longer resolves to the real profile (falls back to safe defaults)', oldSlugConfig.body);
    const newSlugConfig = await request(baseUrl, 'GET', `/api/public/alert-profiles/${newSlug}/config`);
    expect(newSlugConfig.body.theme === 'large', 'the new slug resolves to the real, current profile settings', newSlugConfig.body);

    // --- restart: definitions persist, runtime resets, no replay ---------

    step('Restart the backend: profiles/rules persist, but every runtime counter and queue resets with no replay');
    socket.destroy();
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);

    const profilesAfterRestart = await request(baseUrl, 'GET', '/api/alert-profiles');
    expect(profilesAfterRestart.body.some((p) => p.id === profile1.id) && profilesAfterRestart.body.some((p) => p.id === profile2.id),
      'both profiles survive a restart', profilesAfterRestart.body);
    const rulesAfterRestart = await request(baseUrl, 'GET', `/api/alert-profiles/${profile1.id}/rules`);
    expect(rulesAfterRestart.body.rules.length >= 8, 'profile 1\'s rules survive a restart', rulesAfterRestart.body.rules.length);
    const statusAfterRestart = await queueStatus(baseUrl, profile1.id);
    expect(statusAfterRestart.totalEnqueued === 0 && statusAfterRestart.totalPlayed === 0 && statusAfterRestart.current == null && statusAfterRestart.queuedCount === 0,
      'every runtime counter and the queue reset on restart - nothing is replayed', statusAfterRestart);

    // Note: this integration test's fake credential store
    // (internal/secrets/secretstest, the -tags integration binary's
    // documented substitute for the OS keychain - see cmd/testserver/
    // main.go's own startup warning) is in-memory only and is deliberately
    // never persisted, so a genuine same-account auto-reconnect across a
    // full OS-process restart cannot be exercised here - the account's
    // stored OAuth credential is gone the moment the process exits,
    // exactly like scripts/verify-chat-automation.mjs's own restart test,
    // which never re-establishes a live connector post-restart either. A
    // fresh device-flow connection below proves the alert pipeline itself
    // (Event Bus -> matcher -> queue -> public route) still works after a
    // restart; real auto-reconnect-with-persisted-credentials is covered
    // by scripts/verify-twitch-engagement.mjs's own reconnect scenarios
    // (which restart the CONNECTION, not the process) plus this
    // application's real OS-keychain-backed production SecretStore.
    step('Confirm the alert pipeline still works end-to-end after a restart (via a freshly re-authorized account)');
    const accountC = await connectMetadataAccount(baseUrl, twitchState, 'c');
    const socket2 = await enableEngagement(baseUrl, twitchState, wsState, accountC.accountId, accountC.fakeUserId);
    sendWS(socket2, followEvent('u_follower_postrestart', 'a_follower_postrestart', 'APostRestart'));
    await waitForQueue(baseUrl, profile1.id, (s) => findAlert(s, 'just followed') !== undefined, QUEUE_TIMEOUT_MS, 'a post-restart live event to still become an alert');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile1.id}/queue/clear`);
    socket2.destroy();

    // --- public payload leak scan -----------------------------------

    step('Confirm the public stream never exposes queued-future-alert content, tokens, or raw provider data');
    const iter3 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile1.publicSlug}/stream`);
    const reset3 = await nextEvent(iter3, 10_000, 'a fresh reset');
    const rawText = reset3.raw;
    for (const forbidden of ['nextQueued', 'totalEnqueued', 'access_token', 'refresh_token', 'eventsub', 'reconnect_url', accountA.accountId]) {
      expect(!rawText.includes(forbidden), `the public stream payload never contains "${forbidden}"`, rawText);
    }
    await iter3.return();

    // --- logs never leak secrets or real alert content --------------

    step('Confirm the backend\'s own stdout/stderr never leaks tokens, public slugs, or real (non-synthetic) alert content');
    const backendOutput = backend.getOutput();
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!backendOutput.includes(entry), 'no access token appears in backend logs', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!backendOutput.includes(entry), 'no refresh token appears in backend logs', undefined);
    }
    for (const slug of [profile1.publicSlug, oldSlug, newSlug]) {
      expect(!backendOutput.includes(slug), `public slug "${slug}" never appears in backend logs`, undefined);
    }
    expect(!backendOutput.includes('a_follower_postrestart'), 'a real matched alert\'s username never appears in backend logs', undefined);

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
  console.error(`\nAlerts verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
