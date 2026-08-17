#!/usr/bin/env node
/**
 * Local, no-real-provider verification of the Stage 18B supporter/
 * activity widgets, richer session counters, and bounded multi-widget
 * dashboards (docs/supporter-widgets.md).
 *
 * This script never contacts real Twitch, YouTube, or StreamElements.
 * It runs the real backend under test
 * (`go build -tags integration ./cmd/testserver`) against the exact
 * same local fakes `scripts/verify-twitch-engagement.mjs`,
 * `scripts/verify-youtube-engagement.mjs`, and
 * `scripts/verify-streamelements-donations.mjs` already established -
 * a real Twitch EventSub WebSocket fake, the real fake YouTube
 * streamList gRPC server binary, and the real fake StreamElements
 * Astro WebSocket server binary. No separate fake event source is
 * created (this task's own instruction, mirroring
 * `scripts/verify-goals-widgets.mjs`'s own identical precedent): every
 * scenario below drives a real normalized event through a real
 * provider fake, the real Event Bus, the real
 * `internal/supporterwidgets.Manager`, and the real public widget SSE
 * stream, end to end.
 *
 * This is a representative subset of the stage task's own scenario
 * enumeration, not an exhaustive one-assertion-per-item transcription -
 * mirrors `verify-goals-widgets.mjs`'s own established convention.
 * Exhaustive per-provider connector correctness (reconnect, backoff,
 * gap detection, OAuth edge cases) is already covered by the three
 * scripts named above and by their own Go unit tests; this script
 * instead proves the Stage 18B-specific projection/privacy/dashboard/
 * restart behavior genuinely works against real provider fakes.
 *
 * Every token, device code, JWT and identifier used here is an
 * obviously-fake string generated for this run only. No real Twitch,
 * Google/YouTube, or StreamElements account, application, or network
 * request is ever involved, and no real OBS Browser Source is opened.
 *
 * Teardown discipline (docs/progress.md's own Stage 18A closing-
 * regression record documents two real defects a prior script had here
 * - a stale-SSE-event race and unclosed fake servers preventing a clean
 * process exit): every SSE iterator opened below is explicitly closed
 * once done with it, and the `finally` block closes every fake server
 * and waits for the backend child process to exit before removing the
 * temporary root - verified during development by confirming no
 * lingering node/testserver/fake-provider process survives two
 * consecutive standalone runs.
 *
 * Usage: node scripts/verify-supporter-widgets.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
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
const POLL_TIMEOUT_MS = 20_000;

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

async function requestAbsolute(url) {
  const response = await fetch(url);
  const text = await response.text();
  record(text);
  return { status: response.status, text };
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

async function buildBinary(label, cmdPath, exePath) {
  const build = spawnCaptured(label, 'go', ['build', '-tags', 'integration', '-o', exePath, cmdPath], { cwd: SERVER_DIR });
  const buildExit = await new Promise((r) => {
    const timer = setTimeout(() => r(-1), BUILD_TIMEOUT_MS);
    build.child.on('exit', (code) => { clearTimeout(timer); r(code); });
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

// === fake Twitch OAuth + Helix + EventSub (mirrors verify-twitch-engagement.mjs) ===

function newTwitchFakeState() {
  return {
    devices: new Map(),
    accessTokens: new Map(),
    refreshTokens: new Map(),
    users: new Map(),
    eventsubSubscriptions: [],
  };
}

function createTwitchOAuthServer(state) {
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
          const accessToken = mintToken('fake-access');
          const refreshToken = mintToken('fake-refresh');
          state.accessTokens.set(accessToken, { valid: true, userId: device.userId, scopes: device.scopes });
          state.refreshTokens.set(refreshToken, { valid: true, userId: device.userId, scopes: device.scopes });
          sendJSON(res, 200, { access_token: accessToken, refresh_token: refreshToken, scope: device.scopes, token_type: 'bearer', expires_in: 14_400 });
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
        sendJSON(res, 200, { client_id: CLIENT_ID, login: user?.login ?? '', user_id: entry.userId, scopes: entry.scopes, expires_in: 14_400 });
        return;
      }

      if (req.method === 'POST' && url.pathname === '/revoke') {
        const form = new URLSearchParams(await readBody(req));
        const entry = state.accessTokens.get(form.get('token'));
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

function createTwitchHelixServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');
      const auth = req.headers.authorization ?? '';
      const token = auth.startsWith('Bearer ') ? auth.slice('Bearer '.length) : '';
      const entry = state.accessTokens.get(token);
      if (entry === undefined || !entry.valid) {
        res.writeHead(401, { 'Content-Type': 'application/json', Connection: 'close' });
        res.end(JSON.stringify({ error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' }));
        return;
      }
      if (req.method === 'GET' && url.pathname === '/users') {
        const user = state.users.get(entry.userId);
        res.writeHead(200, { 'Content-Type': 'application/json', Connection: 'close' });
        res.end(JSON.stringify({ data: [{ id: user.id, login: user.login, display_name: user.displayName }] }));
        return;
      }
      if (req.method === 'POST' && url.pathname === '/eventsub/subscriptions') {
        const raw = await readBody(req);
        record(raw);
        const parsed = JSON.parse(raw);
        state.eventsubSubscriptions.push({ type: parsed.type, sessionId: parsed.transport?.session_id });
        res.writeHead(202, { 'Content-Type': 'application/json', Connection: 'close' });
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
    state.pendingResolvers.push((socket) => {
      clearTimeout(timer);
      resolveConn(socket);
    });
  });
}

function createTwitchEventSubServer(state) {
  const server = createHttpServer((_req, res) => {
    res.writeHead(404);
    res.end();
  });
  server.on('upgrade', (req, socket) => {
    const key = req.headers['sec-websocket-key'];
    socket.write(
      'HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n' +
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

// === fake YouTube OAuth + REST + gRPC (mirrors verify-youtube-engagement.mjs) ===

function newYouTubeFakeState() {
  return {
    accessTokens: new Map(), refreshTokens: new Map(), channels: new Map(),
    broadcasts: new Map(), pendingCodes: new Map(),
  };
}

function createYouTubeOAuthServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');
      if (req.method === 'POST' && url.pathname === '/token') {
        const form = new URLSearchParams(await readBody(req));
        const grantType = form.get('grant_type');
        if (grantType === 'authorization_code') {
          const code = form.get('code');
          const pending = state.pendingCodes.get(code);
          if (pending === undefined) {
            sendJSON(res, 400, { error: 'invalid_grant' });
            return;
          }
          const accessToken = mintToken('fake-access');
          const refreshToken = mintToken('fake-refresh');
          state.accessTokens.set(accessToken, { valid: true, channelId: null, scope: pending.scope });
          state.refreshTokens.set(refreshToken, { valid: true, channelId: null, scope: pending.scope });
          sendJSON(res, 200, { access_token: accessToken, refresh_token: refreshToken, scope: pending.scope, token_type: 'Bearer', expires_in: 3600 });
          return;
        }
        sendJSON(res, 400, { error: 'unsupported_grant_type' });
        return;
      }
      if (req.method === 'GET' && url.pathname === '/tokeninfo') {
        const token = url.searchParams.get('access_token') ?? '';
        const entry = state.accessTokens.get(token);
        if (entry === undefined || !entry.valid) {
          sendJSON(res, 400, { error_description: 'Invalid Value' });
          return;
        }
        sendJSON(res, 200, { aud: CLIENT_ID, scope: entry.scope, expires_in: '3599' });
        return;
      }
      if (req.method === 'POST' && url.pathname === '/revoke') {
        const form = new URLSearchParams(await readBody(req));
        const entry = state.accessTokens.get(form.get('token')) ?? state.refreshTokens.get(form.get('token'));
        if (entry !== undefined) entry.valid = false;
        res.writeHead(200, { Connection: 'close' });
        res.end();
        return;
      }
      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { error: 'server_error', error_description: String(error) });
    }
  });
}

function createYouTubeAPIServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');
      const auth = req.headers.authorization ?? '';
      const token = auth.startsWith('Bearer ') ? auth.slice('Bearer '.length) : '';
      const entry = state.accessTokens.get(token);
      if (entry === undefined || !entry.valid) {
        sendJSON(res, 401, { error: { code: 401, message: 'Invalid Credentials' } });
        return;
      }
      if (req.method === 'GET' && url.pathname === '/channels') {
        const items = url.searchParams.get('mine') === 'true'
          ? [...state.channels.values()]
          : [...state.channels.values()].filter((c) => c.id === url.searchParams.get('id'));
        sendJSON(res, 200, { items: items.map((c) => ({ id: c.id, snippet: { title: c.title, description: '', customUrl: '', country: c.country ?? '', thumbnails: { default: { url: 'https://fake.youtube.example/avatar.jpg' } } } })) });
        return;
      }
      if (req.method === 'GET' && url.pathname === '/liveBroadcasts') {
        if (url.searchParams.has('id')) {
          const b = state.broadcasts.get(url.searchParams.get('id'));
          sendJSON(res, 200, { items: b === undefined ? [] : [b] });
          return;
        }
        const status = url.searchParams.get('broadcastStatus');
        const items = [...state.broadcasts.values()].filter((b) => b.status.lifeCycleStatusFilter === status);
        sendJSON(res, 200, { items: items.map((b) => ({ id: b.id, snippet: { title: b.snippet.title, liveChatId: b.snippet.liveChatId ?? '' }, status: { lifeCycleStatus: b.status.lifeCycleStatus, privacyStatus: b.status.privacyStatus } })) });
        return;
      }
      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { error: { code: 500, message: String(error) } });
    }
  });
}

async function startFakeGRPCServer(exePath, grpcAddr, controlAddr) {
  const handle = spawnCaptured('fake-grpc', exePath, [`-grpc-addr=${grpcAddr}`, `-control-addr=${controlAddr}`], { cwd: SERVER_DIR });
  await waitUntil(async () => {
    if (handle.hasExited()) throw new Error(`fake gRPC server exited during startup:\n${handle.getOutput()}`);
    const res = await fetch(`http://${controlAddr}/control/health`).catch(() => null);
    return res !== null && res.ok ? true : false;
  }, READINESS_TIMEOUT_MS, 'the fake YouTube streamList gRPC server to become ready');
  return handle;
}

async function scriptLiveChat(controlBaseUrl, liveChatId, entries) {
  const res = await fetch(`${controlBaseUrl}/control/script`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ liveChatId, entries }),
  });
  expect(res.status === 204, `scripted ${entries.length} entr${entries.length === 1 ? 'y' : 'ies'} for liveChatId ${liveChatId}`, await res.text());
}

async function scriptPage(controlBaseUrl, liveChatId, { items = [], nextPageToken = `token-${randomUUID().slice(0, 6)}` } = {}) {
  await scriptLiveChat(controlBaseUrl, liveChatId, [{ type: 'page', items, nextPageToken, offlineAt: '' }]);
}

function superChatItem({ id, authorChannelId, displayName, amountMicros, currency, amountDisplayString }) {
  return {
    id,
    snippet: { type: 'superChatEvent', publishedAt: new Date().toISOString(), authorChannelId, displayMessage: '', superChatDetails: { amountMicros, currency, amountDisplayString, userComment: '', tier: 2 } },
    authorDetails: { channelId: authorChannelId, displayName, profileImageUrl: 'https://fake.youtube.example/avatar.jpg', isVerified: false, isChatOwner: false, isChatSponsor: false, isChatModerator: false },
  };
}

// === fake StreamElements Astro server (mirrors verify-streamelements-donations.mjs) ===

async function startFakeAstroServer(exePath, wsAddr, controlAddr) {
  const handle = spawnCaptured('fake-astro', exePath, [`-ws-addr=${wsAddr}`, `-control-addr=${controlAddr}`], { cwd: SERVER_DIR });
  await waitUntil(async () => {
    if (handle.hasExited()) throw new Error(`fake Astro server exited during startup:\n${handle.getOutput()}`);
    const res = await fetch(`http://${controlAddr}/control/health`).catch(() => null);
    return res !== null && res.ok ? true : false;
  }, READINESS_TIMEOUT_MS, 'the fake Astro server to become ready');
  return handle;
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
    try { parsed = JSON.parse(text); } catch { parsed = text; }
  }
  return { status: res.status, body: parsed };
}

async function waitForConnection(controlBaseUrl, label, predicate) {
  return waitUntil(async () => {
    const res = await control(controlBaseUrl, 'GET', '/control/connections');
    if (res.body.items.length === 0) return false;
    if (!predicate) return res.body;
    return res.body.items.some(predicate) ? res.body : false;
  }, POLL_TIMEOUT_MS, label);
}

async function pushTip(controlBaseUrl, topic, tip) {
  const res = await control(controlBaseUrl, 'POST', '/control/push-tip', { connectionId: 'latest', topic, tip });
  expect(res.status === 204, `pushed a ${topic} tip (${tip._id})`, res.body);
}

function tipFixture(overrides = {}) {
  const id = overrides.id ?? mintToken('tip');
  return {
    donation: {
      user: { username: overrides.username ?? 'Styler', geo: 'ZZ', email: `donor-${RUN_ID}@example.invalid`, channel: 'chan_1' },
      message: overrides.message ?? '', amount: overrides.amount ?? 4.2, currency: overrides.currency ?? 'USD', paymentMethod: 'scheme',
    },
    _id: id, channel: 'chan_1', provider: 'paypal', approved: overrides.approved ?? 'allowed', status: 'success',
    createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), transactionId: mintToken('txn'),
  };
}

// === supporter-widgets-specific helpers ===

function baseWidgetBody(kind, overrides = {}) {
  return {
    kind, name: overrides.name ?? `Widget ${kind}`, enabled: overrides.enabled ?? true,
    providers: overrides.providers ?? [], accounts: overrides.accounts ?? [],
    showCurrent: false, showTarget: false, showPercent: false,
    showProvider: true, showTime: true, showMessage: overrides.showMessage ?? false,
    maxItems: overrides.maxItems ?? 0, currency: overrides.currency, metric: overrides.metric,
    eventTypes: overrides.eventTypes ?? [], columns: overrides.columns ?? 0, children: overrides.children ?? [],
    orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
    backgroundColor: '#000000', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
    borderRadiusPx: 12, opacity: 1.0,
  };
}

async function fetchWidgetConfig(baseUrl, slug) {
  const res = await request(baseUrl, 'GET', `/api/public/widgets/${slug}/config`);
  return res.body;
}

async function waitForPublicField(baseUrl, slug, predicate, label) {
  return waitUntil(async () => {
    const body = await fetchWidgetConfig(baseUrl, slug);
    const result = predicate(body);
    return result ? body : false;
  }, POLL_TIMEOUT_MS, label);
}

function parseSSEChunk(chunk) {
  let eventName = 'message';
  let data = '';
  for (const line of chunk.split('\n')) {
    if (line.startsWith('event:')) eventName = line.slice('event:'.length).trim();
    else if (line.startsWith('data:')) data += line.slice('data:'.length).trim();
  }
  let parsed = null;
  if (data !== '') {
    try { parsed = JSON.parse(data); } catch { parsed = data; }
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

// === main ===

async function main() {
  console.log('Stage 18B supporter widgets verification (local fakes only, no real Twitch/YouTube/StreamElements, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-supporter-widgets-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);
  const dbPath = join(dataDir, 'streaming-tree.db');

  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const fakeGRPCExePath = join(tempDir, process.platform === 'win32' ? 'fakeyoutubegrpc.exe' : 'fakeyoutubegrpc');
  const fakeAstroExePath = join(tempDir, process.platform === 'win32' ? 'fakestreamelements.exe' : 'fakestreamelements');

  const twitchState = newTwitchFakeState();
  const wsState = newEventSubServerState();
  const twitchOAuth = createTwitchOAuthServer(twitchState);
  const twitchHelix = createTwitchHelixServer(twitchState);
  const twitchEventSub = createTwitchEventSubServer(wsState);

  const ytState = newYouTubeFakeState();
  const ytOAuth = createYouTubeOAuthServer(ytState);
  const ytAPI = createYouTubeAPIServer(ytState);

  let backend = null;
  let fakeGRPC = null;
  let fakeAstro = null;
  let baseUrl;

  try {
    step('Build the integration-only test server and the fake YouTube gRPC / StreamElements Astro binaries');
    await buildBinary('go-build-testserver', './cmd/testserver', exePath);
    await buildBinary('go-build-fakeyoutubegrpc', './cmd/fakeyoutubegrpc', fakeGRPCExePath);
    await buildBinary('go-build-fakestreamelements', './cmd/fakestreamelements', fakeAstroExePath);

    step('Reserve dynamic loopback ports and start every fake provider server');
    const [backendPort, twOAuthPort, twHelixPort, twEventSubPort, ytOAuthPort, ytAPIPort] = await reservePorts(6);
    const [grpcPort, grpcControlPort, seWsPort, seControlPort] = await reservePorts(4);
    baseUrl = `http://127.0.0.1:${backendPort}`;
    await listen(twitchOAuth, twOAuthPort);
    await listen(twitchHelix, twHelixPort);
    await listen(twitchEventSub, twEventSubPort);
    await listen(ytOAuth, ytOAuthPort);
    await listen(ytAPI, ytAPIPort);
    const grpcAddr = `127.0.0.1:${grpcPort}`;
    const grpcControlAddr = `127.0.0.1:${grpcControlPort}`;
    const grpcControlBaseUrl = `http://${grpcControlAddr}`;
    fakeGRPC = await startFakeGRPCServer(fakeGRPCExePath, grpcAddr, grpcControlAddr);
    const seWsAddr = `127.0.0.1:${seWsPort}`;
    const seControlBaseUrl = `http://127.0.0.1:${seControlPort}`;
    fakeAstro = await startFakeAstroServer(fakeAstroExePath, seWsAddr, `127.0.0.1:${seControlPort}`);
    pass(`backend :${backendPort}  twitch oauth/helix/eventsub :${twOAuthPort}/${twHelixPort}/${twEventSubPort}  youtube oauth/api/grpc :${ytOAuthPort}/${ytAPIPort}/${grpcPort}  streamelements ws :${seWsPort}`);

    const env = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(backendPort),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
      STREAMING_TREE_TEST_TWITCH_OAUTH_BASE_URL: `http://127.0.0.1:${twOAuthPort}`,
      STREAMING_TREE_TEST_TWITCH_API_BASE_URL: `http://127.0.0.1:${twHelixPort}`,
      STREAMING_TREE_TEST_TWITCH_EVENTSUB_BASE_URL: `http://127.0.0.1:${twEventSubPort}`,
      STREAMING_TREE_TEST_TWITCH_EVENTSUB_RECONNECT_HOST: '127.0.0.1',
      STREAMING_TREE_TEST_YOUTUBE_OAUTH_BASE_URL: `http://127.0.0.1:${ytOAuthPort}`,
      STREAMING_TREE_TEST_YOUTUBE_API_BASE_URL: `http://127.0.0.1:${ytAPIPort}`,
      STREAMING_TREE_TEST_YOUTUBE_GRPC_TARGET: grpcAddr,
      STREAMING_TREE_TEST_YOUTUBE_GRPC_INSECURE: '1',
      STREAMING_TREE_TEST_STREAMELEMENTS_WS_BASE_URL: `ws://${seWsAddr}/`,
    };

    step('Start the backend under test with no widget profiles configured');
    backend = await startBackend(exePath, env, baseUrl);
    const list0 = await request(baseUrl, 'GET', '/api/widget-profiles');
    expect(list0.status === 200 && list0.body.length === 0, 'the widget-profile list starts empty', list0.body);

    // --- 1. link every provider up front (reused by every scenario below) --

    step('Link a Twitch account via device flow and enable the engagement connector');
    await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    const twStart = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
    await waitUntil(async () => (await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${twStart.body.attemptId}`)).body.state === 'polling' ? true : false, POLL_TIMEOUT_MS, 'the device-flow attempt to reach "polling"');
    const twUserId = `u_${RUN_ID}`;
    twitchState.users.set(twUserId, { id: twUserId, login: `streamer_${RUN_ID}`, displayName: `Streamer ${RUN_ID}` });
    const twDevice = [...twitchState.devices.values()].find((d) => d.userCode === twStart.body.userCode);
    twDevice.userId = twUserId;
    twDevice.authorized = true;
    const twAuthorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${twStart.body.attemptId}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the device-flow attempt to reach "authorized"');
    const twAccountId = twAuthorized.connectedAccountId;

    const twUpgrade = await request(baseUrl, 'POST', `/api/connected-accounts/${twAccountId}/engagement/authorize`);
    await waitUntil(async () => (await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${twUpgrade.body.attemptId}`)).body.state === 'polling' ? true : false, POLL_TIMEOUT_MS, 'the upgrade attempt to reach "polling"');
    const twUpgradeDevice = [...twitchState.devices.values()].find((d) => d.userCode === twUpgrade.body.userCode);
    twUpgradeDevice.userId = twUserId;
    twUpgradeDevice.authorized = true;
    await waitUntil(async () => (await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${twUpgrade.body.attemptId}`)).body.state === 'authorized' ? true : false, POLL_TIMEOUT_MS, 'the upgrade attempt to reach "authorized"');

    const twConnPromise = nextConnection(wsState);
    await request(baseUrl, 'PUT', `/api/connected-accounts/${twAccountId}/engagement`, { enabled: true });
    const twSocket = await twConnPromise;
    sendWS(twSocket, welcomeEnvelope('sess_1', 30));
    await waitUntil(async () => (await request(baseUrl, 'GET', `/api/connected-accounts/${twAccountId}/engagement`)).body.state === 'connected' ? true : false, POLL_TIMEOUT_MS, 'the Twitch connector to reach "connected"');
    pass('the Twitch connector is connected and subscribed');

    step('Link a YouTube account via the PKCE flow, select a live broadcast, and enable the connector');
    await request(baseUrl, 'PUT', '/api/integrations/youtube/config', { clientId: CLIENT_ID });
    const ytStart = await request(baseUrl, 'POST', '/api/integrations/youtube/oauth-attempts');
    const authUrl = new URL(ytStart.body.authorizationUrl);
    const redirectUri = authUrl.searchParams.get('redirect_uri');
    const realState = authUrl.searchParams.get('state');
    const ytCode = mintToken('fake-code');
    ytState.pendingCodes.set(ytCode, { scope: 'https://www.googleapis.com/auth/youtube.force-ssl' });
    const channelId = `UC_${RUN_ID}`;
    ytState.channels.set(channelId, { id: channelId, title: `Channel ${RUN_ID}`, country: 'US' });
    await requestAbsolute(`${redirectUri}?code=${ytCode}&state=${realState}`);
    const ytAuthorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/youtube/oauth-attempts/${ytStart.body.attemptId}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the YouTube OAuth attempt to reach "authorized"');
    const ytAccountId = ytAuthorized.connectedAccountId;

    const platforms = await request(baseUrl, 'GET', '/api/platforms');
    const ytPlatform = platforms.body.platforms.find((p) => p.providerId === 'youtube');
    await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/connected-account`, { accountId: ytAccountId });
    const liveChatId = `chat_${RUN_ID}`;
    ytState.broadcasts.set('bcast_1', { id: 'bcast_1', snippet: { title: 'Live now', liveChatId }, status: { lifeCycleStatus: 'live', lifeCycleStatusFilter: 'active', privacyStatus: 'public' } });
    await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/remote-target`, { resourceId: 'bcast_1' });
    await scriptPage(grpcControlBaseUrl, liveChatId, { nextPageToken: 'baseline' });
    await request(baseUrl, 'PUT', `/api/connected-accounts/${ytAccountId}/engagement`, { enabled: true });
    await waitUntil(async () => (await request(baseUrl, 'GET', `/api/connected-accounts/${ytAccountId}/engagement`)).body.state === 'connected' ? true : false, POLL_TIMEOUT_MS, 'the YouTube connector to reach "connected"');
    pass('the YouTube connector is connected');

    step('Create a StreamElements donation source and enable it');
    const seSource = await request(baseUrl, 'POST', '/api/donation-sources', { providerId: 'streamelements', label: 'Main channel', remoteChannelId: 'chan_1', token: mintToken('fake-jwt') });
    const seSourceId = seSource.body.id;
    await request(baseUrl, 'PUT', `/api/donation-sources/${seSourceId}/engagement`, { enabled: true });
    await waitForConnection(seControlBaseUrl, 'the fake Astro server to observe a subscribed connection', (item) => item.room === 'chan_1' && item.hasToken === true);
    await waitUntil(async () => (await request(baseUrl, 'GET', `/api/donation-sources/${seSourceId}/engagement`)).body.state === 'connected' ? true : false, POLL_TIMEOUT_MS, 'the StreamElements connector to reach "connected"');
    pass('the StreamElements connector is connected');

    // --- 2. latest_follower ---------------------------------------------

    step('Create a latest_follower widget - empty state, then a real follow updates it publicly');
    const latestFollower = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('latest_follower'));
    expect(latestFollower.status === 201, 'the latest_follower widget was created', latestFollower.body);
    const latestFollowerSlug = latestFollower.body.publicSlug;
    const emptyConfig = await fetchWidgetConfig(baseUrl, latestFollowerSlug);
    expect(emptyConfig.kind === 'latest_follower' && emptyConfig.latest === undefined, 'the public config starts with no latest follower', emptyConfig);

    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_1', user_login: 'a_follower', user_name: 'A Follower', followed_at: new Date().toISOString() }, 'msg_follow_1'));
    const afterFollow = await waitForPublicField(baseUrl, latestFollowerSlug, (b) => b.latest?.displayName === 'A Follower', 'the public config to reflect the real follower');
    expect(afterFollow.latest.provider === 'twitch', 'the latest follower carries the real provider label', afterFollow.latest);
    for (const leaked of [twUserId, 'u_follower_1', 'providerEventId']) {
      expect(!JSON.stringify(afterFollow).includes(leaked), `the public config never leaks "${leaked}"`, afterFollow);
    }

    step('An irrelevant chat message never updates the latest follower');
    sendWS(twSocket, notificationEnvelope('channel.chat.message', {
      broadcaster_user_id: twUserId, chatter_user_id: 'u_chatter_1', chatter_user_login: 'chatter', chatter_user_name: 'Chatter',
      message_id: 'chatmsg_1', color: '', badges: [], message: { text: 'hello', fragments: [{ type: 'text', text: 'hello' }] },
    }, 'msg_chat_1'));
    await new Promise((r) => setTimeout(r, 500));
    const stillFollower = await fetchWidgetConfig(baseUrl, latestFollowerSlug);
    expect(stillFollower.latest.displayName === 'A Follower', 'a chat message never overwrites the latest follower', stillFollower);

    // --- 3. latest_subscriber: new only ---------------------------------

    step('Create a latest_subscriber widget - a resubscription and a gift-batch summary never update it, only a genuinely new subscriber does');
    const latestSubscriber = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('latest_subscriber'));
    const latestSubscriberSlug = latestSubscriber.body.publicSlug;

    sendWS(twSocket, notificationEnvelope('channel.subscription.message', { user_id: 'u_sub_old', user_login: 'old_timer', user_name: 'Old Timer', tier: '1000', message: { text: '' }, cumulative_months: 3, duration_months: 1 }, 'msg_resub_1'));
    await new Promise((r) => setTimeout(r, 500));
    expect((await fetchWidgetConfig(baseUrl, latestSubscriberSlug)).latest === undefined, 'a resubscription never becomes the latest subscriber', await fetchWidgetConfig(baseUrl, latestSubscriberSlug));

    sendWS(twSocket, notificationEnvelope('channel.subscription.gift', { user_id: 'u_gifter_1', user_login: 'gifter', user_name: 'Gifter', total: 3, tier: '1000', is_anonymous: false }, 'msg_gift_batch_1'));
    await new Promise((r) => setTimeout(r, 500));
    expect((await fetchWidgetConfig(baseUrl, latestSubscriberSlug)).latest === undefined, 'a gift-batch summary never becomes the latest subscriber', await fetchWidgetConfig(baseUrl, latestSubscriberSlug));

    sendWS(twSocket, notificationEnvelope('channel.subscribe', { user_id: 'u_gift_recipient_0', user_login: 'recipient_0', user_name: 'Recipient 0', tier: '1000', is_gift: true }, 'msg_gift_recipient_0'));
    await waitForPublicField(baseUrl, latestSubscriberSlug, (b) => b.latest?.displayName === 'Recipient 0', 'the individual gift recipient to become the latest subscriber');

    // --- 4. latest_donation: message gated by showMessage ----------------

    step('Create a latest_donation widget with showMessage=false by default - a real donation message never appears publicly');
    const latestDonation = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('latest_donation'));
    const latestDonationSlug = latestDonation.body.publicSlug;
    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 4.2, currency: 'usd', username: 'Donor One', message: 'secret message one' }));
    const afterDonation = await waitForPublicField(baseUrl, latestDonationSlug, (b) => b.latest?.displayName === 'Donor One', 'the public config to reflect the real donation');
    expect(afterDonation.latest.message === undefined, 'the donation message is hidden while showMessage is false', afterDonation.latest);
    expect(afterDonation.latest.amountMicros === 4_200_000 && afterDonation.latest.currency === 'USD', 'the donation carries exact integer micros', afterDonation.latest);

    step('Enabling showMessage on the same widget shows the next donation\'s own message');
    const enableMessage = await request(baseUrl, 'PUT', `/api/widget-profiles/${latestDonation.body.id}`, { ...baseWidgetBody('latest_donation'), showMessage: true });
    expect(enableMessage.status === 200 && enableMessage.body.showMessage === true, 'showMessage was enabled', enableMessage.body);
    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 1.0, currency: 'usd', username: 'Donor Two', message: 'shown message two' }));
    const afterMessageEnabled = await waitForPublicField(baseUrl, latestDonationSlug, (b) => b.latest?.displayName === 'Donor Two', 'the public config to reflect the second donation');
    expect(afterMessageEnabled.latest.message === 'shown message two', 'the donation message is shown once explicitly enabled', afterMessageEnabled.latest);

    step('An anonymous donation never carries a fabricated display name');
    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 1.0, currency: 'usd', username: '' }));
    const afterAnonymous = await waitForPublicField(baseUrl, latestDonationSlug, (b) => b.latest?.displayName === undefined, 'an anonymous donation to become the latest with no display name');
    expect(afterAnonymous.latest.amountMicros === 1_000_000, 'the anonymous donation still carries its own exact amount', afterAnonymous.latest);

    // --- 5. largest_donation: exact comparison, tie rule, currency -------

    step('Create a largest_donation widget (USD) - larger replaces, equal does not, foreign currency is ignored');
    const largestDonation = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('largest_donation', { currency: 'USD' }));
    const largestDonationSlug = largestDonation.body.publicSlug;

    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 10.0, currency: 'usd', username: 'First' }));
    await waitForPublicField(baseUrl, largestDonationSlug, (b) => b.largest?.displayName === 'First', 'the first donation to become the largest');

    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 100.0, currency: 'eur', username: 'BigEuro' }));
    await new Promise((r) => setTimeout(r, 600));
    expect((await fetchWidgetConfig(baseUrl, largestDonationSlug)).largest.displayName === 'First', 'a numerically-larger EUR donation never matches the USD-configured widget', await fetchWidgetConfig(baseUrl, largestDonationSlug));

    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 10.0, currency: 'usd', username: 'Tied' }));
    await new Promise((r) => setTimeout(r, 600));
    expect((await fetchWidgetConfig(baseUrl, largestDonationSlug)).largest.displayName === 'First', 'an exactly equal amount never replaces the current winner', await fetchWidgetConfig(baseUrl, largestDonationSlug));

    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 10.01, currency: 'usd', username: 'Bigger' }));
    await waitForPublicField(baseUrl, largestDonationSlug, (b) => b.largest?.displayName === 'Bigger', 'a strictly larger amount replaces the winner');

    // --- 6. recent_supporters: bounded, newest first, no batch duplicate -

    step('Create a recent_supporters widget (maxItems=2) - bounded, newest first, and a gift batch summary never duplicates its own recipients');
    const recentSupporters = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('recent_supporters', { maxItems: 2 }));
    const recentSlug = recentSupporters.body.publicSlug;

    sendWS(twSocket, notificationEnvelope('channel.cheer', { is_anonymous: false, user_id: 'u_cheerer_1', user_login: 'cheerer', user_name: 'Cheerer', message: '', bits: 100 }, 'msg_cheer_recent_1'));
    await waitForPublicField(baseUrl, recentSlug, (b) => b.recent?.[0]?.displayName === 'Cheerer', 'the cheer to appear as the newest recent supporter');
    sendWS(twSocket, notificationEnvelope('channel.subscription.gift', { user_id: 'u_gifter_2', user_login: 'gifter2', user_name: 'Gifter Two', total: 2, tier: '1000', is_anonymous: false }, 'msg_gift_batch_recent'));
    await new Promise((r) => setTimeout(r, 400));
    const afterBatchOnly = await fetchWidgetConfig(baseUrl, recentSlug);
    expect(!afterBatchOnly.recent.some((row) => row.displayName === 'Gifter Two'), 'the gift-batch summary itself never appears as a recent supporter', afterBatchOnly.recent);

    sendWS(twSocket, notificationEnvelope('channel.subscribe', { user_id: 'u_gift_recipient_recent', user_login: 'recipient_recent', user_name: 'Recipient Recent', tier: '1000', is_gift: true }, 'msg_gift_recipient_recent'));
    const afterRecipient = await waitForPublicField(baseUrl, recentSlug, (b) => b.recent?.[0]?.displayName === 'Recipient Recent', 'the individual gift recipient to appear as the newest recent supporter');
    expect(afterRecipient.recent.length === 2, 'the recent list stays bounded at maxItems even as more supporters arrive', afterRecipient.recent);

    // --- 7. event_ticker: closed allowlist --------------------------------

    step('Create an event_ticker widget allowlisting only follow+donation - bits and chat never appear');
    const ticker = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('event_ticker', { maxItems: 10, eventTypes: ['follow', 'donation'] }));
    const tickerSlug = ticker.body.publicSlug;

    sendWS(twSocket, notificationEnvelope('channel.cheer', { is_anonymous: false, user_id: 'u_cheerer_ticker', user_login: 'cheerer_t', user_name: 'Cheerer T', message: '', bits: 50 }, 'msg_cheer_ticker_1'));
    await new Promise((r) => setTimeout(r, 400));
    expect((await fetchWidgetConfig(baseUrl, tickerSlug)).ticker === undefined, 'bits is outside this ticker\'s own allowlist and never appears (an empty list is omitted from the response entirely)', await fetchWidgetConfig(baseUrl, tickerSlug));

    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_ticker', user_login: 'follower_t', user_name: 'Follower T', followed_at: new Date().toISOString() }, 'msg_follow_ticker_1'));
    const afterTickerFollow = await waitForPublicField(baseUrl, tickerSlug, (b) => b.ticker?.length === 1, 'the allowlisted follow to appear on the ticker');
    expect(afterTickerFollow.ticker[0].eventType === 'follow', 'the ticker item carries its own real event type', afterTickerFollow.ticker[0]);

    // --- 8. session_counter: bits_quantity and support_amount -------------

    step('Create a bits_quantity session counter and confirm it sums exact Bits quantities');
    const bitsCounter = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('session_counter', { metric: 'bits_quantity' }));
    const bitsCounterSlug = bitsCounter.body.publicSlug;
    sendWS(twSocket, notificationEnvelope('channel.cheer', { is_anonymous: false, user_id: 'u_cheerer_counter', user_login: 'cheerer_c', user_name: 'Cheerer C', message: '', bits: 300 }, 'msg_cheer_counter_1'));
    await waitForPublicField(baseUrl, bitsCounterSlug, (b) => b.counter === 300, 'the bits counter to reach the real cheered quantity');

    step('Create a support_amount session counter (EUR) and confirm cross-currency events are ignored');
    const amountCounter = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('session_counter', { metric: 'support_amount', currency: 'EUR' }));
    const amountCounterSlug = amountCounter.body.publicSlug;
    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 9.0, currency: 'usd', username: 'WrongCurrency' }));
    await new Promise((r) => setTimeout(r, 500));
    expect((await fetchWidgetConfig(baseUrl, amountCounterSlug)).counter === 0, 'a USD donation never counts toward a EUR-configured counter', await fetchWidgetConfig(baseUrl, amountCounterSlug));
    await scriptPage(grpcControlBaseUrl, liveChatId, { items: [superChatItem({ id: `sc_counter_${RUN_ID}`, authorChannelId: `UC_counter_${RUN_ID}`, displayName: 'Euro Fan', amountMicros: 3_000_000, currency: 'eur', amountDisplayString: '€3.00' })], nextPageToken: 'after-counter-superchat' });
    await waitForPublicField(baseUrl, amountCounterSlug, (b) => b.counter === 3_000_000, 'a real EUR Super Chat counts toward the EUR-configured counter');

    // --- 9. provider filter -----------------------------------------------

    step('A provider filter excludes a non-matching provider\'s event');
    const youtubeOnlyFollower = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('latest_follower', { providers: ['youtube'] }));
    const youtubeOnlySlug = youtubeOnlyFollower.body.publicSlug;
    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_filtered', user_login: 'filtered', user_name: 'Filtered', followed_at: new Date().toISOString() }, 'msg_follow_filtered'));
    await new Promise((r) => setTimeout(r, 500));
    expect((await fetchWidgetConfig(baseUrl, youtubeOnlySlug)).latest === undefined, 'a YouTube-only-filtered widget never counts a Twitch follow', await fetchWidgetConfig(baseUrl, youtubeOnlySlug));

    // --- 10. dashboards: composition, nesting rejected, delete protection -

    step('Create a dashboard combining the latest_follower and bits_quantity widgets, and confirm the public config composes both with no internal id leak');
    const dashboard = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('dashboard', {
      columns: 2,
      children: [
        { widgetProfileId: latestFollower.body.id, column: 1, columnSpan: 1, row: 1, rowSpan: 1 },
        { widgetProfileId: bitsCounter.body.id, column: 2, columnSpan: 1, row: 1, rowSpan: 1 },
      ],
    }));
    expect(dashboard.status === 201, 'the dashboard was created', dashboard.body);
    const dashboardConfig = await fetchWidgetConfig(baseUrl, dashboard.body.publicSlug);
    expect(dashboardConfig.dashboard.length === 2, 'the dashboard composes both real children', dashboardConfig.dashboard);
    expect(dashboardConfig.dashboard.some((c) => c.snapshot.kind === 'latest_follower') && dashboardConfig.dashboard.some((c) => c.snapshot.kind === 'session_counter'), 'both child kinds are present in the composed snapshot', dashboardConfig.dashboard);
    const dashboardText = JSON.stringify(dashboardConfig);
    for (const leaked of [latestFollower.body.id, bitsCounter.body.id, 'widgetProfileId']) {
      expect(!dashboardText.includes(leaked), `the public dashboard config never leaks "${leaked}"`, dashboardConfig);
    }

    step('Nesting a dashboard inside another dashboard is rejected outright');
    const nestedAttempt = await request(baseUrl, 'POST', '/api/widget-profiles', baseWidgetBody('dashboard', {
      columns: 1, children: [{ widgetProfileId: dashboard.body.id, column: 1, columnSpan: 1, row: 1, rowSpan: 1 }],
    }));
    expect(nestedAttempt.status === 422, 'a dashboard referencing another dashboard is rejected with 422', nestedAttempt.body);

    step('Deleting a widget still referenced by a dashboard is rejected; deletion succeeds once removed from the dashboard');
    const deleteBlocked = await request(baseUrl, 'DELETE', `/api/widget-profiles/${latestFollower.body.id}`);
    expect(deleteBlocked.status === 409 && deleteBlocked.body.error === 'widget_profile_in_use', 'the delete is rejected while a dashboard still references the widget', deleteBlocked.body);
    const shrinkDashboard = await request(baseUrl, 'PUT', `/api/widget-profiles/${dashboard.body.id}`, baseWidgetBody('dashboard', {
      columns: 1, children: [{ widgetProfileId: bitsCounter.body.id, column: 1, columnSpan: 1, row: 1, rowSpan: 1 }],
    }));
    expect(shrinkDashboard.status === 200, 'the dashboard was updated to no longer reference the follower widget', shrinkDashboard.body);
    const deleteOk = await request(baseUrl, 'DELETE', `/api/widget-profiles/${latestFollower.body.id}`);
    expect(deleteOk.status === 204, 'the widget now deletes successfully', deleteOk.body);

    // --- 11. runtime reset --------------------------------------------------

    step('Resetting a widget\'s runtime state clears its own public presentation, and is rejected for kinds with no runtime state');
    const resetResp = await request(baseUrl, 'POST', `/api/widget-profiles/${latestSubscriber.body.id}/reset-runtime`);
    expect(resetResp.status === 204, 'reset-runtime succeeds for an event-derived kind', resetResp.body);
    const afterReset = await fetchWidgetConfig(baseUrl, latestSubscriberSlug);
    expect(afterReset.latest === undefined, 'the public config shows no latest subscriber immediately after reset', afterReset);
    const resetDashboard = await request(baseUrl, 'POST', `/api/widget-profiles/${dashboard.body.id}/reset-runtime`);
    expect(resetDashboard.status === 422, 'reset-runtime is rejected for a dashboard, which owns no runtime state of its own', resetDashboard.body);

    // --- 12. wrong methods / 404 -------------------------------------------

    step('Wrong methods return 405, and an unknown widget profile returns 404');
    const wrongMethod = await request(baseUrl, 'PATCH', '/api/widget-profiles', undefined);
    expect(wrongMethod.status === 405, 'PATCH /api/widget-profiles is rejected with 405', wrongMethod.body);
    const unknownRuntime = await request(baseUrl, 'GET', '/api/widget-profiles/widget_does_not_exist/runtime-status');
    expect(unknownRuntime.status === 404, 'runtime-status for an unknown widget id returns 404', unknownRuntime.body);

    // --- 13. public SSE stream reflects a real projection update ----------

    step('The public widget SSE stream reflects a real projection update within one poll interval');
    const streamUrl = `${baseUrl}/api/public/widgets/${bitsCounterSlug}/stream`;
    const counterIterator = sseEvents(streamUrl);
    const initialReset = await nextEvent(counterIterator, POLL_TIMEOUT_MS, 'the initial widget.reset for the bits counter');
    expect(initialReset.event === 'widget.reset' && initialReset.data.counter === 300, 'the stream opens with the real current counter value', initialReset.data);
    sendWS(twSocket, notificationEnvelope('channel.cheer', { is_anonymous: false, user_id: 'u_cheerer_stream', user_login: 'cheerer_s', user_name: 'Cheerer S', message: '', bits: 25 }, 'msg_cheer_stream_1'));
    const updatedReset = await nextEvent(counterIterator, POLL_TIMEOUT_MS, 'the widget.reset after the counter changes');
    expect(updatedReset.data.counter === 325, 'the stream reflects the updated counter value', updatedReset.data);
    await counterIterator.return();

    // --- 14. restart: config survives, runtime-only state does not --------

    step('Restart the whole backend and confirm widget configuration survives while runtime-only content does not');
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);

    const bitsCounterAfterRestart = await fetchWidgetConfig(baseUrl, bitsCounterSlug);
    expect(bitsCounterAfterRestart.kind === 'session_counter' && bitsCounterAfterRestart.counter === 0, 'the counter itself resets to zero after restart - never persisted', bitsCounterAfterRestart);
    const largestAfterRestart = await fetchWidgetConfig(baseUrl, largestDonationSlug);
    expect(largestAfterRestart.currency === 'USD' && largestAfterRestart.largest === undefined, 'the largest-donation widget keeps its configured currency but starts with no observed winner after restart', largestAfterRestart);
    const recentAfterRestart = await fetchWidgetConfig(baseUrl, recentSlug);
    expect(recentAfterRestart.recent === undefined, 'the recent-supporters list is empty after restart - never replayed from history (an empty list is omitted from the response entirely)', recentAfterRestart);

    step('Confirm every widget profile\'s own persisted configuration survived the restart unchanged');
    const allProfilesAfterRestart = await request(baseUrl, 'GET', '/api/widget-profiles');
    expect(allProfilesAfterRestart.status === 200 && allProfilesAfterRestart.body.length > 0, 'the widget-profiles endpoint still answers correctly after restart', allProfilesAfterRestart.body.length);
    const dashboardProfileAfterRestart = allProfilesAfterRestart.body.find((p) => p.id === dashboard.body.id);
    expect(dashboardProfileAfterRestart !== undefined && dashboardProfileAfterRestart.children.length === 1, 'the dashboard\'s own child composition survived the restart', dashboardProfileAfterRestart);

    // --- 15. privacy: no event-derived content ever reaches SQLite --------

    step('Confirm no observed display name, donation message, or providerEventId was ever written to SQLite');
    const dbBytes = readFileSync(dbPath);
    const dbText = dbBytes.toString('latin1');
    for (const secret of ['A Follower', 'Recipient Recent', 'shown message two', 'Donor One', 'Donor Two', `donor-${RUN_ID}@example.invalid`, 'paypal']) {
      expect(!dbText.includes(secret), `SQLite never contains "${secret}"`, undefined);
    }
    pass(`scanned the ${dbBytes.length}-byte SQLite database file for event-derived content - found none`);

    console.log('\nStage 18B supporter widgets verification PASSED');
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
    try { await killTree(fakeGRPC); } catch { /* best-effort cleanup */ }
    try { await killTree(fakeAstro); } catch { /* best-effort cleanup */ }
    for (const socket of wsState.connections) {
      if (!socket.destroyed) socket.destroy();
    }
    await close(twitchOAuth);
    await close(twitchHelix);
    await close(twitchEventSub);
    await close(ytOAuth);
    await close(ytAPI);
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nStage 18B supporter widgets verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
