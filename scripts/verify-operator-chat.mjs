#!/usr/bin/env node
/**
 * Local, no-real-Twitch verification of the Stage 9 unified operator chat:
 * the operator-chat projection, its persisted preferences, the Twitch chat
 * asset resolver, and the HTTP API tying them together.
 *
 * This script never contacts real Twitch. It reuses exactly the same fake-
 * server conventions as scripts/verify-twitch-engagement.mjs (a fake OAuth
 * server, a fake Helix server, and a hand-rolled minimal EventSub WebSocket
 * server), extended with two Helix routes this stage's asset resolver
 * needs: GET /chat/badges/global and GET /chat/badges. The backend under
 * test is the same `-tags integration` testserver binary, pointed at these
 * fakes via the same STREAMING_TREE_TEST_TWITCH_* env vars.
 *
 * This is a representative subset of the stage task's own ~35-step
 * verification list, not the complete enumeration - see docs/progress.md
 * for exactly which scenarios are covered here versus by Go unit tests
 * (internal/operatorchat, internal/domain/operatorchatprefs,
 * internal/provider/twitch/chatassets, internal/httpapi) instead. Not
 * covered here (and named as an intentional omission, not an oversight):
 * a second connected account merging into the same timeline (the existing
 * Stage 8A script already exercises full account-connect plumbing once;
 * repeating a second full device-flow connection here would mostly
 * re-test that same plumbing rather than anything operator-chat-specific),
 * and a deliberately forced projection-side gap (the projection's own gap
 * detection is already exercised directly by Go tests with a tiny,
 * controllable buffer capacity - reproducing that timing reliably against
 * a live child process would be flaky for no additional coverage).
 *
 * Every token, device code, user code and client ID used here is an
 * obviously-fake string generated for this run only. No real Twitch
 * account, application, or network request to Twitch is ever involved.
 *
 * Usage: node scripts/verify-operator-chat.mjs
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
//
// Identical to verify-twitch-engagement.mjs's own fakes, plus two Helix
// routes this stage's chat-asset resolver needs: GET /chat/badges/global
// and GET /chat/badges.

function newTwitchFakeState() {
  return {
    devices: new Map(),
    accessTokens: new Map(),
    refreshTokens: new Map(),
    users: new Map(),
    eventsubSubscriptions: [],
    globalBadgesRequests: 0,
    channelBadgesRequests: 0,
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

      if (req.method === 'GET' && url.pathname === '/chat/badges/global') {
        state.globalBadgesRequests += 1;
        res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({
          data: [{
            set_id: 'vip',
            versions: [{ id: '1', image_url_1x: 'https://static-cdn.jtvnw.net/badges/v1/vip/1', image_url_2x: 'https://static-cdn.jtvnw.net/badges/v1/vip/2', image_url_4x: 'https://static-cdn.jtvnw.net/badges/v1/vip/4' }],
          }],
        }));
        return;
      }

      if (req.method === 'GET' && url.pathname === '/chat/badges') {
        state.channelBadgesRequests += 1;
        res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({
          data: [{
            set_id: 'moderator',
            versions: [{ id: '1', image_url_1x: 'https://static-cdn.jtvnw.net/badges/v1/mod/1', image_url_2x: 'https://static-cdn.jtvnw.net/badges/v1/mod/2', image_url_4x: 'https://static-cdn.jtvnw.net/badges/v1/mod/4' }],
          }],
        }));
        return;
      }

      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { status: 500, message: String(error) });
    }
  });
}

// --- fake EventSub WebSocket server (identical to verify-twitch-engagement.mjs) ---

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

async function findItemMatching(baseUrl, predicate, timeoutMs = 10_000, label = 'a matching operator-chat item') {
  return waitUntil(async () => {
    const items = await request(baseUrl, 'GET', '/api/operator-chat/items?limit=200');
    const match = items.body.items.find(predicate);
    return match ?? false;
  }, timeoutMs, label);
}

async function readOneSSEEvent(url, headers, timeoutMs = 10_000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, { headers, signal: controller.signal });
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      // eslint-disable-next-line no-await-in-loop
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      if (buffer.includes('\n\n')) {
        reader.cancel().catch(() => {});
        return buffer;
      }
    }
    return buffer;
  } finally {
    clearTimeout(timer);
  }
}

async function main() {
  console.log('Unified operator chat (Stage 9) verification (local fakes only, no real Twitch)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-operator-chat-'));
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
  let backendPort;
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
    let oauthPort, helixPort, eventSubPort;
    [backendPort, oauthPort, helixPort, eventSubPort] = await reservePorts(4);
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

    step('Start the backend under test; the operator-chat projection begins empty');
    backend = await startBackend(exePath, env, baseUrl);
    const status0 = await request(baseUrl, 'GET', '/api/operator-chat/status');
    expect(status0.status === 200 && status0.body.retainedCount === 0 && status0.body.bufferCapacity === 500,
      'operator-chat status starts empty with the default 500-item buffer capacity', status0.body);

    step('Confirm default operator-chat preferences before anything has been saved');
    const prefs0 = await request(baseUrl, 'GET', '/api/operator-chat/preferences');
    expect(prefs0.status === 200 && prefs0.body.showPlatformIcon === true && prefs0.body.compactMode === false,
      'GET /api/operator-chat/preferences reports the documented defaults', prefs0.body);

    step('Configure the Twitch Client ID and connect an account (metadata scope only)');
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

    step('Grant the engagement permission upgrade with the same Twitch identity');
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
    pass('engagement permission upgrade completed');

    step('Enable engagement: the connector dials the fake EventSub server');
    const connPromise = nextConnection(wsState);
    await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: true });
    const socket1 = await connPromise;
    sendWS(socket1, welcomeEnvelope('sess_1', 30));
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.state === 'connected' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to reach "connected"');

    step('Send a chat message with a badge and an emote fragment; confirm it becomes one operator-chat message item');
    sendWS(socket1, notificationEnvelope('channel.chat.message', {
      broadcaster_user_id: fakeUserId, chatter_user_id: 'u_chatter_1', chatter_user_login: 'chatter', chatter_user_name: 'Chatter',
      message_id: 'chatmsg_1', color: '#FF0000',
      badges: [{ set_id: 'moderator', id: '1', info: '' }],
      message: { text: 'hello Kappa', fragments: [{ type: 'text', text: 'hello ' }, { type: 'emote', text: 'Kappa', emote: { id: '25' } }] },
    }, 'msg_chat_1'));
    const chatItem = await findItemMatching(baseUrl, (item) => item.kind === 'message' && item.message?.plainText === 'hello Kappa');
    expect(chatItem.message.fragments.length === 2, 'the item preserves both ordered fragments', chatItem.message);
    expect(chatItem.message.fragments[1].emoteImageUrl === 'https://static-cdn.jtvnw.net/emoticons/v2/25/static/dark/2.0',
      'the emote fragment carries the documented CDN URL built from its emote id, with no catalog fetch', chatItem.message.fragments[1]);
    expect(chatItem.user.badges.length === 1 && chatItem.user.badges[0].imageUrl2x === 'https://static-cdn.jtvnw.net/badges/v1/mod/2',
      'the moderator badge resolved to the channel-specific catalog image', chatItem.user.badges);
    expect(twitchState.channelBadgesRequests === 1, 'the channel badge catalog was fetched exactly once (then cached)', twitchState.channelBadgesRequests);

    step('Send a second chat message with a global-only badge; confirm the fallback-to-global resolution path');
    sendWS(socket1, notificationEnvelope('channel.chat.message', {
      broadcaster_user_id: fakeUserId, chatter_user_id: 'u_chatter_2', chatter_user_login: 'chatter2', chatter_user_name: 'Chatter2',
      message_id: 'chatmsg_2', color: '', badges: [{ set_id: 'vip', id: '1', info: '' }],
      message: { text: 'vip here', fragments: [{ type: 'text', text: 'vip here' }] },
    }, 'msg_chat_2'));
    const vipItem = await findItemMatching(baseUrl, (item) => item.kind === 'message' && item.message?.plainText === 'vip here');
    expect(vipItem.user.badges[0].imageUrl2x === 'https://static-cdn.jtvnw.net/badges/v1/vip/2',
      'the vip badge (channel catalog has no vip entry) resolved via the global catalog fallback', vipItem.user.badges);
    expect(twitchState.globalBadgesRequests === 1, 'the global badge catalog was fetched exactly once', twitchState.globalBadgesRequests);

    step('Delete the first message; confirm the SAME item id updates in place with its content preserved');
    sendWS(socket1, notificationEnvelope('channel.chat.message_delete', {
      broadcaster_user_id: fakeUserId, target_user_id: 'u_chatter_1', message_id: 'chatmsg_1',
    }, 'msg_chat_delete_1'));
    const deletedItem = await findItemMatching(baseUrl, (item) => item.id === chatItem.id && item.lifecycle.deleted === true);
    expect(deletedItem.message.plainText === 'hello Kappa', 'the deleted item keeps its original text, never blanked', deletedItem.message);
    expect(deletedItem.lifecycle.deletionReason === 'moderator_deleted', 'the deletion reason is moderator_deleted', deletedItem.lifecycle);

    step('Send a message from a third user, then clear just that user\'s messages (no moderationRef needed)');
    sendWS(socket1, notificationEnvelope('channel.chat.message', {
      broadcaster_user_id: fakeUserId, chatter_user_id: 'u_chatter_3', chatter_user_login: 'chatter3', chatter_user_name: 'Chatter3',
      message_id: 'chatmsg_3', color: '', badges: [],
      message: { text: 'spam spam spam', fragments: [{ type: 'text', text: 'spam spam spam' }] },
    }, 'msg_chat_3'));
    const thirdItem = await findItemMatching(baseUrl, (item) => item.kind === 'message' && item.message?.plainText === 'spam spam spam');
    sendWS(socket1, notificationEnvelope('channel.chat.clear_user_messages', {
      broadcaster_user_id: fakeUserId, target_user_id: 'u_chatter_3',
    }, 'msg_clear_user_1'));
    const clearedThird = await findItemMatching(baseUrl, (item) => item.id === thirdItem.id && item.lifecycle.deleted === true);
    expect(clearedThird.lifecycle.deletionReason === 'user_messages_cleared', 'the per-user clear reason is user_messages_cleared', clearedThird.lifecycle);
    expect(clearedThird.message.plainText === 'spam spam spam', 'the per-user-cleared message keeps its original text', clearedThird.message);
    await findItemMatching(baseUrl, (item) => item.kind === 'moderation' && item.moderation?.action === 'user_messages_cleared' && item.moderation?.targetUserId === 'u_chatter_3');
    pass('a moderation row records the per-user clear, scoped to that one user');

    step('Clear all chat; confirm the second message is marked deleted and a system row is added');
    sendWS(socket1, notificationEnvelope('channel.chat.clear', { broadcaster_user_id: fakeUserId }, 'msg_chat_clear_1'));
    const clearedVip = await findItemMatching(baseUrl, (item) => item.id === vipItem.id && item.lifecycle.deleted === true);
    expect(clearedVip.lifecycle.deletionReason === 'chat_cleared', 'the clear-scoped deletion reason is chat_cleared', clearedVip.lifecycle);
    await findItemMatching(baseUrl, (item) => item.kind === 'system' && item.moderation?.action === 'chat_cleared');
    pass('a system row records the chat clear');

    step('Send follow, subscription-gift-batch, gifted-subscription-recipient, bits, raid, and channel-point-redemption events');
    sendWS(socket1, notificationEnvelope('channel.follow', { user_id: 'u_follower_1', user_login: 'a_follower', user_name: 'A Follower', followed_at: new Date().toISOString() }, 'msg_follow_1'));
    sendWS(socket1, notificationEnvelope('channel.subscription.gift', { user_id: 'u_gifter_1', user_login: 'gifter', user_name: 'Gifter', total: 5, tier: '1000', is_anonymous: false }, 'msg_gift_batch_1'));
    sendWS(socket1, notificationEnvelope('channel.subscribe', { user_id: 'u_recipient_1', user_login: 'recipient', user_name: 'Recipient', tier: '1000', is_gift: true }, 'msg_gift_recipient_1'));
    sendWS(socket1, notificationEnvelope('channel.cheer', { is_anonymous: false, user_id: 'u_cheerer_1', user_login: 'cheerer', user_name: 'Cheerer', message: 'Cheer100', bits: 100 }, 'msg_cheer_1'));
    sendWS(socket1, notificationEnvelope('channel.raid', { from_broadcaster_user_id: 'u_raider_1', from_broadcaster_user_login: 'raider', from_broadcaster_user_name: 'Raider', to_broadcaster_user_id: fakeUserId, viewers: 12 }, 'msg_raid_1'));
    sendWS(socket1, notificationEnvelope('channel.channel_points_custom_reward_redemption.add', { user_id: 'u_redeemer_1', user_login: 'redeemer', user_name: 'Redeemer', reward: { title: 'Highlight' }, user_input: '' }, 'msg_redeem_1'));

    const followItem = await findItemMatching(baseUrl, (i) => i.kind === 'activity' && i.activity?.activityType === 'follow');
    expect(followItem.user.login === 'a_follower', 'the follow item preserves the follower login', followItem.user);
    const giftBatchItem = await findItemMatching(baseUrl, (i) => i.kind === 'activity' && i.activity?.activityType === 'subscription_gift_batch');
    const giftedSubItem = await findItemMatching(baseUrl, (i) => i.kind === 'activity' && i.activity?.activityType === 'gifted_subscription');
    expect(giftBatchItem.id !== giftedSubItem.id, 'the gift batch and the individual recipient gift stay distinct items', { giftBatchItem, giftedSubItem });
    expect(giftBatchItem.activity.quantity === 5, 'the gift batch preserves the total gifted count', giftBatchItem.activity);
    expect(giftedSubItem.user.login === 'recipient', 'the gifted-subscription item is the recipient, not the gifter', giftedSubItem.user);
    const bitsItem = await findItemMatching(baseUrl, (i) => i.kind === 'activity' && i.activity?.activityType === 'bits');
    expect(bitsItem.activity.activityType === 'bits', 'bits is labeled "bits", never "donation"', bitsItem.activity);
    const raidItem = await findItemMatching(baseUrl, (i) => i.kind === 'activity' && i.activity?.activityType === 'raid');
    expect(raidItem.activity.quantity === 12, 'the raid item preserves the raider viewer count as reported, never invented', raidItem.activity);
    await findItemMatching(baseUrl, (i) => i.kind === 'activity' && i.activity?.activityType === 'channel_point_redemption');
    pass('every activity type appeared as its own distinct, correctly-labeled item');

    step('Send a remote stream.online notification; confirm it is never treated as proof of the local FFmpeg output');
    sendWS(socket1, notificationEnvelope('stream.online', { id: 's1', type: 'live', started_at: new Date().toISOString() }, 'msg_online_1'));
    const onlineItem = await findItemMatching(baseUrl, (i) => i.kind === 'activity' && i.activity?.activityType === 'stream.online');
    expect(onlineItem.activity.activityType === 'stream.online', 'the item is labeled exactly "stream.online" - a remote notice, not a local claim', onlineItem);

    step('Save operator-chat preferences and confirm the round trip');
    const savedPrefs = await request(baseUrl, 'PUT', '/api/operator-chat/preferences', {
      showPlatformIcon: false, showPlatformName: true, showAccountLabel: true, showBadges: true,
      showTimestamps: false, showActivityEvents: true, showDeletedMessages: false, hideCommandMessages: true, compactMode: true,
    });
    expect(savedPrefs.status === 200 && savedPrefs.body.compactMode === true, 'preferences saved successfully', savedPrefs.body);
    const readBackPrefs = await request(baseUrl, 'GET', '/api/operator-chat/preferences');
    expect(readBackPrefs.body.showPlatformIcon === false && readBackPrefs.body.hideCommandMessages === true,
      'preferences read back exactly as saved', readBackPrefs.body);

    step('Hide a user, confirm the list, then remove the hidden-user entry');
    const hideResp = await request(baseUrl, 'POST', '/api/operator-chat/hidden-users', {
      providerId: 'twitch', connectedAccountId: accountId, providerUserId: 'u_chatter_2', label: 'test',
    });
    expect(hideResp.status === 200 && typeof hideResp.body.id === 'string', 'the user was added to the hidden list', hideResp.body);
    const hiddenList = await request(baseUrl, 'GET', '/api/operator-chat/hidden-users');
    expect(hiddenList.body.items.length === 1, 'the hidden-users list has exactly one entry', hiddenList.body);
    const removeResp = await fetch(`${baseUrl}/api/operator-chat/hidden-users/${hideResp.body.id}`, { method: 'DELETE' });
    expect(removeResp.status === 204, 'removing the hidden-user entry succeeds', removeResp.status);

    step('Mark a user as a bot and confirm it does not appear in the hidden-users list');
    const botResp = await request(baseUrl, 'POST', '/api/operator-chat/bot-users', {
      providerId: 'twitch', connectedAccountId: accountId, providerUserId: 'u_bot_1',
    });
    expect(botResp.status === 200, 'the user was added to the bot list', botResp.body);
    const hiddenAfterBot = await request(baseUrl, 'GET', '/api/operator-chat/hidden-users');
    expect(hiddenAfterBot.body.items.length === 0, 'marking a user as a bot never also hides them', hiddenAfterBot.body);

    step('Set one account invisible and confirm it is reported by GET /api/operator-chat/account-visibility');
    const visResp = await request(baseUrl, 'PUT', `/api/operator-chat/account-visibility/${accountId}`, { visible: false });
    expect(visResp.status === 200 && visResp.body.visible === false, 'the account visibility PUT succeeds', visResp.body);
    const visList = await request(baseUrl, 'GET', '/api/operator-chat/account-visibility');
    expect(visList.body.items.some((i) => i.accountId === accountId && i.visible === false),
      'the account is reported invisible', visList.body);
    await request(baseUrl, 'PUT', `/api/operator-chat/account-visibility/${accountId}`, { visible: true });

    step('Open the operator-chat SSE stream and confirm it replays a retained item');
    const sseChunk = await readOneSSEEvent(`${baseUrl}/api/operator-chat/stream`, { Accept: 'text/event-stream' });
    expect(sseChunk.includes('event: operator-chat.item'), 'the stream emits operator-chat.item events on connect (replay)', sseChunk.slice(0, 200));
    expect(!sseChunk.includes('accessToken') && !sseChunk.includes('sessionId') && !sseChunk.includes('reconnectUrl'),
      'the SSE payload never leaks a token/session/reconnect-URL-shaped field', undefined);

    step('Confirm no raw EventSub envelope field appears in any operator-chat API payload');
    const itemsSnapshot = await request(baseUrl, 'GET', '/api/operator-chat/items?limit=200');
    const itemsText = JSON.stringify(itemsSnapshot.body);
    for (const rawField of ['metadata', 'message_id', 'subscription_type', 'chatter_user_id']) {
      expect(!itemsText.includes(rawField), `operator-chat items never contain the raw EventSub field "${rawField}"`, undefined);
    }

    step('Restart the backend: chat content resets, but preferences/bot-user marking persist');
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);

    const statusAfterRestart = await request(baseUrl, 'GET', '/api/operator-chat/status');
    expect(statusAfterRestart.body.retainedCount === 0, 'the operator-chat projection is empty again after a restart (transient, in-memory only)', statusAfterRestart.body);

    const prefsAfterRestart = await request(baseUrl, 'GET', '/api/operator-chat/preferences');
    expect(prefsAfterRestart.body.compactMode === true && prefsAfterRestart.body.hideCommandMessages === true,
      'saved preferences survive the restart', prefsAfterRestart.body);

    const botsAfterRestart = await request(baseUrl, 'GET', '/api/operator-chat/bot-users');
    expect(botsAfterRestart.body.items.some((i) => i.providerUserId === 'u_bot_1'), 'the bot-user marking survives the restart', botsAfterRestart.body);

    step('Search every captured HTTP response body and backend log line for real secret material');
    const haystack = secretScanChunks.join('\n');
    const backendOutput = backend.getOutput();
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!haystack.includes(entry), 'access token is never present in captured HTTP responses', undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!haystack.includes(entry), 'refresh token is never present in captured HTTP responses', undefined);
    }
    expect(!backendOutput.includes('sess_1'), 'no EventSub WebSocket session id ever appears in the backend\'s own stdout/stderr', undefined);
    expect(!backendOutput.includes('hello Kappa') && !backendOutput.includes('vip here'),
      'no chat message text ever appears in the backend\'s own stdout/stderr', undefined);
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
  console.error(`\nUnified operator chat verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
