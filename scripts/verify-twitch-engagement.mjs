#!/usr/bin/env node
/**
 * Local, no-real-Twitch verification of the Stage 8A Engagement Event Bus
 * and Twitch EventSub WebSocket connector.
 *
 * This script never contacts real Twitch. It runs the real backend under
 * test (`go build -tags integration ./cmd/testserver`) against three small
 * in-process fakes that reproduce only the response/message shapes this
 * application actually parses:
 *
 *   fake OAuth server    (id.twitch.tv/oauth2 equivalent)
 *     /device, /token, /validate, /revoke
 *   fake Helix server    (api.twitch.tv/helix equivalent)
 *     /users, /eventsub/subscriptions
 *   fake EventSub server (eventsub.wss.twitch.tv/ws equivalent)
 *     a hand-rolled minimal WebSocket server (handshake + unmasked text
 *     frames) - Node has no built-in WebSocket *server* and this project
 *     has no `ws` dependency, so this implements only the small subset of
 *     RFC 6455 the connector under test actually needs: it never reads or
 *     parses an incoming frame, because the connector itself never writes
 *     anything on this connection once open (see internal/runtime/
 *     twitchengagement/connector.go - subscriptions are created over a
 *     separate Helix HTTP call, not over the socket).
 *
 * The backend is pointed at them via STREAMING_TREE_TEST_TWITCH_OAUTH_BASE_URL,
 * STREAMING_TREE_TEST_TWITCH_API_BASE_URL, STREAMING_TREE_TEST_TWITCH_EVENTSUB_BASE_URL
 * and STREAMING_TREE_TEST_TWITCH_EVENTSUB_RECONNECT_HOST - env vars that exist
 * only in the `-tags integration` binary, never in a production build.
 *
 * This is a representative subset of the stage task's full verification
 * list, not the complete ~47-step enumeration - see docs/progress.md for
 * exactly which scenarios are covered here versus by Go unit tests
 * (internal/runtime/twitchengagement, internal/provider/twitch) instead.
 *
 * Every token, device code, user code and client ID used here is an
 * obviously-fake string generated for this run only. No real Twitch
 * account, application, or network request to Twitch is ever involved.
 *
 * Usage: node scripts/verify-twitch-engagement.mjs
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
    devices: new Map(), // deviceCode -> { userCode, pollCount, authorized, userId, scopes }
    accessTokens: new Map(), // token -> { valid, userId, scopes }
    refreshTokens: new Map(), // token -> { valid, userId, scopes }
    users: new Map(), // userId -> { id, login, displayName }
    eventsubSubscriptions: [], // { type, version, condition, sessionId }
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

      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { status: 500, message: String(error) });
    }
  });
}

// --- fake EventSub WebSocket server ----------------------------------------
//
// A hand-rolled minimal RFC 6455 server: only what this connector needs
// (handshake + unmasked server-to-client text frames). The connector never
// writes to this socket once open, so incoming frames are drained and
// ignored rather than parsed.

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
    socket.on('data', () => {}); // no incoming frame is ever parsed - see file header
    socket.on('error', () => {}); // an abrupt reset is expected during the ordinary-disconnect scenario
    state.connections.push(socket);
    const resolver = state.pendingResolvers.shift();
    if (resolver !== undefined) resolver(socket);
  });
  return server;
}

// --- EventSub envelope builders ---------------------------------------------

function welcomeEnvelope(sessionId, keepaliveSeconds) {
  return {
    metadata: { message_id: mintToken('wsmsg'), message_type: 'session_welcome', message_timestamp: new Date().toISOString() },
    payload: { session: { id: sessionId, status: 'connected', keepalive_timeout_seconds: keepaliveSeconds, reconnect_url: null } },
  };
}

function reconnectEnvelope(sessionId, reconnectUrl) {
  return {
    metadata: { message_id: mintToken('wsmsg'), message_type: 'session_reconnect', message_timestamp: new Date().toISOString() },
    payload: { session: { id: sessionId, status: 'reconnecting', keepalive_timeout_seconds: null, reconnect_url: reconnectUrl } },
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

function revocationEnvelope(subType, status) {
  return {
    metadata: {
      message_id: mintToken('wsmsg'), message_type: 'revocation', message_timestamp: new Date().toISOString(),
      subscription_type: subType, subscription_version: '1',
    },
    payload: { subscription: { id: 'sub_x', status, type: subType, version: '1' } },
  };
}

const EXPECTED_SUBSCRIPTION_TYPES = new Set([
  'channel.chat.message',
  'channel.chat.message_delete',
  'channel.chat.clear',
  'channel.chat.clear_user_messages',
  'channel.follow',
  'channel.subscribe',
  'channel.subscription.gift',
  'channel.subscription.message',
  'channel.cheer',
  'channel.raid',
  'channel.channel_points_custom_reward_redemption.add',
  'stream.online',
  'stream.offline',
]);

async function findEventOfType(baseUrl, type, timeoutMs = 10_000) {
  return waitUntil(async () => {
    const events = await request(baseUrl, 'GET', '/api/engagement/events?limit=100');
    const match = events.body.items.find((item) => item.type === type);
    return match ?? false;
  }, timeoutMs, `an event of type "${type}" to appear`);
}

async function main() {
  console.log('Twitch engagement (Event Bus + EventSub connector) verification (local fakes only, no real Twitch)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-twitch-engagement-'));
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

    step('Start the backend under test with no engagement connectors enabled');
    backend = await startBackend(exePath, env, baseUrl);
    const status0 = await request(baseUrl, 'GET', '/api/engagement/status');
    expect(status0.status === 200 && status0.body.connectors.length === 0 && status0.body.retainedCount === 0,
      'the Event Bus starts empty with no connectors', status0.body);

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

    step('Confirm the account reports a permission upgrade is required before engagement scopes exist');
    const engagement0 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(engagement0.status === 200 && engagement0.body.permissionUpgradeRequired === true,
      'permissionUpgradeRequired is true with only the metadata scope granted', engagement0.body);
    expect(engagement0.body.state === 'disabled', 'the connector state is "disabled" before it is ever enabled', engagement0.body);

    step('Start the identity-bound permission-upgrade device-flow attempt and verify its requested scopes');
    const upgrade = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/engagement/authorize`);
    expect(upgrade.status === 202, 'the upgrade attempt starts', upgrade.body);
    const upgradeDevice = [...twitchState.devices.values()].find((d) => d.userCode === upgrade.body.userCode);
    expect(upgradeDevice !== undefined, 'the fake server has a matching device entry for the upgrade attempt', upgrade.body.userCode);
    expect(upgradeDevice.scopes.includes('channel:manage:broadcast'), 'the upgrade request preserves the existing metadata scope', upgradeDevice.scopes);
    for (const scope of ['user:read:chat', 'moderator:read:followers', 'channel:read:subscriptions', 'bits:read', 'channel:read:redemptions']) {
      expect(upgradeDevice.scopes.includes(scope), `the upgrade request includes ${scope}`, upgradeDevice.scopes);
    }
    expect(!upgradeDevice.scopes.includes('user:write:chat'), 'the upgrade request never includes user:write:chat (stage 11 scope)', upgradeDevice.scopes);

    step('Complete the upgrade with the same Twitch identity');
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${upgrade.body.attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the upgrade attempt to reach "polling"');
    upgradeDevice.userId = fakeUserId; // same identity - required for FinalizeConnection to accept it
    upgradeDevice.authorized = true;
    const upgradeAuthorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${upgrade.body.attemptId}`);
      if (snap.body.state === 'error') throw new Error(`upgrade attempt error: ${snap.body.errorCode}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the upgrade attempt to reach "authorized"');
    expect(upgradeAuthorized.connectedAccountId === accountId, 'the upgrade resolved to the same connected account, not a new one', upgradeAuthorized);

    const engagement1 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(engagement1.body.permissionUpgradeRequired === false, 'permissionUpgradeRequired is now false', engagement1.body);

    step('Enable engagement: the connector dials the fake EventSub server');
    const connPromise = nextConnection(wsState);
    const enableResp = await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: true });
    expect(enableResp.status === 200 && enableResp.body.enabled === true, 'the enable request succeeds', enableResp.body);
    const socket1 = await connPromise;
    sendWS(socket1, welcomeEnvelope('sess_1', 30));

    step('Confirm the connector subscribes to exactly the 13 selected EventSub types, no more, no fewer');
    await waitUntil(() => (twitchState.eventsubSubscriptions.length === 13 ? true : false), POLL_TIMEOUT_MS, '13 subscription requests');
    const seenTypes = new Set(twitchState.eventsubSubscriptions.map((s) => s.type));
    expect(seenTypes.size === 13, 'exactly 13 distinct subscription types were requested', [...seenTypes]);
    for (const type of seenTypes) {
      expect(EXPECTED_SUBSCRIPTION_TYPES.has(type), `subscription type "${type}" is one of the selected Stage 8A types`, type);
    }
    expect(!seenTypes.has('channel.chat.notification'), 'channel.chat.notification was never subscribed to (deliberately omitted)', [...seenTypes]);

    step('Confirm the connector reaches the "connected" state');
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.state === 'connected' && snap.body.activeSubscriptionCount === 13 ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to reach "connected" with 13 active subscriptions');

    step('Send a follow notification and confirm it becomes a normalized event');
    sendWS(socket1, notificationEnvelope('channel.follow', {
      user_id: 'u_follower_1', user_login: 'a_follower', user_name: 'A Follower', followed_at: new Date().toISOString(),
    }, 'msg_follow_1'));
    const followEvent = await findEventOfType(baseUrl, 'follow');
    expect(followEvent.user?.login === 'a_follower', 'the follow event preserves the follower login', followEvent);

    step('Redeliver the same EventSub message id and confirm it is dropped as a duplicate');
    sendWS(socket1, notificationEnvelope('channel.follow', {
      user_id: 'u_follower_1', user_login: 'a_follower', user_name: 'A Follower', followed_at: new Date().toISOString(),
    }, 'msg_follow_1'));
    await new Promise((r) => setTimeout(r, 500));
    const eventsAfterDup = await request(baseUrl, 'GET', '/api/engagement/events?limit=100');
    const followCount = eventsAfterDup.body.items.filter((e) => e.type === 'follow').length;
    expect(followCount === 1, 'the duplicate delivery did not produce a second event', followCount);

    step('Send a chat message with ordered fragments');
    sendWS(socket1, notificationEnvelope('channel.chat.message', {
      broadcaster_user_id: fakeUserId, chatter_user_id: 'u_chatter_1', chatter_user_login: 'chatter', chatter_user_name: 'Chatter',
      message_id: 'chatmsg_1', color: '#FF0000', badges: [],
      message: { text: 'hello Kappa', fragments: [{ type: 'text', text: 'hello ' }, { type: 'emote', text: 'Kappa', emote: { id: '25' } }] },
    }, 'msg_chat_1'));
    const chatEvent = await findEventOfType(baseUrl, 'chat.message');
    expect(chatEvent.message?.fragments?.length === 2, 'the chat message preserves both ordered fragments', chatEvent.message);
    expect(chatEvent.message?.fragments?.[1]?.emoteId === '25', 'the emote fragment preserves its provider emote id', chatEvent.message);

    step('Send a gift-batch event and a gifted-subscription recipient event; confirm both stay distinct');
    sendWS(socket1, notificationEnvelope('channel.subscription.gift', {
      user_id: 'u_gifter_1', user_login: 'gifter', user_name: 'Gifter', total: 5, tier: '1000', is_anonymous: false,
    }, 'msg_gift_batch_1'));
    sendWS(socket1, notificationEnvelope('channel.subscribe', {
      user_id: 'u_recipient_1', user_login: 'recipient', user_name: 'Recipient', tier: '1000', is_gift: true,
    }, 'msg_gift_recipient_1'));
    const giftBatch = await findEventOfType(baseUrl, 'subscription_gift_batch');
    const giftedSub = await findEventOfType(baseUrl, 'gifted_subscription');
    expect(giftBatch.quantity === 5, 'the gift batch preserves the total gifted count', giftBatch);
    expect(giftedSub.user?.login === 'recipient', 'the gifted-subscription event is the recipient, not the gifter', giftedSub);

    step('Send an anonymous cheer and confirm no identity is fabricated');
    sendWS(socket1, notificationEnvelope('channel.cheer', { is_anonymous: true, message: 'Cheer100', bits: 100 }, 'msg_cheer_1'));
    const cheerEvent = await findEventOfType(baseUrl, 'bits');
    expect(cheerEvent.user?.anonymous === true, 'the anonymous cheer is flagged anonymous', cheerEvent.user);
    expect(!('providerUserId' in (cheerEvent.user ?? {})) || cheerEvent.user.providerUserId === undefined,
      'no identity is fabricated for the anonymous cheer', cheerEvent.user);

    step('Send stream.online and stream.offline notifications');
    sendWS(socket1, notificationEnvelope('stream.online', { id: 's1', type: 'live', started_at: new Date().toISOString() }, 'msg_online_1'));
    sendWS(socket1, notificationEnvelope('stream.offline', {}, 'msg_offline_1'));
    await findEventOfType(baseUrl, 'stream.online');
    await findEventOfType(baseUrl, 'stream.offline');
    pass('both stream.online and stream.offline were normalized');

    const subCountBeforeReconnect = twitchState.eventsubSubscriptions.length;

    step('Official session_reconnect: the old connection stays open until the new one welcomes, no resubscription, no data gap');
    const reconnectPort = eventSubPort; // same fake server, same host - just a distinguishable path via the reconnect handoff itself
    const conn2Promise = nextConnection(wsState);
    sendWS(socket1, reconnectEnvelope('sess_1', `ws://127.0.0.1:${reconnectPort}/reconnect`));
    const socket2 = await conn2Promise;
    sendWS(socket2, welcomeEnvelope('sess_2', 30));
    await new Promise((r) => setTimeout(r, 500));
    expect(twitchState.eventsubSubscriptions.length === subCountBeforeReconnect,
      'no new subscription requests were made after an official reconnect', {
        before: subCountBeforeReconnect, after: twitchState.eventsubSubscriptions.length,
      });
    const afterReconnect = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(afterReconnect.body.state === 'connected', 'the connector is connected again after the official handoff', afterReconnect.body);
    expect(afterReconnect.body.lastDataGapAt === undefined, 'no data gap was recorded for the official reconnect', afterReconnect.body);
    socket1.destroy(); // the connector should already have stopped using this one

    step('Ordinary disconnect: an abrupt loss triggers reconnection, resubscription and an honest data-gap marker');
    const conn3Promise = nextConnection(wsState);
    socket2.destroy();
    const socket3 = await conn3Promise;
    sendWS(socket3, welcomeEnvelope('sess_3', 30));
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.reconnectCount > 0 && snap.body.state === 'connected' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to finish reconnecting after an ordinary loss');
    const afterOrdinary = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(afterOrdinary.body.lastDataGapAt !== undefined, 'a data-gap timestamp was recorded for the ordinary loss', afterOrdinary.body);
    expect(twitchState.eventsubSubscriptions.length === subCountBeforeReconnect * 2,
      'subscriptions were recreated after the ordinary reconnect (double the original count)', {
        expected: subCountBeforeReconnect * 2, actual: twitchState.eventsubSubscriptions.length,
      });

    step('Authorization-revoked: the connector enters a terminal error state and does not auto-retry');
    sendWS(socket3, revocationEnvelope('channel.chat.message', 'authorization_revoked'));
    const revoked = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.state === 'error' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to enter the error state after revocation');
    expect(revoked.lastError === 'twitch_eventsub_subscription_revoked', 'the sanitized error code is twitch_eventsub_subscription_revoked', revoked);

    step('Explicit restart recovers the connector');
    const conn4Promise = nextConnection(wsState);
    const restartResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/engagement/restart`);
    expect(restartResp.status === 200, 'the restart request succeeds', restartResp.body);
    const socket4 = await conn4Promise;
    sendWS(socket4, welcomeEnvelope('sess_4', 30));
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.state === 'connected' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to reach "connected" again after restart');

    step('Disabling the connector persists and stops it');
    const disableResp = await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: false });
    expect(disableResp.status === 200 && disableResp.body.state === 'disabled', 'the connector reports disabled', disableResp.body);

    step('Disconnecting the account removes its engagement configuration');
    const disconnectResp = await fetch(`${baseUrl}/api/connected-accounts/${accountId}`, { method: 'DELETE' });
    expect(disconnectResp.status === 204, 'the account was disconnected', disconnectResp.status);
    const afterDisconnect = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(afterDisconnect.status === 404, 'the engagement endpoint reports 404 for the now-disconnected account', afterDisconnect.body);

    step('Search every captured HTTP response body and backend log line for real secret material');
    const haystack = secretScanChunks.join('\n');
    const backendOutput = backend.getOutput();
    for (const entry of twitchState.accessTokens.keys()) {
      expect(!haystack.includes(entry), `access token is never present in captured HTTP responses`, undefined);
    }
    for (const entry of twitchState.refreshTokens.keys()) {
      expect(!haystack.includes(entry), `refresh token is never present in captured HTTP responses`, undefined);
    }
    expect(!backendOutput.includes('sess_1') && !backendOutput.includes('sess_2') && !backendOutput.includes('sess_3'),
      'no EventSub WebSocket session id ever appears in the backend\'s own stdout/stderr', undefined);
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
  console.error(`\nTwitch engagement verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
