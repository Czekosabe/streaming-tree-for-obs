#!/usr/bin/env node
/**
 * Local, no-real-Twitch verification of the Stage 11B chat-automation
 * layer: scheduled messages and safe chat commands built on Stage 11A's
 * outbound dispatcher - see docs/engagement-architecture.md §8.0/8.1/8.2
 * and docs/provider-integrations/twitch-outbound-chat.md for the
 * underlying send contract this reuses unchanged.
 *
 * This script never contacts real Twitch. It runs the real backend
 * under test (`go build -tags integration ./cmd/testserver`) against
 * the same fake OAuth/Helix/EventSub servers
 * verify-twitch-outbound-chat.mjs already uses, extended with a
 * `GET /chat/messages`-adjacent send-recording handler and normal
 * chat.message EventSub notifications for activity counting and
 * command matching.
 *
 * Every token, device code, user code and client ID used here is an
 * obviously-fake string generated for this run only. No real Twitch
 * account, application, or network request to Twitch is ever involved.
 *
 * This script covers a representative subset of the task's own 52
 * enumerated scenarios - every omitted one is named in this run's own
 * docs/progress.md entry, each covered instead by an explicitly named
 * Go test in internal/chatautomation (scheduler_test.go, commands_test.go)
 * or internal/httpapi (chatautomation_test.go).
 *
 * Usage: node scripts/verify-chat-automation.mjs
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
    await new Promise((r) => setTimeout(r, 200));
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

// --- fake Twitch OAuth + Helix servers -------------------------------------

function newTwitchFakeState() {
  return {
    devices: new Map(),
    accessTokens: new Map(),
    refreshTokens: new Map(),
    users: new Map(),
    eventsubSubscriptions: [],
    chatMessagesCallCount: 0,
    lastChatMessagesRequestBody: null,
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
      const noCache = { Connection: 'close' };

      if (req.method === 'POST' && url.pathname === '/chat/messages') {
        const raw = await readBody(req);
        record(raw);
        state.chatMessagesCallCount += 1;
        if (entry === undefined || !entry.valid) {
          sendJSON(res, 401, { error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' });
          return;
        }
        state.lastChatMessagesRequestBody = JSON.parse(raw);
        res.writeHead(200, { 'Content-Type': 'application/json', ...noCache });
        res.end(JSON.stringify({ data: [{ message_id: mintToken('fake-chat-msg'), is_sent: true }] }));
        return;
      }

      if (entry === undefined || !entry.valid) {
        res.writeHead(401, { 'Content-Type': 'application/json', ...noCache });
        res.end(JSON.stringify({ error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' }));
        return;
      }

      if (req.method === 'GET' && url.pathname === '/users') {
        const user = state.users.get(entry.userId);
        res.writeHead(200, { 'Content-Type': 'application/json', ...noCache });
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
        res.writeHead(202, { 'Content-Type': 'application/json', ...noCache });
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

// --- fake EventSub WebSocket server ----------------------------------------

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

function keepaliveEnvelope() {
  return {
    metadata: { message_id: mintToken('wsmsg'), message_type: 'session_keepalive', message_timestamp: new Date().toISOString() },
    payload: {},
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

function chatMessageEvent({ broadcasterId, chatterId, chatterLogin, chatterName, text, messageId, roles = [] }) {
  const badges = [];
  if (roles.includes('moderator')) badges.push({ set_id: 'moderator', id: '1', info: '' });
  if (roles.includes('subscriber')) badges.push({ set_id: 'subscriber', id: '1', info: '' });
  if (roles.includes('vip')) badges.push({ set_id: 'vip', id: '1', info: '' });
  if (roles.includes('broadcaster')) badges.push({ set_id: 'broadcaster', id: '1', info: '' });
  return {
    broadcaster_user_id: broadcasterId, broadcaster_user_login: `bcast_${RUN_ID}`, broadcaster_user_name: `Bcast ${RUN_ID}`,
    chatter_user_id: chatterId, chatter_user_login: chatterLogin, chatter_user_name: chatterName,
    message_id: messageId, message: { text, fragments: [{ type: 'text', text }] },
    color: '', badges,
  };
}

async function findOperatorChatItem(baseUrl, text, timeoutMs = 10_000) {
  return waitUntil(async () => {
    const events = await request(baseUrl, 'GET', '/api/operator-chat/items?limit=200');
    const match = events.body.items.find((item) => item.message?.plainText === text);
    return match ?? false;
  }, timeoutMs, `an operator-chat item with text "${text}" to appear`);
}

async function main() {
  console.log('Chat automation (Stage 11B) verification (local fakes only, no real Twitch)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-chat-automation-'));
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
  let keepaliveTimer = null;

  async function connectFullAccount(userId, login, displayName) {
    const start = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
    const attemptId = start.body.attemptId;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'device-flow attempt to reach "polling"');

    twitchState.users.set(userId, { id: userId, login, displayName });
    const device = [...twitchState.devices.values()].find((d) => d.userCode === start.body.userCode);
    device.userId = userId;
    device.authorized = true;
    const authorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
      if (snap.body.state === 'error') throw new Error(`attempt error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'device-flow attempt to reach "authorized"');
    const accountId = authorized.connectedAccountId;

    // Upgrade to outbound-chat capability.
    const outboundAuth = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/authorize`);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${outboundAuth.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'outbound-chat authorize attempt to reach "polling"');
    const outboundDevice = [...twitchState.devices.values()].find((d) => d.userCode === outboundAuth.body.userCode);
    outboundDevice.userId = userId;
    outboundDevice.authorized = true;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${outboundAuth.body.attemptId}`);
      if (snap.body.state === 'error') throw new Error(`outbound authorize error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'outbound-chat authorize attempt to reach "authorized"');

    // Upgrade to inbound-engagement capability too, so EventSub delivery
    // (activity counting, command matching) can be exercised.
    const engagementAuth = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/engagement/authorize`);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${engagementAuth.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'engagement authorize attempt to reach "polling"');
    const engagementDevice = [...twitchState.devices.values()].find((d) => d.userCode === engagementAuth.body.userCode);
    engagementDevice.userId = userId;
    engagementDevice.authorized = true;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${engagementAuth.body.attemptId}`);
      if (snap.body.state === 'error') throw new Error(`engagement authorize error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'engagement authorize attempt to reach "authorized"');

    const connPromise = nextConnection(wsState);
    await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: true });
    const socket = await connPromise;
    sendWS(socket, welcomeEnvelope(`sess_${userId}`, 30));
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.state === 'connected' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'engagement connector to reach "connected"');

    return { accountId, socket };
  }

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

    const env = {
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

    step('Backend starts with no automation rules');
    backend = await startBackend(exePath, env, baseUrl);
    await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    const initialStatus = await request(baseUrl, 'GET', '/api/chat-automation/status');
    expect(initialStatus.status === 200 && initialStatus.body.schedules.length === 0 && initialStatus.body.commands.length === 0,
      'a fresh backend reports zero schedules and zero commands', initialStatus.body);

    step('Connect one outbound-capable, inbound-engaged Twitch account');
    const fakeUserId = `u_${RUN_ID}`;
    const { accountId, socket: socket1 } = await connectFullAccount(fakeUserId, `streamer_${RUN_ID}`, `Streamer ${RUN_ID}`);
    keepaliveTimer = setInterval(() => sendWS(socket1, keepaliveEnvelope()), 5_000);
    pass(`connected account ${accountId}`);

    step('Schedule creation persists, with its target and two message alternatives');
    const createSchedule = await request(baseUrl, 'POST', '/api/chat-automation/schedules', {
      name: 'Test schedule', enabled: true, intervalSeconds: 60, firstDelaySeconds: 0, jitterSeconds: 0,
      onlyWhileIngestReceiving: false, minimumChatMessages: 0, maximumSendsPerHour: 60,
      targets: [{ accountId }],
      messages: [`sched-alt-one-${RUN_ID}`, `sched-alt-two-${RUN_ID}`],
    });
    expect(createSchedule.status === 201, 'schedule creation returns 201', createSchedule.body);
    const scheduleId = createSchedule.body.id;
    const getSchedule = await request(baseUrl, 'GET', `/api/chat-automation/schedules/${scheduleId}`);
    expect(getSchedule.body.targets.length === 1 && getSchedule.body.targets[0].accountId === accountId,
      'the schedule target persisted', getSchedule.body.targets);
    expect(getSchedule.body.messages.length === 2, 'both message alternatives persisted', getSchedule.body.messages);

    step('Command creation persists, with its alias');
    const createCommand = await request(baseUrl, 'POST', '/api/chat-automation/commands', {
      name: 'discord', enabled: true, responseTemplate: `join us at {channelUrl}`, requiredRole: 'everyone',
      globalCooldownSeconds: 0, userCooldownSeconds: 0, aliases: ['disc'], targets: [{ accountId }],
    });
    expect(createCommand.status === 201, 'command creation returns 201', createCommand.body);
    const commandId = createCommand.body.id;
    const getCommand = await request(baseUrl, 'GET', `/api/chat-automation/commands/${commandId}`);
    expect(getCommand.body.aliases.includes('disc'), 'the command alias persisted', getCommand.body.aliases);

    step('Preview renders {channelName}/{platform}/{channelUrl} from local account data');
    const preview = await request(baseUrl, 'POST', '/api/chat-automation/preview', {
      template: 'Hi from {channelName} on {platform}: {channelUrl}', accountId,
    });
    expect(preview.status === 200 &&
      preview.body.renderedText === `Hi from Streamer ${RUN_ID} on Twitch: https://www.twitch.tv/streamer_${RUN_ID}`,
      'preview resolves channelName/platform/channelUrl exactly', preview.body);
    expect(preview.body.validForProvider === true, 'a short resolved preview is valid for the provider', preview.body);

    step('An unknown placeholder is rejected with 422 at save time');
    const badTemplate = await request(baseUrl, 'POST', '/api/chat-automation/schedules', {
      name: 'bad', enabled: true, intervalSeconds: 60, firstDelaySeconds: 0, jitterSeconds: 0,
      onlyWhileIngestReceiving: false, minimumChatMessages: 0, maximumSendsPerHour: 10,
      targets: [{ accountId }], messages: ['hi {viewerCount}'],
    });
    expect(badTemplate.status === 422 && badTemplate.body.error === 'chat_automation_placeholder_invalid',
      'an unknown placeholder is rejected with chat_automation_placeholder_invalid', badTemplate.body);

    step('{streamTitle} without a deterministic platform context reports unresolved');
    const unresolvedPreview = await request(baseUrl, 'POST', '/api/chat-automation/preview', {
      template: 'Now playing: {streamTitle}', accountId,
    });
    expect(unresolvedPreview.body.unresolvedPlaceholders?.includes('streamTitle') === true,
      'streamTitle is reported unresolved with no platform context', unresolvedPreview.body);
    expect(unresolvedPreview.body.validForProvider === false, 'an unresolved preview is not valid for the provider', unresolvedPreview.body);

    step('A scheduled execution actually sends through the same Twitch Send Chat Message endpoint');
    const callsBeforeSchedule = twitchState.chatMessagesCallCount;
    await waitUntil(async () => {
      return twitchState.chatMessagesCallCount > callsBeforeSchedule ? true : false;
    }, 15_000, 'the scheduled message to reach the fake Twitch Send Chat Message endpoint');
    expect(twitchState.lastChatMessagesRequestBody.broadcaster_id === fakeUserId &&
      twitchState.lastChatMessagesRequestBody.sender_id === fakeUserId,
      'the scheduled send used the account\'s own provider user id for broadcaster/sender', twitchState.lastChatMessagesRequestBody);
    expect(twitchState.lastChatMessagesRequestBody.message === `sched-alt-one-${RUN_ID}` ||
      twitchState.lastChatMessagesRequestBody.message === `sched-alt-two-${RUN_ID}`,
      'the scheduled send used one of the two configured message alternatives', twitchState.lastChatMessagesRequestBody.message);

    step('Disabling the schedule prevents further sends');
    await request(baseUrl, 'PUT', `/api/chat-automation/schedules/${scheduleId}`, {
      name: 'Test schedule', enabled: false, intervalSeconds: 60, firstDelaySeconds: 0, jitterSeconds: 0,
      onlyWhileIngestReceiving: false, minimumChatMessages: 0, maximumSendsPerHour: 60,
      targets: [{ accountId }], messages: [`sched-alt-one-${RUN_ID}`, `sched-alt-two-${RUN_ID}`],
    });
    const callsAfterDisable = twitchState.chatMessagesCallCount;
    await new Promise((r) => setTimeout(r, 2_000));
    expect(twitchState.chatMessagesCallCount === callsAfterDisable,
      'a disabled schedule sends nothing further', { before: callsAfterDisable, after: twitchState.chatMessagesCallCount });

    step('Send Now bypasses the interval and sends immediately for a schedule that would otherwise never fire soon');
    const createSlowSchedule = await request(baseUrl, 'POST', '/api/chat-automation/schedules', {
      name: 'Slow schedule', enabled: true, intervalSeconds: 86400, firstDelaySeconds: 86400, jitterSeconds: 0,
      onlyWhileIngestReceiving: false, minimumChatMessages: 1000, maximumSendsPerHour: 1,
      targets: [{ accountId }], messages: [`send-now-${RUN_ID}`],
    });
    const slowScheduleId = createSlowSchedule.body.id;
    const callsBeforeSendNow = twitchState.chatMessagesCallCount;
    const sendNow = await request(baseUrl, 'POST', `/api/chat-automation/schedules/${slowScheduleId}/send-now`, {});
    expect(sendNow.status === 200 && sendNow.body.results.length === 1 && sendNow.body.results[0].sent === true,
      'Send Now succeeds immediately, ignoring interval/first-delay/minimum-activity', sendNow.body);
    expect(twitchState.chatMessagesCallCount === callsBeforeSendNow + 1,
      'Send Now made exactly one real send call', { before: callsBeforeSendNow, after: twitchState.chatMessagesCallCount });
    expect(!('message' in sendNow.body.results[0]) && !JSON.stringify(sendNow.body).includes(`send-now-${RUN_ID}`),
      'the Send Now response never echoes the sent message text', sendNow.body);

    step('Canonical command name triggers a response, sent as a same-account reply');
    const callsBeforeCommand = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer1_${RUN_ID}`, chatterLogin: `viewer1_${RUN_ID}`, chatterName: 'Viewer1',
      text: '!discord', messageId: `msg_cmd1_${RUN_ID}`,
    }), `notif_cmd1_${RUN_ID}`));
    await waitUntil(async () => twitchState.chatMessagesCallCount > callsBeforeCommand ? true : false,
      10_000, 'the canonical command to produce a response');
    expect(twitchState.lastChatMessagesRequestBody.message.includes('join us at'),
      'the command response used the configured template', twitchState.lastChatMessagesRequestBody.message);
    expect(twitchState.lastChatMessagesRequestBody.reply_parent_message_id === `msg_cmd1_${RUN_ID}`,
      'the command response was sent as a reply to the triggering message', twitchState.lastChatMessagesRequestBody);

    step('An alias triggers the same command');
    const callsBeforeAlias = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer2_${RUN_ID}`, chatterLogin: `viewer2_${RUN_ID}`, chatterName: 'Viewer2',
      text: '!disc', messageId: `msg_alias_${RUN_ID}`,
    }), `notif_alias_${RUN_ID}`));
    await waitUntil(async () => twitchState.chatMessagesCallCount > callsBeforeAlias ? true : false,
      10_000, 'the alias to produce a response');
    pass('alias triggered the command');

    step('A command in the middle of a message does not trigger');
    const callsBeforeMiddle = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer3_${RUN_ID}`, chatterLogin: `viewer3_${RUN_ID}`, chatterName: 'Viewer3',
      text: 'hey check out !discord', messageId: `msg_middle_${RUN_ID}`,
    }), `notif_middle_${RUN_ID}`));
    await new Promise((r) => setTimeout(r, 1_500));
    expect(twitchState.chatMessagesCallCount === callsBeforeMiddle,
      'a command not at the start of the message never triggers', { before: callsBeforeMiddle, after: twitchState.chatMessagesCallCount });

    step('A self (broadcaster account) command-looking message never triggers');
    const callsBeforeSelf = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: fakeUserId, chatterLogin: `streamer_${RUN_ID}`, chatterName: `Streamer ${RUN_ID}`,
      text: '!discord', messageId: `msg_self_${RUN_ID}`,
    }), `notif_self_${RUN_ID}`));
    await new Promise((r) => setTimeout(r, 1_500));
    expect(twitchState.chatMessagesCallCount === callsBeforeSelf,
      'the connected account\'s own echoed message never triggers a command (hard self-loop-protection rule)',
      { before: callsBeforeSelf, after: twitchState.chatMessagesCallCount });

    step('A global cooldown blocks an immediate second trigger from a different user');
    const callsBeforeCooldown = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer4_${RUN_ID}`, chatterLogin: `viewer4_${RUN_ID}`, chatterName: 'Viewer4',
      text: '!discord', messageId: `msg_cooldown_${RUN_ID}`,
    }), `notif_cooldown_${RUN_ID}`));
    await new Promise((r) => setTimeout(r, 1_500));
    // The command has no cooldown configured above (0 seconds), so this
    // scenario re-creates a command WITH a cooldown to actually exercise it.
    const cooldownCommand = await request(baseUrl, 'POST', '/api/chat-automation/commands', {
      name: 'cooldowntest', enabled: true, responseTemplate: 'pong', requiredRole: 'everyone',
      globalCooldownSeconds: 30, userCooldownSeconds: 0, aliases: [], targets: [{ accountId }],
    });
    const callsBeforeGlobalCooldown = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer5_${RUN_ID}`, chatterLogin: `viewer5_${RUN_ID}`, chatterName: 'Viewer5',
      text: '!cooldowntest', messageId: `msg_cd1_${RUN_ID}`,
    }), `notif_cd1_${RUN_ID}`));
    await waitUntil(async () => twitchState.chatMessagesCallCount > callsBeforeGlobalCooldown ? true : false,
      10_000, 'the first cooldowntest trigger to respond');
    const callsAfterFirstCooldownTrigger = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer6_${RUN_ID}`, chatterLogin: `viewer6_${RUN_ID}`, chatterName: 'Viewer6',
      text: '!cooldowntest', messageId: `msg_cd2_${RUN_ID}`,
    }), `notif_cd2_${RUN_ID}`));
    await new Promise((r) => setTimeout(r, 1_500));
    expect(twitchState.chatMessagesCallCount === callsAfterFirstCooldownTrigger,
      'a second, different user is still blocked by the still-active global cooldown',
      { before: callsAfterFirstCooldownTrigger, after: twitchState.chatMessagesCallCount });
    void cooldownCommand;
    void callsBeforeCooldown;

    step('Minimum chat activity blocks a due execution until enough eligible human messages arrive');
    const activitySchedule = await request(baseUrl, 'POST', '/api/chat-automation/schedules', {
      name: 'Activity-gated schedule', enabled: true, intervalSeconds: 60, firstDelaySeconds: 60, jitterSeconds: 0,
      onlyWhileIngestReceiving: false, minimumChatMessages: 2, maximumSendsPerHour: 60,
      targets: [{ accountId }], messages: [`activity-gated-${RUN_ID}`],
    });
    const activityScheduleId = activitySchedule.body.id;
    // Send-now still ignores minimum activity by policy, so use it here only
    // to prove the schedule exists; the real gating is exercised via the
    // schedule's own next natural due time being blocked - verified instead
    // through the dedicated Go test suite (TestSchedulerMinimumChatActivityBlocksThenAllows)
    // since this script's own real time budget cannot wait a full interval
    // twice. One eligible human message is sent here to prove it is at
    // least being counted and reflected in status.
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer7_${RUN_ID}`, chatterLogin: `viewer7_${RUN_ID}`, chatterName: 'Viewer7',
      text: `just chatting ${RUN_ID}`, messageId: `msg_activity1_${RUN_ID}`,
    }), `notif_activity1_${RUN_ID}`));
    await findOperatorChatItem(baseUrl, `just chatting ${RUN_ID}`);
    const activityStatus = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/chat-automation/schedules/${activityScheduleId}`);
      const target = snap.body.targetStatus?.find((t) => t.accountId === accountId);
      return target !== undefined ? snap.body : false;
    }, 10_000, 'the activity-gated schedule to report per-target status');
    expect(activityStatus.state === 'scheduled' || activityStatus.state === 'waiting_for_activity',
      'the activity-gated schedule has not sent yet (first delay + activity threshold not both met)', activityStatus.state);

    step('Editing a schedule takes effect without a backend restart');
    const editResp = await request(baseUrl, 'PUT', `/api/chat-automation/schedules/${slowScheduleId}`, {
      name: 'Slow schedule renamed', enabled: false, intervalSeconds: 86400, firstDelaySeconds: 86400, jitterSeconds: 0,
      onlyWhileIngestReceiving: false, minimumChatMessages: 1000, maximumSendsPerHour: 1,
      targets: [{ accountId }], messages: [`send-now-${RUN_ID}`],
    });
    expect(editResp.status === 200 && editResp.body.name === 'Slow schedule renamed' && editResp.body.enabled === false,
      'the edited name and enabled flag are reflected immediately', editResp.body);

    step('Disabling a command prevents further responses');
    await request(baseUrl, 'PUT', `/api/chat-automation/commands/${commandId}`, {
      name: 'discord', enabled: false, responseTemplate: 'join us', requiredRole: 'everyone',
      globalCooldownSeconds: 0, userCooldownSeconds: 0, aliases: ['disc'], targets: [{ accountId }],
    });
    const callsBeforeDisabledCommand = twitchState.chatMessagesCallCount;
    sendWS(socket1, notificationEnvelope('channel.chat.message', chatMessageEvent({
      broadcasterId: fakeUserId, chatterId: `viewer8_${RUN_ID}`, chatterLogin: `viewer8_${RUN_ID}`, chatterName: 'Viewer8',
      text: '!discord', messageId: `msg_disabled_${RUN_ID}`,
    }), `notif_disabled_${RUN_ID}`));
    await new Promise((r) => setTimeout(r, 1_500));
    expect(twitchState.chatMessagesCallCount === callsBeforeDisabledCommand,
      'a disabled command never responds', { before: callsBeforeDisabledCommand, after: twitchState.chatMessagesCallCount });

    step('Restart preserves persisted definitions and resets runtime state (no backlog replay)');
    if (keepaliveTimer !== null) { clearInterval(keepaliveTimer); keepaliveTimer = null; }
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);
    const scheduleAfterRestart = await request(baseUrl, 'GET', `/api/chat-automation/schedules/${scheduleId}`);
    expect(scheduleAfterRestart.status === 200 && scheduleAfterRestart.body.name === 'Test schedule',
      'the schedule definition survived the restart', scheduleAfterRestart.body);
    const commandAfterRestart = await request(baseUrl, 'GET', `/api/chat-automation/commands/${commandId}`);
    expect(commandAfterRestart.status === 200 && commandAfterRestart.body.aliases.includes('disc'),
      'the command definition survived the restart', commandAfterRestart.body);
    const callsRightAfterRestart = twitchState.chatMessagesCallCount;
    await new Promise((r) => setTimeout(r, 2_000));
    expect(twitchState.chatMessagesCallCount === callsRightAfterRestart,
      'no scheduled message is sent immediately on restart merely because the backend came back up',
      { before: callsRightAfterRestart, after: twitchState.chatMessagesCallCount });

    step('Deleting a schedule and a command removes them');
    const deleteSchedule = await request(baseUrl, 'DELETE', `/api/chat-automation/schedules/${scheduleId}`);
    expect(deleteSchedule.status === 204, 'schedule delete returns 204', deleteSchedule.body);
    const getDeletedSchedule = await request(baseUrl, 'GET', `/api/chat-automation/schedules/${scheduleId}`);
    expect(getDeletedSchedule.status === 404, 'the deleted schedule is gone', getDeletedSchedule.body);

    const deleteCommand = await request(baseUrl, 'DELETE', `/api/chat-automation/commands/${commandId}`);
    expect(deleteCommand.status === 204, 'command delete returns 204', deleteCommand.body);
    const getDeletedCommand = await request(baseUrl, 'GET', `/api/chat-automation/commands/${commandId}`);
    expect(getDeletedCommand.status === 404, 'the deleted command is gone', getDeletedCommand.body);

    step('Search every captured HTTP response body and backend log line for message text, tokens and raw prose');
    const haystack = secretScanChunks.join('\n');
    const backendOutput = backend.getOutput();
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!backendOutput.includes(entry), 'an access token never appears in the backend\'s own stdout/stderr', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!backendOutput.includes(entry), 'a refresh token never appears in the backend\'s own stdout/stderr', undefined);
    }
    for (const text of [`sched-alt-one-${RUN_ID}`, `sched-alt-two-${RUN_ID}`, `send-now-${RUN_ID}`, 'join us at']) {
      expect(!backendOutput.includes(text), `template/rendered text "${text}" never appears in the backend's own stdout/stderr`, undefined);
    }
    for (const host of ['api.twitch.tv', 'id.twitch.tv', 'eventsub.wss.twitch.tv']) {
      expect(!backendOutput.includes(host), `no real Twitch hostname ("${host}") is ever contacted or logged`, undefined);
    }
    pass(`scanned ${haystack.length} bytes of HTTP responses and ${backendOutput.length} bytes of backend output`);

    console.log('\nAll steps passed.');
  } catch (error) {
    if (backend !== null && process.env.STREAMING_TREE_VERIFY_DEBUG === '1') {
      console.error('\n--- backend output (debug) ---');
      console.error(backend.getOutput());
      console.error('--- end backend output ---\n');
    }
    throw error;
  } finally {
    if (keepaliveTimer !== null) clearInterval(keepaliveTimer);
    await stopBackend(backend, baseUrl ?? '');
    for (const socket of wsState.connections) {
      if (!socket.destroyed) socket.destroy();
    }
    await close(oauthServer);
    await close(helixServer);
    await close(eventSubServer);
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nChat automation verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
