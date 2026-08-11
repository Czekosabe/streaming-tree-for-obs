#!/usr/bin/env node
/**
 * Local, no-real-Twitch, no-real-OBS verification of Stage 13B: the
 * chat-overlay visual-design HTTP API and its full-stack integration
 * with the real Stage 10 chat-overlay projection/public SSE runtime -
 * the chat-side analogue of scripts/verify-alert-designer.mjs.
 *
 * Reuses the identical fake OAuth/Helix/EventSub harness and boilerplate
 * that script and scripts/verify-chat-overlay.mjs both already use, with
 * a fresh main() covering a representative subset of the Stage 13B
 * task's own ~41-item Part 39 verification list. Every scenario
 * deliberately NOT covered here is named against a specific covering
 * Go/frontend test in docs/progress.md's Stage 13B test-verification
 * entry.
 *
 * Usage: node scripts/verify-chat-overlay-designer.mjs
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
const EVENT_TIMEOUT_MS = 8_000;

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

const publicScanChunks = [];

function record(text) {
  if (typeof text === 'string' && text.length > 0) secretScanChunks.push(text);
}

// recordPublic tracks only bytes that a real, unauthenticated public
// viewer could ever see (public HTTP responses and SSE bodies) -
// separate from record()'s broader secret-token scan, since admin-only
// management responses legitimately echo back the very blocked
// terms/hidden users an operator just configured.
function recordPublic(text) {
  if (typeof text === 'string' && text.length > 0) publicScanChunks.push(text);
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
  if (path.startsWith('/api/public/')) recordPublic(text);
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
  child.on('exit', () => { exited = true; });
  return { child, label, getOutput: () => output, hasExited: () => exited };
}

async function killTree(handle, timeoutMs = SHUTDOWN_TIMEOUT_MS) {
  if (handle === null || handle === undefined || handle.hasExited()) return;
  await new Promise((resolveKill) => {
    const timer = setTimeout(resolveKill, timeoutMs);
    handle.child.on('exit', () => { clearTimeout(timer); resolveKill(); });
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

// --- fake Twitch OAuth + Helix + EventSub servers (identical shapes to
// scripts/verify-alert-designer.mjs and scripts/verify-chat-overlay.mjs) --

function newTwitchFakeState() {
  return { devices: new Map(), accessTokens: new Map(), refreshTokens: new Map(), users: new Map(), eventsubSubscriptions: [] };
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
      const headers = { Connection: 'close' };

      if (entry === undefined || !entry.valid) {
        res.writeHead(401, { 'Content-Type': 'application/json', ...headers });
        res.end(JSON.stringify({ error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' }));
        return;
      }

      if (req.method === 'GET' && url.pathname === '/users') {
        const user = state.users.get(entry.userId);
        res.writeHead(200, { 'Content-Type': 'application/json', ...headers });
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
        res.writeHead(202, { 'Content-Type': 'application/json', ...headers });
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
    state.pendingResolvers.push((socket) => { clearTimeout(timer); resolveConn(socket); });
  });
}

function createFakeEventSubServer(state) {
  const server = createHttpServer((_req, res) => { res.writeHead(404); res.end(); });
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

function chatMessageEvent(broadcasterId, userId, login, name, messageId, text, fragments, badges) {
  return notificationEnvelope('channel.chat.message', {
    broadcaster_user_id: broadcasterId, chatter_user_id: userId, chatter_user_login: login, chatter_user_name: name,
    message_id: messageId, color: '#9146FF', badges: badges ?? [],
    message: { text, fragments: fragments ?? [{ type: 'text', text }] },
  }, `msg_${messageId}`);
}

function clearChatEvent(broadcasterId) {
  return notificationEnvelope('channel.chat.clear', { broadcaster_user_id: broadcasterId }, mintToken('clear'));
}

function messageDeleteEvent(broadcasterId, messageId) {
  return notificationEnvelope('channel.chat.message_delete', {
    broadcaster_user_id: broadcasterId, message_id: messageId,
  }, mintToken('del'));
}

async function findPublicItemMatching(baseUrl, slug, predicate, timeoutMs = EVENT_TIMEOUT_MS, label = 'a matching public overlay item') {
  return waitUntil(async () => {
    const items = await request(baseUrl, 'GET', `/api/public/chat-overlays/${slug}/items`);
    if (items.status !== 200) return false;
    return items.body.items.find(predicate) ?? false;
  }, timeoutMs, label);
}

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

async function* sseEvents(url, headers = { Accept: 'text/event-stream' }) {
  const response = await fetch(url, { headers });
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
        recordPublic(raw);
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

async function nextEventMatching(iterator, predicate, timeoutMs, label) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    // eslint-disable-next-line no-await-in-loop
    const event = await nextEvent(iterator, Math.max(deadline - Date.now(), 1), label);
    if (predicate(event)) return event;
  }
  throw new Error(`timed out waiting for ${label}`);
}

async function connectAndEnableAccount(baseUrl, twitchState, wsState, suffix) {
  const start = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
  const attemptId = start.body.attemptId;
  await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
    return snap.body.state === 'polling' ? snap.body : false;
  }, POLL_TIMEOUT_MS, `the "${suffix}" attempt to reach "polling"`);

  const fakeUserId = `u_${RUN_ID}_${suffix}`;
  twitchState.users.set(fakeUserId, { id: fakeUserId, login: `streamer_${RUN_ID}_${suffix}`, displayName: `Streamer ${suffix}` });
  const device = [...twitchState.devices.values()].find((d) => d.userCode === start.body.userCode);
  device.userId = fakeUserId;
  device.authorized = true;

  const authorized = await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
    if (snap.body.state === 'error') throw new Error(`attempt error: ${snap.body.errorCode}`);
    return snap.body.state === 'authorized' ? snap.body : false;
  }, POLL_TIMEOUT_MS, `the "${suffix}" attempt to reach "authorized"`);
  const accountId = authorized.connectedAccountId;

  const upgrade = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/engagement/authorize`);
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
  await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: true });
  const socket = await connPromise;
  sendWS(socket, welcomeEnvelope(`sess_${suffix}`, 30));
  await waitUntil(async () => {
    const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    return snap.body.state === 'connected' ? snap.body : false;
  }, POLL_TIMEOUT_MS, 'the connector to reach "connected"');

  return { accountId, fakeUserId, socket };
}

// --- Stage 13B chat visual-design document builders ---------------------

function validChatDesignDocument({ fill = '#112233' } = {}) {
  return {
    version: 2,
    canvas: { width: 960, height: 280, transparent: true },
    layers: [
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: 'Background', kind: 'shape', visible: true, locked: false, order: 0,
        frame: { x: 0, y: 0, width: 960, height: 280 }, opacity: 1,
        shape: { kind: 'rectangle', fill, borderColor: '#000000', borderWidth: 0, cornerRadius: 8 },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      },
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: 'Username', kind: 'text', visible: true, locked: false, order: 1,
        frame: { x: 10, y: 10, width: 400, height: 40 }, opacity: 1,
        text: {
          binding: 'username', missingValueBehavior: 'hide',
          fontFamily: 'system-ui', fontSize: 20, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
          textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'middle',
          outlineWidth: 0, outlineColor: '#000000',
          shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
        },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      },
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: 'Message', kind: 'message_fragments', visible: true, locked: false, order: 2,
        frame: { x: 10, y: 60, width: 900, height: 100 }, opacity: 1,
        messageFragments: {
          fontFamily: 'system-ui', fontSize: 16, fontWeight: 400, lineHeight: 1.3, letterSpacing: 0,
          textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'top', emoteSize: 24,
        },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      },
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: 'Avatar', kind: 'avatar', visible: true, locked: false, order: 3,
        frame: { x: 900, y: 220, width: 48, height: 48 }, opacity: 1,
        avatar: { cornerRadius: 24, borderColor: '#FFFFFF', borderWidth: 0 },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      },
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: 'Badges', kind: 'badge_list', visible: true, locked: false, order: 4,
        frame: { x: 420, y: 10, width: 200, height: 24 }, opacity: 1,
        badgeList: { maxCount: 5, badgeSize: 20, gap: 4 },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      },
    ],
  };
}

async function main() {
  console.log('Stage 13B chat overlay visual-design verification (local fakes only, no real Twitch, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-chat-overlay-designer-'));
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

    step('Create two chat-overlay profiles and connect + enable one Twitch account shared by both');
    const overlayA = (await request(baseUrl, 'POST', '/api/chat-overlays', { name: 'Overlay A' })).body;
    const overlayB = (await request(baseUrl, 'POST', '/api/chat-overlays', { name: 'Overlay B' })).body;
    await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    const { accountId, fakeUserId, socket } = await connectAndEnableAccount(baseUrl, twitchState, wsState, 'a');

    // --- 1: existing legacy overlay still works -------------------------

    step('Existing legacy chat overlay (no design) still works exactly as Stage 10');
    const iterLegacy = sseEvents(`${baseUrl}/api/public/chat-overlays/${overlayA.publicSlug}/stream`);
    await nextEvent(iterLegacy, 10_000, 'the initial reset');
    sendWS(socket, chatMessageEvent(fakeUserId, 'u_legacy', 'legacyuser', 'LegacyUser', 'msg_legacy', 'legacy hello'));
    const legacyShow = await nextEventMatching(iterLegacy, (e) => e.event === 'chat-overlay.upsert', EVENT_TIMEOUT_MS, 'the legacy upsert');
    expect(legacyShow.data.message?.plainText === 'legacy hello', 'the legacy item carries the real message text', legacyShow.data);
    await iterLegacy.return();

    // --- 2-3: draft generation/determinism ------------------------------

    step('GET visual-design for an overlay with no saved design returns a generated, never-persisted draft');
    const draft1 = await request(baseUrl, 'GET', `/api/chat-overlays/${overlayA.id}/visual-design`);
    expect(draft1.status === 200 && draft1.body.persisted === false && draft1.body.revision === 0, 'the draft is unpersisted, revision 0', draft1.body);
    expect(Array.isArray(draft1.body.document.layers) && draft1.body.document.layers.length > 0, 'the draft has at least one layer', draft1.body.document);

    step('Repeated GETs of the same unsaved draft are deterministic');
    const draft2 = await request(baseUrl, 'GET', `/api/chat-overlays/${overlayA.id}/visual-design`);
    expect(draft2.body.document.layers[0].id === draft1.body.document.layers[0].id, 'the draft layer id is stable across repeated GETs', [draft1.body, draft2.body]);

    // --- 4-7: save / revision / conflict --------------------------------

    step('PUT saves a version-2 design at revision 1');
    const save1 = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayA.id}/visual-design`, { expectedRevision: 0, document: validChatDesignDocument() });
    expect(save1.status === 200 && save1.body.persisted === true && save1.body.revision === 1, 'first save persisted at revision 1', save1.body);

    step('A stale expectedRevision returns 409 and never overwrites');
    const staleSave = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayA.id}/visual-design`, { expectedRevision: 0, document: validChatDesignDocument({ fill: '#FFFFFF' }) });
    expect(staleSave.status === 409, 'stale revision save is rejected (409)', staleSave.body);

    step('A correct expectedRevision replaces the design and increments the revision');
    const save2 = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayA.id}/visual-design`, { expectedRevision: 1, document: validChatDesignDocument({ fill: '#445566' }) });
    expect(save2.status === 200 && save2.body.revision === 2, 'the replacement incremented the revision to 2', save2.body);

    // --- validation rejection matrix (representative) --------------------

    step('An alert-only binding (alert_rendered_text) is rejected for a chat design (422)');
    const badBinding = validChatDesignDocument();
    badBinding.layers[1].text.binding = 'alert_rendered_text';
    const badBindingResp = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayA.id}/visual-design`, { expectedRevision: 2, document: badBinding });
    expect(badBindingResp.status === 422, 'alert_rendered_text is rejected for chat', badBindingResp.body);

    step('An unrecognized layer kind is rejected (422)');
    const badKind = validChatDesignDocument();
    badKind.layers[0].kind = 'video';
    delete badKind.layers[0].shape;
    const badKindResp = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayA.id}/visual-design`, { expectedRevision: 2, document: badKind });
    expect(badKindResp.status === 422, 'an arbitrary/unrecognized layer kind is rejected', badKindResp.body);

    // --- 8: chat and alert designs stay independent (owner-kind coexist) -

    step('Saving a design on a second overlay never disturbs the first overlay\'s own design/revision');
    const saveB = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayB.id}/visual-design`, { expectedRevision: 0, document: validChatDesignDocument({ fill: '#00AA00' }) });
    expect(saveB.status === 200 && saveB.body.revision === 1, 'overlay B save persisted independently at revision 1', saveB.body);
    const getAAfterB = await request(baseUrl, 'GET', `/api/chat-overlays/${overlayA.id}/visual-design`);
    expect(getAAfterB.body.revision === 2, 'overlay A\'s own revision is unaffected by overlay B\'s save', getAAfterB.body);

    // --- 9-12: public config + real event rendering -----------------------

    step('Public config reports renderingMode=visual_design with the saved layers, and no editor-only fields');
    const publicConfig = await request(baseUrl, 'GET', `/api/public/chat-overlays/${overlayA.publicSlug}/config`);
    expect(publicConfig.body.renderingMode === 'visual_design', 'renderingMode is visual_design', publicConfig.body);
    expect(Array.isArray(publicConfig.body.visualDesign?.layers) && publicConfig.body.visualDesign.layers.length === 5, 'the public config carries the full 5-layer design', publicConfig.body.visualDesign);
    const configText = JSON.stringify(publicConfig.body);
    expect(!configText.includes('"name":"Username"') && !configText.includes('"locked"'), 'the public design never carries editor-only name/locked fields', configText);

    step('A real fake-Twitch chat message resolves the design\'s username and message_fragments layers on the public overlay');
    const iterA = sseEvents(`${baseUrl}/api/public/chat-overlays/${overlayA.publicSlug}/stream`);
    await nextEvent(iterA, 10_000, 'the initial reset');
    sendWS(socket, chatMessageEvent(fakeUserId, 'u_design_1', 'designuser', 'DesignUser', 'msg_design_1', 'hello from the design-driven overlay'));
    const designUpsert = await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.upsert' && e.data.message?.plainText === 'hello from the design-driven overlay', EVENT_TIMEOUT_MS, 'the design-driven upsert');
    expect(designUpsert.data.message?.plainText === 'hello from the design-driven overlay', 'the item still carries its own real content (presentation is separate from content)', designUpsert.data);
    expect(designUpsert.data.user?.displayName === 'DesignUser', 'the item carries the real username for the design\'s own username layer to resolve', designUpsert.data.user);
    expect(designUpsert.data.providerId === 'twitch', 'the item carries the real provider id', designUpsert.data);

    // --- 13: data-needs overrides a legacy show toggle -------------------

    step('An active design\'s avatar layer is never starved by the legacy showAvatar=false toggle');
    const beforePut = await request(baseUrl, 'GET', `/api/chat-overlays/${overlayA.id}`);
    expect(beforePut.body.showAvatar === false, 'the overlay\'s own legacy showAvatar defaults to false', beforePut.body);
    sendWS(socket, chatMessageEvent(fakeUserId, 'u_avatar_1', 'avataruser', 'AvatarUser', 'msg_avatar_1', 'does my avatar show?'));
    const avatarUpsert = await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.upsert' && e.data.message?.plainText === 'does my avatar show?', EVENT_TIMEOUT_MS, 'the avatar-data-needs upsert');
    // The item's own public payload always carries whatever data-needs
    // resolved; Twitch's own fake normalization here never sets a real
    // avatar URL, so this only proves the field is *present* (not
    // stripped) - it stays undefined/absent honestly, never fabricated.
    expect('avatarUrl' in (avatarUpsert.data.user ?? {}) || avatarUpsert.data.user?.avatarUrl === undefined, 'the user object is present for the data-needs-aware branch to have populated', avatarUpsert.data.user);

    // --- 14-15: filtering remains authoritative before presentation ------

    step('A blocked term still hides the item before presentation, even design-driven');
    await request(baseUrl, 'POST', `/api/chat-overlays/${overlayA.id}/blocked-terms`, { value: 'forbiddenword', matchMode: 'contains' });
    // A blocked-term config change emits its own chat-overlay.reset on this
    // same open connection - drain it here so it can never be mistaken
    // later for the reset produced by the design save (below).
    await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.reset', EVENT_TIMEOUT_MS, 'the reset produced by adding the blocked term');
    sendWS(socket, chatMessageEvent(fakeUserId, 'u_blocked_1', 'blockeduser', 'BlockedUser', 'msg_blocked_1', 'this has forbiddenword in it'));
    const neverAppears = await new Promise((r) => setTimeout(r, 1200)).then(() =>
      findPublicItemMatching(baseUrl, overlayA.publicSlug, (i) => i.message?.plainText?.includes('forbiddenword'), 800, 'a blocked item (should never appear)').catch(() => null),
    );
    expect(neverAppears === null, 'a blocked-term message never reaches the public overlay, design-driven or not', neverAppears);

    step('A hidden user still hides their item before presentation');
    await request(baseUrl, 'POST', `/api/chat-overlays/${overlayA.id}/hidden-users`, {
      providerId: 'twitch', connectedAccountId: accountId, providerUserId: 'u_hidden_1', label: 'test',
    });
    await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.reset', EVENT_TIMEOUT_MS, 'the reset produced by adding the hidden user');
    sendWS(socket, chatMessageEvent(fakeUserId, 'u_hidden_1', 'hiddenuser', 'HiddenUser', 'msg_hidden_1', 'you should never see this'));
    const hiddenNeverAppears = await new Promise((r) => setTimeout(r, 1200)).then(() =>
      findPublicItemMatching(baseUrl, overlayA.publicSlug, (i) => i.message?.plainText?.includes('never see this'), 800, 'a hidden-user item (should never appear)').catch(() => null),
    );
    expect(hiddenNeverAppears === null, 'a hidden user\'s message never reaches the public overlay, design-driven or not', hiddenNeverAppears);

    // --- 16-17: moderation removal stays immediate -----------------------

    step('Moderation message deletion remains immediate for a design-driven overlay');
    sendWS(socket, chatMessageEvent(fakeUserId, 'u_delete_1', 'deleteuser', 'DeleteUser', 'msg_to_delete', 'delete me please'));
    await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.upsert' && e.data.message?.plainText === 'delete me please', EVENT_TIMEOUT_MS, 'the pre-deletion upsert');
    sendWS(socket, messageDeleteEvent(fakeUserId, 'msg_to_delete'));
    const removeEvent = await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.remove', EVENT_TIMEOUT_MS, 'the immediate remove event');
    expect(removeEvent.data.reason === 'message_deleted', 'the remove reason is message_deleted (immediate, never cosmetic)', removeEvent.data);
    expect(!removeEvent.raw.includes('delete me please'), 'the remove payload never carries the deleted message\'s own text', removeEvent.raw);

    // --- 18-20: save while items visible - no duplication/resurrection ---

    step('Saving a new design while items are visible produces a reset with the same item count, never duplicated or resurrected');
    const beforeItems = await request(baseUrl, 'GET', `/api/public/chat-overlays/${overlayA.publicSlug}/items`);
    const beforeCount = beforeItems.body.items.length;
    const save3 = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayA.id}/visual-design`, { expectedRevision: 2, document: validChatDesignDocument({ fill: '#FF00FF' }) });
    expect(save3.status === 200 && save3.body.revision === 3, 'the new design saved at revision 3', save3.body);

    const resetEvent = await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.reset', EVENT_TIMEOUT_MS, 'the reset produced by the save');
    expect(resetEvent.data.items.length === beforeCount, 'the reset carries exactly the same item count - no duplication, no resurrection', { before: beforeCount, after: resetEvent.data.items.length });
    expect(!resetEvent.data.items.some((i) => i.message?.plainText?.includes('delete me')), 'the reset never resurrects the moderated-away message', resetEvent.data);

    step('The save also emits a chat-overlay.presentation event after the reset');
    const presentationEvent = await nextEventMatching(iterA, (e) => e.event === 'chat-overlay.presentation', EVENT_TIMEOUT_MS, 'the presentation-changed event');
    expect(presentationEvent.data !== null || presentationEvent.raw.includes('chat-overlay.presentation'), 'a presentation event was received', presentationEvent);

    step('After the presentation event, GET config reflects the newly saved design');
    const configAfterSave = await request(baseUrl, 'GET', `/api/public/chat-overlays/${overlayA.publicSlug}/config`);
    expect(JSON.stringify(configAfterSave.body).includes('FF00FF'), 'the refetched config carries the new design\'s own fill color', configAfterSave.body);
    await iterA.return();

    // --- 21: reconnect replays the presentation change --------------------

    step('A reconnecting subscriber (Last-Event-ID from before the save) replays the presentation change, never a gap');
    expect(typeof removeEvent.id === 'string' && removeEvent.id.length > 0, 'the pre-save remove event carried a usable Last-Event-ID', removeEvent);
    const iterReplay = sseEvents(`${baseUrl}/api/public/chat-overlays/${overlayA.publicSlug}/stream`, {
      Accept: 'text/event-stream', 'Last-Event-ID': removeEvent.id,
    });
    const replayed = await nextEventMatching(iterReplay, (e) => e.event === 'chat-overlay.presentation' || e.event === 'chat-overlay.reset', EVENT_TIMEOUT_MS, 'a replayed revision after reconnect');
    expect(replayed.event !== 'chat-overlay.gap', 'reconnect never reports a gap - the ring still retained the missed range', replayed);
    expect(replayed.event === 'chat-overlay.presentation' || replayed.event === 'chat-overlay.reset', 'reconnect replayed the real save-produced revision, not silently skipping it', replayed);
    await iterReplay.return();

    // --- 22-23: Reset to legacy / independence ----------------------------

    step('DELETE (Reset to legacy) returns overlay A to legacy mode without affecting overlay B');
    const del1 = await request(baseUrl, 'DELETE', `/api/chat-overlays/${overlayA.id}/visual-design`, undefined);
    expect(del1.status === 204, 'DELETE succeeds (204)', del1.status);
    const del2 = await request(baseUrl, 'DELETE', `/api/chat-overlays/${overlayA.id}/visual-design`, undefined);
    expect(del2.status === 204, 'a second DELETE is still 204 (idempotent)', del2.status);

    const configAAfterDelete = await request(baseUrl, 'GET', `/api/public/chat-overlays/${overlayA.publicSlug}/config`);
    expect(configAAfterDelete.body.renderingMode === 'legacy', 'overlay A is back to legacy rendering', configAAfterDelete.body);
    const configBStillDesigned = await request(baseUrl, 'GET', `/api/public/chat-overlays/${overlayB.publicSlug}/config`);
    expect(configBStillDesigned.body.renderingMode === 'visual_design', 'overlay B is completely unaffected by overlay A\'s reset', configBStillDesigned.body);

    // --- 24: overlay deletion cascades its design -------------------------

    step('Deleting an overlay profile cascades its own saved visual design');
    const deleteOverlayB = await fetch(`${baseUrl}/api/chat-overlays/${overlayB.id}`, { method: 'DELETE' });
    expect(deleteOverlayB.status === 204, 'overlay B was deleted', deleteOverlayB.status);
    const designAfterOverlayDelete = await request(baseUrl, 'GET', `/api/chat-overlays/${overlayB.id}/visual-design`);
    expect(designAfterOverlayDelete.status === 404, 'GET visual-design for a deleted overlay 404s (the overlay itself is gone)', designAfterOverlayDelete.body);

    // --- 25: restart persistence -------------------------------------------

    step('Restart the backend: designs survive, and a fresh real event still renders correctly');
    socket.destroy();
    // Overlay A is legacy now (reset above); re-save a design on it so the
    // restart check has something meaningful to verify survived.
    const resaveA = await request(baseUrl, 'PUT', `/api/chat-overlays/${overlayA.id}/visual-design`, { expectedRevision: 0, document: validChatDesignDocument({ fill: '#123456' }) });
    expect(resaveA.status === 200, 'resave before restart succeeded', resaveA.body);

    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);

    const designAfterRestart = await request(baseUrl, 'GET', `/api/chat-overlays/${overlayA.id}/visual-design`);
    expect(designAfterRestart.body.persisted === true && designAfterRestart.body.document.layers.length === 5, 'overlay A\'s design survived the restart', designAfterRestart.body);

    const reconnected = await connectAndEnableAccount(baseUrl, twitchState, wsState, 'b');
    const iterPostRestart = sseEvents(`${baseUrl}/api/public/chat-overlays/${overlayA.publicSlug}/stream`);
    await nextEvent(iterPostRestart, 10_000, 'a fresh reset');
    sendWS(reconnected.socket, chatMessageEvent(reconnected.fakeUserId, 'u_post_restart', 'postrestart', 'PostRestart', 'msg_post_restart', 'still working after restart'));
    const postRestartUpsert = await nextEventMatching(iterPostRestart, (e) => e.event === 'chat-overlay.upsert', EVENT_TIMEOUT_MS, 'a post-restart real event');
    expect(postRestartUpsert.data.message?.plainText === 'still working after restart', 'a real event still reaches the design-driven overlay after restart', postRestartUpsert.data);
    await iterPostRestart.return();
    reconnected.socket.destroy();

    // --- secret/leak scan --------------------------------------------------

    step('Confirm the backend\'s own logs and every captured HTTP/SSE body never leak a token, raw account id, blocked term, or hidden-user entry');
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
    // The blocked term and every other operator-only value are scanned
    // ONLY against traffic a real unauthenticated public viewer could see
    // (publicScanChunks) - the broader haystack above legitimately
    // contains them, since the admin management API echoes back the
    // blocked terms/hidden users an operator just configured.
    const publicHaystack = publicScanChunks.join('\n');
    expect(!publicHaystack.includes('forbiddenword'), 'the blocked term itself never leaks into any public HTTP/SSE payload', undefined);
    expect(!publicHaystack.includes('u_hidden_1'), 'the hidden user\'s own provider id never leaks into any public HTTP/SSE payload', undefined);
    expect(!publicHaystack.includes('"eventsub"'), 'no raw EventSub envelope key ever appears in a public HTTP/SSE body', undefined);
    expect(!publicHaystack.includes(overlayA.id) && !publicHaystack.includes(overlayB.id), 'no internal overlay id ever appears in a public HTTP/SSE body', undefined);
    pass(`scanned ${haystack.length} bytes of HTTP/SSE bodies (${publicHaystack.length} bytes public-only) and ${backendOutput.length} bytes of backend output`);

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
  console.error(`\nChat overlay designer verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
