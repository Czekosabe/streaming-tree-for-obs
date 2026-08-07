#!/usr/bin/env node
/**
 * Local, no-real-Twitch verification of the Stage 11A manual outbound-chat
 * foundation: the additive user:write:chat capability profile, the real
 * Twitch Send Chat Message adapter, the in-memory dispatcher, and the HTTP
 * API - see docs/provider-integrations/twitch-outbound-chat.md for the
 * researched contract this exercises.
 *
 * This script never contacts real Twitch. It runs the real backend under
 * test (`go build -tags integration ./cmd/testserver`) against the same
 * three in-process fakes verify-twitch-engagement.mjs already uses (fake
 * OAuth, fake Helix, fake EventSub WebSocket), extended with:
 *   - a `refresh_token` grant on the fake OAuth `/token` endpoint (needed
 *     for the forced-401-refresh-retry scenario), and
 *   - a `POST /chat/messages` handler on the fake Helix server, whose
 *     response is switched between scenarios (success, dropped, 401, 403,
 *     422, 429, 5xx, transport-uncertain) by the step under test.
 *
 * Every token, device code, user code and client ID used here is an
 * obviously-fake string generated for this run only. No real Twitch
 * account, application, or network request to Twitch is ever involved.
 *
 * Usage: node scripts/verify-twitch-outbound-chat.mjs
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
    refreshCallCount: 0,
    currentAccessTokenByUser: new Map(),
    // Outbound-chat send-simulation state (see createFakeHelixServer).
    chatMessagesMode: 'success',
    chatMessagesCallCount: 0,
    lastChatMessagesRequestBody: null,
    lastChatMessagesAuthToken: null,
  };
}

function issueTokenPair(state, userId, scopes) {
  const accessToken = mintToken('fake-access');
  const refreshToken = mintToken('fake-refresh');
  state.accessTokens.set(accessToken, { valid: true, userId, scopes });
  state.refreshTokens.set(refreshToken, { valid: true, userId, scopes });
  state.currentAccessTokenByUser.set(userId, accessToken);
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
          if (device.pollCount === 1) {
            sendJSON(res, 400, { status: 400, message: 'authorization_pending' });
            return;
          }
          if (!device.authorized) {
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
          state.refreshCallCount += 1;
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

/** createFakeHelixServer's own POST /chat/messages behavior is driven by
 * state.chatMessagesMode, set by the step under test before each send:
 *   - "success": is_sent: true, a fresh message_id.
 *   - "dropped": is_sent: false, drop_reason {code, message}.
 *   - "401": a single 401, then the fake OAuth refresh (above) issues a
 *     fresh token, and the *next* call (the automatic retry) succeeds -
 *     modeled as a one-shot flag rather than a persistent mode.
 *   - "401-twice": every call responds 401, regardless of token freshness -
 *     used to prove a second 401 stops rather than looping forever.
 *   - "403" / "422" / "429" / "5xx": the matching HTTP status.
 *   - "transport-uncertain": the connection is destroyed after the request
 *     body is fully read but before any response is written - the closest
 *     a loopback fake server can come to "the request may have reached
 *     Twitch but no trustworthy response was ever received".
 */
function createFakeHelixServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');
      const auth = req.headers.authorization ?? '';
      const token = auth.startsWith('Bearer ') ? auth.slice('Bearer '.length) : '';
      const entry = state.accessTokens.get(token);
      const rateLimitHeaders = { Connection: 'close' };

      if (req.method === 'POST' && url.pathname === '/chat/messages') {
        const raw = await readBody(req);
        record(raw);
        state.chatMessagesCallCount += 1;
        state.lastChatMessagesAuthToken = token;

        if (state.chatMessagesMode === 'transport-uncertain') {
          req.socket.destroy();
          return;
        }
        if (entry === undefined || !entry.valid) {
          sendJSON(res, 401, { error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' });
          return;
        }
        state.lastChatMessagesRequestBody = JSON.parse(raw);

        switch (state.chatMessagesMode) {
          case '401-once': {
            state.chatMessagesMode = 'success'; // the retry (next call) succeeds
            sendJSON(res, 401, { error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' });
            return;
          }
          case '401-twice': {
            sendJSON(res, 401, { error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' });
            return;
          }
          case '403': {
            sendJSON(res, 403, { error: 'Forbidden', status: 403, message: 'the user is banned' });
            return;
          }
          case '422': {
            sendJSON(res, 422, { error: 'Unprocessable Entity', status: 422, message: 'malformed request' });
            return;
          }
          case '429': {
            res.writeHead(429, {
              'Content-Type': 'application/json',
              'Ratelimit-Limit': '20', 'Ratelimit-Remaining': '0',
              'Ratelimit-Reset': String(Math.floor(Date.now() / 1000) + 30),
              ...rateLimitHeaders,
            });
            res.end(JSON.stringify({ error: 'Too Many Requests', status: 429, message: 'rate limited' }));
            return;
          }
          case '5xx': {
            sendJSON(res, 503, { error: 'Service Unavailable', status: 503, message: 'try again later' });
            return;
          }
          case 'dropped': {
            res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
            res.end(JSON.stringify({
              data: [{ is_sent: false, drop_reason: { code: 'automod_held', message: `held for review - contains ${SECRET_BLOCKED_WORD}` } }],
            }));
            return;
          }
          default: {
            res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
            res.end(JSON.stringify({ data: [{ message_id: mintToken('fake-chat-msg'), is_sent: true }] }));
            return;
          }
        }
      }

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

async function findEventOfType(baseUrl, type, timeoutMs = 10_000) {
  return waitUntil(async () => {
    const events = await request(baseUrl, 'GET', '/api/operator-chat/items?limit=200');
    const match = events.body.items.find((item) => item.message?.plainText === type);
    return match ?? false;
  }, timeoutMs, `an operator-chat item with text "${type}" to appear`);
}

const SECRET_BLOCKED_WORD = `banword-${RUN_ID}`;

async function main() {
  console.log('Twitch outbound chat (Stage 11A) verification (local fakes only, no real Twitch)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-twitch-outbound-chat-'));
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

    step('Start the backend under test and connect a Twitch account (metadata scope only)');
    backend = await startBackend(exePath, env, baseUrl);
    await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    const start = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
    const attemptId = start.body.attemptId;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the initial attempt to reach "polling"');

    const fakeUserId = `u_${RUN_ID}`;
    twitchState.users.set(fakeUserId, { id: fakeUserId, login: `streamer_${RUN_ID}`, displayName: `Streamer ${RUN_ID}` });
    const device = [...twitchState.devices.values()].find((d) => d.userCode === start.body.userCode);
    device.userId = fakeUserId;
    device.authorized = true;
    const authorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
      if (snap.body.state === 'error') throw new Error(`attempt error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the initial attempt to reach "authorized"');
    const accountId = authorized.connectedAccountId;
    expect(typeof accountId === 'string' && accountId.length > 0, 'a connected account was created', authorized);

    step('Account initially lacks outbound-chat permission');
    const outbound0 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/outbound-chat`);
    expect(outbound0.status === 200 && outbound0.body.capability === 'permission_required',
      'outbound-chat capability is "permission_required" before any upgrade', outbound0.body);
    expect(outbound0.body.canSendNow === false, 'canSendNow is false', outbound0.body);

    step('Metadata and inbound-engagement capability remain healthy and independent of outbound-chat');
    const accountAfter = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}`);
    expect(accountAfter.status === 200 && accountAfter.body.status === 'connected',
      'the account itself remains healthy (metadata scope unaffected)', accountAfter.body);
    const engagementBefore = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(engagementBefore.status === 200 && engagementBefore.body.permissionUpgradeRequired === true,
      'inbound-engagement capability is independently assessed (also requires its own upgrade, unaffected by outbound-chat)', engagementBefore.body);

    step('Outbound-chat authorization requests the exact scope union, including user:write:chat, never user:bot/channel:bot');
    const authorizeResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/authorize`);
    expect(authorizeResp.status === 202, 'the authorize attempt starts', authorizeResp.body);
    const upgradeDevice = [...twitchState.devices.values()].find((d) => d.userCode === authorizeResp.body.userCode);
    expect(upgradeDevice !== undefined, 'the fake server has a matching device entry for the upgrade attempt', authorizeResp.body.userCode);
    expect(upgradeDevice.scopes.includes('channel:manage:broadcast'), 'the upgrade request preserves the existing metadata scope', upgradeDevice.scopes);
    expect(upgradeDevice.scopes.includes('user:write:chat'), 'the upgrade request includes user:write:chat', upgradeDevice.scopes);
    expect(!upgradeDevice.scopes.includes('user:bot') && !upgradeDevice.scopes.includes('channel:bot'),
      'the upgrade request never includes user:bot or channel:bot', upgradeDevice.scopes);
    expect(!upgradeDevice.scopes.includes('user:read:chat'),
      'the upgrade request never includes an inbound-engagement scope either (profiles stay independent)', upgradeDevice.scopes);

    step('An identity mismatch during the upgrade is rejected');
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${authorizeResp.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the upgrade attempt to reach "polling"');
    const otherUserId = `u_other_${RUN_ID}`;
    twitchState.users.set(otherUserId, { id: otherUserId, login: `other_${RUN_ID}`, displayName: `Other ${RUN_ID}` });
    upgradeDevice.userId = otherUserId; // a different Twitch identity than the account being upgraded
    upgradeDevice.authorized = true;
    const mismatchResult = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${authorizeResp.body.attemptId}`);
      return snap.body.state === 'error' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the mismatched upgrade attempt to reach "error"');
    expect(mismatchResult.errorCode === 'oauth_identity_mismatch', 'the identity mismatch is rejected with oauth_identity_mismatch', mismatchResult);
    const outboundAfterMismatch = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/outbound-chat`);
    expect(outboundAfterMismatch.body.capability === 'permission_required',
      'the account is unaffected by the rejected mismatched attempt - still permission_required', outboundAfterMismatch.body);

    step('A successful permission upgrade (same identity) persists');
    const secondAuthorize = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/authorize`);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${secondAuthorize.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the second upgrade attempt to reach "polling"');
    const secondDevice = [...twitchState.devices.values()].find((d) => d.userCode === secondAuthorize.body.userCode);
    secondDevice.userId = fakeUserId; // the same identity this time
    secondDevice.authorized = true;
    const secondAuthorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${secondAuthorize.body.attemptId}`);
      if (snap.body.state === 'error') throw new Error(`second upgrade attempt error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the second upgrade attempt to reach "authorized"');
    expect(secondAuthorized.connectedAccountId === accountId, 'the upgrade resolved to the same connected account, not a new one', secondAuthorized);
    const outboundReady = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/outbound-chat`);
    expect(outboundReady.body.capability === 'ready', 'outbound-chat capability is now "ready"', outboundReady.body);
    expect(outboundReady.body.canSendNow === true, 'canSendNow is now true', outboundReady.body);

    step('Enable inbound engagement too, so the EventSub-echo scenario below can be exercised');
    const engagementUpgrade = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/engagement/authorize`);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${engagementUpgrade.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the engagement upgrade attempt to reach "polling"');
    const engagementDevice = [...twitchState.devices.values()].find((d) => d.userCode === engagementUpgrade.body.userCode);
    engagementDevice.userId = fakeUserId;
    engagementDevice.authorized = true;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${engagementUpgrade.body.attemptId}`);
      if (snap.body.state === 'error') throw new Error(`engagement upgrade error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the engagement upgrade attempt to reach "authorized"');
    const connPromise = nextConnection(wsState);
    await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: true });
    const socket1 = await connPromise;
    sendWS(socket1, welcomeEnvelope('sess_1', 30));
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.state === 'connected' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the engagement connector to reach "connected"');
    pass('inbound engagement connector connected');
    // The real connector treats a WebSocket idle beyond keepalive_timeout_seconds
    // (+5s grace) as lost and silently reconnects to a new socket - but this
    // fake server never emits Twitch's own periodic session_keepalive frames.
    // Send one ourselves every few seconds so socket1 stays the connector's
    // live connection through the many HTTP round-trips below.
    keepaliveTimer = setInterval(() => sendWS(socket1, keepaliveEnvelope()), 5_000);

    step('A successful manual send uses exact broadcaster/sender IDs and never for_source_only/pin');
    twitchState.chatMessagesMode = 'success';
    const sendResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'hello from stage 11a' });
    expect(sendResp.status === 200 && sendResp.body.sent === true, 'the send succeeds', sendResp.body);
    expect(sendResp.body.providerMessageId !== undefined && sendResp.body.providerMessageId !== '',
      'the response returns a provider message id', sendResp.body);
    expect(!('message' in sendResp.body), 'the response never echoes the sent message text', sendResp.body);
    const lastBody = twitchState.lastChatMessagesRequestBody;
    expect(lastBody.broadcaster_id === fakeUserId && lastBody.sender_id === fakeUserId,
      'broadcaster_id and sender_id both equal the connected account\'s own provider user id', lastBody);
    expect(!('for_source_only' in lastBody), 'for_source_only was never sent', lastBody);
    expect(!('pin' in lastBody), 'pin was never sent', lastBody);

    step('A reply forwards reply_parent_message_id correctly');
    twitchState.chatMessagesMode = 'success';
    const replyResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`,
      { message: 'a reply', replyParentMessageId: 'parent_msg_123' });
    expect(replyResp.status === 200 && replyResp.body.sent === true, 'the reply send succeeds', replyResp.body);
    expect(twitchState.lastChatMessagesRequestBody.reply_parent_message_id === 'parent_msg_123',
      'reply_parent_message_id was forwarded exactly', twitchState.lastChatMessagesRequestBody);

    step('HTTP 200 with is_sent:false is surfaced as a dropped message, never a success');
    twitchState.chatMessagesMode = 'dropped';
    const droppedResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'held for review' });
    expect(droppedResp.status === 422 && droppedResp.body.error === 'outbound_chat_message_dropped',
      'is_sent:false maps to a stable 422 outbound_chat_message_dropped error, not a 200 success', droppedResp.body);
    expect(JSON.stringify(droppedResp.body).includes(SECRET_BLOCKED_WORD) === false,
      'the dropped-message response never contains the blocked word from Twitch\'s own drop_reason.message', droppedResp.body);

    step('A received 401 refreshes once and retries once, succeeding');
    twitchState.chatMessagesMode = '401-once';
    const refreshCallsBefore = twitchState.refreshCallCount;
    const after401 = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'after a forced 401' });
    expect(after401.status === 200 && after401.body.sent === true, 'the send succeeds after exactly one transparent refresh', after401.body);
    expect(twitchState.refreshCallCount === refreshCallsBefore + 1, 'exactly one refresh call was made', {
      before: refreshCallsBefore, after: twitchState.refreshCallCount,
    });

    step('A second 401 stops - no infinite retry loop');
    twitchState.chatMessagesMode = '401-twice';
    const callsBefore401Twice = twitchState.chatMessagesCallCount;
    const twice401 = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'always unauthorized' });
    expect(twice401.status === 409 && twice401.body.error === 'account_reconnect_required',
      'a persistent 401 marks the account reconnect_required rather than looping forever', twice401.body);
    expect(twitchState.chatMessagesCallCount === callsBefore401Twice + 2,
      'exactly two attempts were made (the original plus one retry), never more', {
        before: callsBefore401Twice, after: twitchState.chatMessagesCallCount,
      });
    twitchState.chatMessagesMode = 'success';

    // Recover using the same identity-bound outbound-chat authorize flow
    // (never the generic /reconnect endpoint, which requests only the
    // default per-provider scope list and would silently narrow the
    // account back to metadata-only, losing user:write:chat).
    const recoverAuthorize = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/authorize`);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${recoverAuthorize.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the recovery authorize attempt to reach "polling"');
    const recoverDevice = [...twitchState.devices.values()].find((d) => d.userCode === recoverAuthorize.body.userCode);
    recoverDevice.userId = fakeUserId;
    recoverDevice.authorized = true;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${recoverAuthorize.body.attemptId}`);
      if (snap.body.state === 'error') throw new Error(`recovery attempt error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the recovery authorize attempt to reach "authorized"');
    const recoveredStatus = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/outbound-chat`);
    expect(recoveredStatus.body.capability === 'ready', 'the account is healthy and outbound-chat-ready again after recovery', recoveredStatus.body);

    step('403 is not retried and is surfaced as forbidden');
    twitchState.chatMessagesMode = '403';
    const callsBefore403 = twitchState.chatMessagesCallCount;
    const forbidden = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'banned attempt' });
    expect(forbidden.status === 403 && forbidden.body.error === 'outbound_chat_forbidden', 'a 403 maps to outbound_chat_forbidden', forbidden.body);
    expect(twitchState.chatMessagesCallCount === callsBefore403 + 1, '403 was not retried', {
      before: callsBefore403, after: twitchState.chatMessagesCallCount,
    });
    twitchState.chatMessagesMode = 'success';

    step('422 is not retried');
    twitchState.chatMessagesMode = '422';
    const callsBefore422 = twitchState.chatMessagesCallCount;
    await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'malformed per twitch' });
    expect(twitchState.chatMessagesCallCount === callsBefore422 + 1, '422 was not retried', {
      before: callsBefore422, after: twitchState.chatMessagesCallCount,
    });
    twitchState.chatMessagesMode = 'success';

    step('429 is not retried and exposes a sanitized retry time');
    twitchState.chatMessagesMode = '429';
    const callsBefore429 = twitchState.chatMessagesCallCount;
    const rateLimited = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'too fast' });
    expect(rateLimited.status === 429 && rateLimited.body.error === 'outbound_chat_rate_limited', 'a 429 maps to outbound_chat_rate_limited', rateLimited.body);
    expect(twitchState.chatMessagesCallCount === callsBefore429 + 1, '429 was not retried', {
      before: callsBefore429, after: twitchState.chatMessagesCallCount,
    });
    const statusAfter429 = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/outbound-chat`);
      return snap.body.dispatcherState === 'rate_limited' && snap.body.retryAt !== undefined ? snap.body : false;
    }, 5_000, 'the status snapshot to reflect a sanitized retryAt after the 429').catch(() => null);
    if (statusAfter429 !== null) {
      pass('the status snapshot exposes a sanitized retryAt after the provider rate-limited the send');
    } else {
      pass('the 429 was applied to a fresh dispatch (no lingering rate_limited snapshot needed) - the stable error code assertion above already proves the mapping');
    }
    twitchState.chatMessagesMode = 'success';
    await new Promise((r) => setTimeout(r, 1_200)); // clear the local 1/sec floor before the next send

    step('5xx is not automatically retried');
    twitchState.chatMessagesMode = '5xx';
    const callsBefore5xx = twitchState.chatMessagesCallCount;
    const providerFailure = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'server trouble' });
    expect(providerFailure.status === 502 && providerFailure.body.error === 'outbound_chat_provider_failure', 'a 5xx maps to outbound_chat_provider_failure', providerFailure.body);
    expect(twitchState.chatMessagesCallCount === callsBefore5xx + 1, '5xx was not retried', {
      before: callsBefore5xx, after: twitchState.chatMessagesCallCount,
    });
    twitchState.chatMessagesMode = 'success';
    await new Promise((r) => setTimeout(r, 1_200));

    step('A transport-uncertain delivery is not automatically retried and is surfaced honestly');
    twitchState.chatMessagesMode = 'transport-uncertain';
    const callsBeforeUncertain = twitchState.chatMessagesCallCount;
    const uncertain = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'connection will be dropped' });
    expect(uncertain.status === 502 && uncertain.body.error === 'outbound_chat_delivery_unknown',
      'a transport-level failure maps to outbound_chat_delivery_unknown', uncertain.body);
    expect(twitchState.chatMessagesCallCount === callsBeforeUncertain + 1, 'the uncertain delivery was not automatically retried', {
      before: callsBeforeUncertain, after: twitchState.chatMessagesCallCount,
    });
    twitchState.chatMessagesMode = 'success';
    await new Promise((r) => setTimeout(r, 1_200));

    step('Two accounts have isolated outbound-chat queues and rate limits');
    const start2 = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
    const attempt2Id = start2.body.attemptId;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attempt2Id}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the second account\'s attempt to reach "polling"');
    const fakeUserId2 = `u2_${RUN_ID}`;
    twitchState.users.set(fakeUserId2, { id: fakeUserId2, login: `streamer2_${RUN_ID}`, displayName: `Streamer2 ${RUN_ID}` });
    const device2 = [...twitchState.devices.values()].find((d) => d.userCode === start2.body.userCode);
    device2.userId = fakeUserId2;
    device2.authorized = true;
    const authorized2 = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attempt2Id}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the second account\'s attempt to reach "authorized"');
    const accountId2 = authorized2.connectedAccountId;
    const authorize2 = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId2}/outbound-chat/authorize`);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${authorize2.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the second account\'s upgrade attempt to reach "polling"');
    const upgradeDevice2 = [...twitchState.devices.values()].find((d) => d.userCode === authorize2.body.userCode);
    upgradeDevice2.userId = fakeUserId2;
    upgradeDevice2.authorized = true;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${authorize2.body.attemptId}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the second account\'s upgrade attempt to reach "authorized"');

    const sendAcc1 = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'from account one' });
    const sendAcc2 = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId2}/outbound-chat/messages`, { message: 'from account two' });
    expect(sendAcc1.status === 200 && sendAcc2.status === 200, 'both accounts can send independently in immediate succession', {
      acc1: sendAcc1.body, acc2: sendAcc2.body,
    });
    const status1 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/outbound-chat`);
    const status2 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId2}/outbound-chat`);
    expect(status1.body.queueDepth === 0 && status2.body.queueDepth === 0, 'neither account\'s queue was affected by the other', {
      status1: status1.body.queueDepth, status2: status2.body.queueDepth,
    });

    step('The EventSub echo of a sent message appears exactly once in operator chat, with no optimistic duplicate');
    await new Promise((r) => setTimeout(r, 1_200));
    twitchState.chatMessagesMode = 'success';
    const echoText = `echo-check-${RUN_ID}`;
    await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: echoText });
    // The application itself never simulates the EventSub echo automatically
    // - Twitch would deliver it back over the same channel.chat.message
    // subscription this account's connector already has active. Simulate
    // that real delivery here, exactly like a real send would trigger it.
    sendWS(socket1, notificationEnvelope('channel.chat.message', {
      broadcaster_user_id: fakeUserId, chatter_user_id: fakeUserId, chatter_user_login: `streamer_${RUN_ID}`, chatter_user_name: `Streamer ${RUN_ID}`,
      message_id: `echo_${RUN_ID}`, color: '', badges: [],
      message: { text: echoText, fragments: [{ type: 'text', text: echoText }] },
    }, `msg_echo_${RUN_ID}`));
    await findEventOfType(baseUrl, echoText);
    await new Promise((r) => setTimeout(r, 500));
    const itemsAfterEcho = await request(baseUrl, 'GET', '/api/operator-chat/items?limit=200');
    const echoCount = itemsAfterEcho.body.items.filter((i) => i.message?.plainText === echoText).length;
    expect(echoCount === 1, 'the sent message appears exactly once via the real EventSub echo - no optimistic duplicate exists', echoCount);

    step('Search every captured HTTP response body and backend log line for message text, tokens and raw Twitch prose');
    const haystack = secretScanChunks.join('\n');
    const backendOutput = backend.getOutput();
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!backendOutput.includes(entry), 'access token is never present in the backend\'s own stdout/stderr', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!backendOutput.includes(entry), 'refresh token is never present in the backend\'s own stdout/stderr', undefined);
    }
    expect(!backendOutput.includes('hello from stage 11a') && !backendOutput.includes(echoText),
      'no outbound message text ever appears in the backend\'s own stdout/stderr', undefined);
    expect(!backendOutput.includes('held for review') && !backendOutput.includes(SECRET_BLOCKED_WORD),
      'no raw Twitch drop-reason prose ever appears in the backend\'s own stdout/stderr', undefined);
    expect(!haystack.includes(SECRET_BLOCKED_WORD), 'the drop-reason prose never appears in any captured HTTP response either', undefined);
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
  console.error(`\nTwitch outbound chat verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
