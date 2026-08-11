#!/usr/bin/env node
/**
 * Local, no-real-Twitch, no-real-OBS verification of Stage 13A: the
 * shared visual-design document, its persistence/HTTP API, and the
 * immutable per-alert-instance snapshot integration - layered on top
 * of the Stage 12A/12B queue/playback runtime already covered by
 * scripts/verify-alerts.mjs and scripts/verify-alert-advanced-queue.mjs
 * (which this script does not repeat).
 *
 * Reuses the identical fake OAuth/Helix/EventSub harness (same shapes,
 * copied verbatim - see verify-alerts.mjs for the canonical version of
 * each helper) but a fresh, focused main() covering a representative
 * subset of the Stage 13A task's own ~36-item Part 57 verification
 * list. Every scenario intentionally NOT covered here is named against
 * a specific covering Go/frontend test in docs/progress.md's Stage 13A
 * test-verification entry.
 *
 * Usage: node scripts/verify-alert-designer.mjs
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

async function waitForShow(iterator, predicate, timeoutMs, label) {
  return waitUntil(async () => {
    const ev = await nextEvent(iterator, 3000, label).catch(() => null);
    if (ev !== null && ev.event === 'alert.show' && predicate(ev.data.alert)) return ev;
    return false;
  }, timeoutMs, label);
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

function ruleBody(overrides) {
  return {
    name: overrides.name,
    enabled: overrides.enabled ?? true,
    eventType: overrides.eventType,
    priority: overrides.priority ?? 50,
    durationMs: overrides.durationMs ?? 3000,
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

// --- Stage 13A visual-design document builders -----------------------

/** A minimal, valid document (CurrentVersion - bumped to 2 by Stage 13B,
 * then to 3 by Stage 14B, docs/visual-designs.md's own version-decision
 * sections; a Stage 13A v1 document's wire shape is unchanged, only the
 * label moved) - one text layer bound to alert_rendered_text, colored/
 * named distinctly per call so tests can tell two saved documents apart
 * by their own fill/name. */
function designDocument({ layerName = 'Alert text', fill = '#112233', staticSuffix = '' } = {}) {
  return {
    version: 3,
    canvas: { width: 1920, height: 1080, transparent: true },
    layers: [
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: layerName, kind: 'shape', visible: true, locked: false, order: 0,
        frame: { x: 0, y: 900, width: 1920, height: 180 }, opacity: 1,
        shape: { kind: 'rectangle', fill, borderColor: '#000000', borderWidth: 0, cornerRadius: 0 },
        entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 300,
      },
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: `${layerName} text${staticSuffix}`, kind: 'text', visible: true, locked: false, order: 1,
        frame: { x: 160, y: 940, width: 1600, height: 100 }, opacity: 1,
        text: {
          binding: 'alert_rendered_text', missingValueBehavior: 'hide',
          fontFamily: 'system-ui', fontSize: 40, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
          textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
          outlineWidth: 0, outlineColor: '#000000',
          shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
        },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      },
    ],
  };
}

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
  console.log('Stage 13A alert visual-design verification (local fakes only, no real Twitch, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-alert-designer-'));
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

    step('Create a profile and connect + enable one Twitch account (Stage 12 still works with no design at all)');
    const createProfile = await request(baseUrl, 'POST', '/api/alert-profiles', { name: 'Designer Verification' });
    expect(createProfile.status === 201, 'the profile was created', createProfile.body);
    const profile = createProfile.body;
    await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    const account = await connectMetadataAccount(baseUrl, twitchState, 'a');
    const socket = await enableEngagement(baseUrl, twitchState, wsState, account.accountId, account.fakeUserId);

    step('Create a low-priority interruptible follow rule and a high-priority interrupting raid rule, neither with a design yet');
    const lowRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Low (interruptible)', eventType: 'follow', priority: 40, durationMs: 4000, interruptible: true,
      textTemplate: '{username} followed (low)!',
    }))).body;
    const highRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'High (interrupting)', eventType: 'raid', priority: 90, durationMs: 3000, showQuantity: true,
      interruptMode: 'lower_priority', textTemplate: '{username} raided with {quantity} (high)!',
    }))).body;
    const bitsRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Bits (groupable)', eventType: 'bits', priority: 50, allowGrouping: true, groupWindowMs: 3000,
      showQuantity: true, textTemplate: '{username} cheered {quantity} bits x{groupCount} (bits)!',
    }))).body;

    // --- 1-4: generated draft, never persisted, deterministic --------

    step('GET visual-design for a rule with no saved design returns a generated, never-persisted draft');
    const draft1 = await request(baseUrl, 'GET', `/api/alert-rules/${lowRule.id}/visual-design`);
    expect(draft1.status === 200 && draft1.body.persisted === false && draft1.body.revision === 0, 'the draft is unpersisted, revision 0', draft1.body);
    expect(Array.isArray(draft1.body.document.layers) && draft1.body.document.layers.length > 0, 'the draft has at least one layer representing the legacy presentation', draft1.body.document);

    step('Repeated GETs of the same unsaved draft are deterministic');
    const draft2 = await request(baseUrl, 'GET', `/api/alert-rules/${lowRule.id}/visual-design`);
    expect(draft2.body.document.layers[0].id === draft1.body.document.layers[0].id, 'the draft layer id is stable across repeated GETs', [draft1.body, draft2.body]);
    expect(draft2.body.persisted === false, 'a second GET still reports unpersisted - opening the draft never saves it', draft2.body);

    // --- 5-9: save / revision / conflict -------------------------------

    step('PUT saves a version-1 design at revision 1');
    const lowDoc1 = designDocument({ layerName: 'Low v1', fill: '#112233' });
    const save1 = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 0, document: lowDoc1 });
    expect(save1.status === 200 && save1.body.persisted === true && save1.body.revision === 1, 'first save persisted at revision 1', save1.body);

    step('GET now returns the saved design, not a draft');
    const getSaved = await request(baseUrl, 'GET', `/api/alert-rules/${lowRule.id}/visual-design`);
    expect(getSaved.body.persisted === true && getSaved.body.revision === 1, 'GET reflects the persisted save', getSaved.body);

    step('A stale expectedRevision returns 409 and never overwrites');
    const staleSave = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 0, document: designDocument({ layerName: 'Should not persist' }) });
    expect(staleSave.status === 409, 'stale revision save is rejected (409)', staleSave.body);
    const stillOld = await request(baseUrl, 'GET', `/api/alert-rules/${lowRule.id}/visual-design`);
    expect(stillOld.body.revision === 1, 'the design was never overwritten by the stale save', stillOld.body);

    step('A correct expectedRevision replaces the design and increments the revision');
    const lowDoc2 = designDocument({ layerName: 'Low v2', fill: '#445566' });
    const save2 = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 1, document: lowDoc2 });
    expect(save2.status === 200 && save2.body.revision === 2, 'the replacement incremented the revision to 2', save2.body);

    // --- validation rejection matrix (representative subset - full
    // matrix already covered by internal/httpapi/visualdesign_test.go
    // and internal/domain/visualdesign/validation_test.go) -----------

    step('An off-canvas frame is rejected (422)');
    const offCanvas = designDocument();
    offCanvas.layers[0].frame = { x: 1900, y: 0, width: 400, height: 100 };
    const offCanvasResp = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 2, document: offCanvas });
    expect(offCanvasResp.status === 422, 'an off-canvas frame is rejected', offCanvasResp.body);

    step('An unavailable text binding for the rule\'s own event type is rejected (422)');
    const badBinding = designDocument();
    badBinding.layers[1].text.binding = 'quantity'; // follow has no quantity
    const badBindingResp = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 2, document: badBinding });
    expect(badBindingResp.status === 422, 'a quantity binding on a follow rule is rejected', badBindingResp.body);

    step('An invalid (non-hex) color is rejected (422)');
    const badColor = designDocument();
    badColor.layers[0].shape.fill = 'red';
    const badColorResp = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 2, document: badColor });
    expect(badColorResp.status === 422, 'a non-hex color is rejected', badColorResp.body);

    step('An unrecognized layer kind is rejected (422)');
    const badKind = designDocument();
    badKind.layers[0].kind = 'video';
    delete badKind.layers[0].shape;
    const badKindResp = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 2, document: badKind });
    expect(badKindResp.status === 422, 'an arbitrary/unrecognized layer kind is rejected', badKindResp.body);

    step('An oversized document (static text over the 500-code-point bound) is rejected (422)');
    const oversized = designDocument();
    oversized.layers[1].text.binding = 'static';
    oversized.layers[1].text.staticText = 'a'.repeat(501);
    const oversizedResp = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 2, document: oversized });
    expect(oversizedResp.status === 422, 'oversized static text is rejected', oversizedResp.body);

    // --- 13-17: real event/queue integration ---------------------------

    step('Resume the queue, save a design on the high (raid) rule too, then confirm a real follow event shows renderingMode=visual_design with the saved layout');
    await request(baseUrl, 'PUT', `/api/alert-rules/${highRule.id}/visual-design`, { expectedRevision: 0, document: designDocument({ layerName: 'High v1', fill: '#990000' }) });
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);

    const iter1 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter1, 10_000, 'the initial reset');
    sendWS(socket, followEvent('u_follow_1', 'followuser1', 'FollowUser1'));
    const followShow = await waitForShow(iter1, (alert) => alert?.renderedText?.includes('(low)'), QUEUE_TIMEOUT_MS, 'the real follow alert to show design-driven');
    expect(followShow.data.alert.renderingMode === 'visual_design', 'the real follow alert reports renderingMode=visual_design', followShow.data.alert);
    expect(followShow.data.alert.visualDesign?.layers?.length === 2, 'the public payload carries the full 2-layer design', followShow.data.alert.visualDesign);
    expect(!('name' in followShow.data.alert.visualDesign.layers[0]), 'public layers never carry the editor-only "name" field', followShow.data.alert.visualDesign.layers[0]);
    expect(!('locked' in followShow.data.alert.visualDesign.layers[0]), 'public layers never carry the editor-only "locked" field', followShow.data.alert.visualDesign.layers[0]);
    expect(!followShow.raw.includes('Low v2'), 'the raw SSE payload never contains the editor-only layer name text', followShow.raw);
    const firstAlertId = followShow.data.alert.alertId;

    step('Saving a NEW design while that alert is still current never mutates it (Part 22\'s central guarantee)');
    const lowDoc3 = designDocument({ layerName: 'Low v3', fill: '#00AA00' });
    const save3 = await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 2, document: lowDoc3 });
    expect(save3.status === 200 && save3.body.revision === 3, 'the new design saved at revision 3', save3.body);
    let st = await queueStatus(baseUrl, profile.id);
    expect(st.current?.alertId === firstAlertId, 'the currently-playing alert is still the same instance after the save', st.current);

    step('The next NEW real alert for the same rule uses the newly-saved design revision');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await waitForQueue(baseUrl, profile.id, (s) => s.current == null, QUEUE_TIMEOUT_MS, 'the first alert to finish naturally');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket, followEvent('u_follow_2', 'followuser2', 'FollowUser2'));
    const secondShow = await waitForShow(iter1, (alert) => alert?.renderedText?.includes('(low)'), QUEUE_TIMEOUT_MS, 'a second real follow alert using the new design');
    expect(secondShow.raw.includes('00AA00'), 'the second alert uses the new design\'s own fill color', secondShow.raw);
    await iter1.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 4200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    // --- 18: Test Rule uses the saved design through the real queue ----

    step('Test Rule creates a synthetic alert carrying the rule\'s own saved design, through the real queue/public route');
    const iter2 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter2, 10_000, 'a fresh reset');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    const testResp = await request(baseUrl, 'POST', `/api/alert-rules/${lowRule.id}/test`, {});
    expect(testResp.status === 200 && testResp.body.synthetic === true, 'the synthetic test alert was created', testResp.body);
    const testShow = await waitForShow(iter2, (alert) => alert?.alertId === testResp.body.alertId, QUEUE_TIMEOUT_MS, 'the synthetic test alert to show with its design');
    expect(testShow.data.alert.renderingMode === 'visual_design', 'the synthetic test alert is also design-driven', testShow.data.alert);
    await iter2.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 4200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    // --- 19-20: replay preserves its own original snapshot -------------

    step('Replay Previous re-shows the alert\'s own originally-captured design, not whatever is currently saved');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    const iter3 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter3, 10_000, 'a fresh reset');
    sendWS(socket, followEvent('u_follow_replay', 'followreplay', 'FollowReplay'));
    await waitForShow(iter3, (alert) => alert?.renderedText?.includes('(low)'), QUEUE_TIMEOUT_MS, 'the alert to replay to show first');
    await waitForQueue(baseUrl, profile.id, (s) => s.replayAvailable === true, QUEUE_TIMEOUT_MS, 'a replay-eligible snapshot to exist');

    // Change the design again - the replay must still show the OLD one.
    await request(baseUrl, 'PUT', `/api/alert-rules/${lowRule.id}/visual-design`, { expectedRevision: 3, document: designDocument({ layerName: 'Low v4 (post-replay-snapshot)', fill: '#FF00FF' }) });
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/replay-previous`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    const replayShow = await waitForShow(iter3, (alert) => alert?.replayed === true, QUEUE_TIMEOUT_MS, 'the replayed alert to show');
    expect(!replayShow.raw.includes('FF00FF'), 'the replayed alert never picks up a design saved after it was originally captured', replayShow.raw);
    await iter3.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 4200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);

    // --- 21: grouping never mutates the design snapshot -----------------

    step('A grouped (merged) alert stays design-driven with its own unaffected design snapshot');
    const iter4 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter4, 10_000, 'a fresh reset');
    await request(baseUrl, 'PUT', `/api/alert-rules/${bitsRule.id}/visual-design`, { expectedRevision: 0, document: designDocument({ layerName: 'Bits design', fill: '#3355AA' }) });
    sendWS(socket, cheerEvent('u_cheer_group', 'cheergroup', 'CheerGroup', 100));
    sendWS(socket, cheerEvent('u_cheer_group', 'cheergroup', 'CheerGroup', 50));
    const groupedShow = await waitForShow(iter4, (alert) => alert?.groupCount === 2, QUEUE_TIMEOUT_MS, 'the merged Bits alert to show grouped and design-driven');
    expect(groupedShow.data.alert.renderingMode === 'visual_design', 'the grouped alert is still design-driven', groupedShow.data.alert);
    expect(groupedShow.data.alert.visualDesign?.layers?.length === 2, 'the grouped alert\'s design is the same untouched 2-layer document', groupedShow.data.alert.visualDesign);
    await iter4.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 3200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    // --- 22: preemption - each alert carries its own correct design ----

    step('A preemption replaces the current alert with the interrupting alert\'s own correct design (hide, then show, own design each)');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    const iter5 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter5, 10_000, 'a fresh reset');
    sendWS(socket, followEvent('u_follow_preempt', 'followpreempt', 'FollowPreempt'));
    const lowShow = await waitForShow(iter5, (alert) => alert?.renderedText?.includes('(low)'), QUEUE_TIMEOUT_MS, 'the low-priority alert to show first');
    const lowAlertId = lowShow.data.alert.alertId;

    sendWS(socket, raidEvent('u_raid_preempt', 'raidpreempt', 'RaidPreempt', 5));
    const hideEv = await waitUntil(async () => {
      const ev = await nextEvent(iter5, 3000, 'a hide event').catch(() => null);
      return ev !== null && ev.event === 'alert.hide' ? ev : false;
    }, QUEUE_TIMEOUT_MS, 'the low-priority alert to be hidden (preempted)');
    expect(hideEv.data.alertId === lowAlertId && hideEv.data.reason === 'preempted', 'the hide payload names the preempted alert and reason', hideEv.data);

    const highShow = await waitForShow(iter5, (alert) => alert?.renderedText?.includes('(high)'), QUEUE_TIMEOUT_MS, 'the interrupting raid alert to show with its OWN design');
    expect(highShow.raw.includes('990000'), 'the interrupting alert uses the high rule\'s own saved fill color, never the low rule\'s', highShow.raw);
    expect(highShow.data.alert.alertId !== lowAlertId, 'the interrupting alert has its own distinct id', highShow.data.alert);
    await iter5.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 3200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);

    // --- 23-25: Reset to legacy -----------------------------------------

    step('DELETE (Reset to legacy) returns the rule to legacy mode for future alerts, idempotently');
    const del1 = await request(baseUrl, 'DELETE', `/api/alert-rules/${lowRule.id}/visual-design`, undefined);
    expect(del1.status === 204, 'DELETE succeeds (204)', del1.status);
    const del2 = await request(baseUrl, 'DELETE', `/api/alert-rules/${lowRule.id}/visual-design`, undefined);
    expect(del2.status === 204, 'a second DELETE is still 204 (idempotent)', del2.status);
    const afterDelete = await request(baseUrl, 'GET', `/api/alert-rules/${lowRule.id}/visual-design`);
    expect(afterDelete.body.persisted === false, 'GET after delete returns an unpersisted draft again', afterDelete.body);

    step('A NEW alert created after Reset to legacy renders through the legacy fixed renderer, not the old design');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    const iter6 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter6, 10_000, 'a fresh reset');
    sendWS(socket, followEvent('u_follow_legacy', 'followlegacy', 'FollowLegacy'));
    const legacyShow = await waitForShow(iter6, (alert) => alert?.renderedText?.includes('(low)'), QUEUE_TIMEOUT_MS, 'a post-reset alert to show as legacy');
    expect(legacyShow.data.alert.renderingMode === 'legacy', 'the new alert is legacy-rendered after Reset to legacy', legacyShow.data.alert);
    expect(legacyShow.data.alert.visualDesign === null || legacyShow.data.alert.visualDesign === undefined, 'a legacy alert carries no visualDesign payload', legacyShow.data.alert);
    await iter6.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/pause`);
    await new Promise((r) => setTimeout(r, 4200));
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);

    // --- 26: rule-deletion cascade ---------------------------------------

    step('Deleting a rule cascades its saved visual design');
    const throwawayRule = (await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/rules`, ruleBody({
      name: 'Throwaway', eventType: 'follow', textTemplate: '{username} followed (throwaway)!',
    }))).body;
    await request(baseUrl, 'PUT', `/api/alert-rules/${throwawayRule.id}/visual-design`, { expectedRevision: 0, document: designDocument({ layerName: 'Throwaway design' }) });
    const deleteRule = await fetch(`${baseUrl}/api/alert-rules/${throwawayRule.id}`, { method: 'DELETE' });
    expect(deleteRule.status === 204, 'the throwaway rule was deleted', deleteRule.status);
    const designAfterRuleDelete = await request(baseUrl, 'GET', `/api/alert-rules/${throwawayRule.id}/visual-design`);
    expect(designAfterRuleDelete.status === 404, 'GET visual-design for a deleted rule 404s (the rule itself is gone)', designAfterRuleDelete.body);

    // --- 27: restart survives ---------------------------------------------

    step('Restart the backend: the high rule\'s saved design survives, and a fresh real event still renders it correctly');
    socket.destroy();
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);

    const designAfterRestart = await request(baseUrl, 'GET', `/api/alert-rules/${highRule.id}/visual-design`);
    expect(designAfterRestart.body.persisted === true && designAfterRestart.body.document.layers.length === 2, 'the high rule\'s design survived the restart', designAfterRestart.body);

    const accountAfterRestart = await connectMetadataAccount(baseUrl, twitchState, 'b');
    const socket2 = await enableEngagement(baseUrl, twitchState, wsState, accountAfterRestart.accountId, accountAfterRestart.fakeUserId);
    const iter7 = sseEvents(`${baseUrl}/api/public/alert-profiles/${profile.publicSlug}/stream`);
    await nextEvent(iter7, 10_000, 'a fresh reset');
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/resume`);
    sendWS(socket2, raidEvent('u_raid_postrestart', 'raidpostrestart', 'RaidPostRestart', 3));
    const postRestartShow = await waitForShow(iter7, (alert) => alert?.renderedText?.includes('(high)'), QUEUE_TIMEOUT_MS, 'a post-restart real event to still render design-driven');
    expect(postRestartShow.data.alert.renderingMode === 'visual_design', 'the post-restart alert is still design-driven', postRestartShow.data.alert);
    await iter7.return();
    await request(baseUrl, 'POST', `/api/alert-profiles/${profile.id}/queue/clear`);
    socket2.destroy();

    // --- secret/leak scan --------------------------------------------

    step('Confirm the backend\'s own logs and every captured HTTP/SSE body never leak a token or raw EventSub payload');
    const backendOutput = backend.getOutput();
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!backendOutput.includes(entry), 'no access token appears in backend logs', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!backendOutput.includes(entry), 'no refresh token appears in backend logs', undefined);
    }
    const haystack = secretScanChunks.join('\n');
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!haystack.includes(entry), 'access token never appears in captured HTTP/SSE bodies', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!haystack.includes(entry), 'refresh token never appears in captured HTTP/SSE bodies', undefined);
    }
    expect(!haystack.includes('"eventsub"'), 'no raw EventSub envelope key ever appears in a captured HTTP/SSE body', undefined);
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
  console.error(`\nAlert designer verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
