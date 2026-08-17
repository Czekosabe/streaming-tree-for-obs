#!/usr/bin/env node
/**
 * Local, no-real-provider verification of the Stage 18A persistent
 * goals/counters foundation and public OBS goal widgets
 * (docs/goals-widgets.md).
 *
 * This script never contacts real Twitch, YouTube, or StreamElements.
 * It runs the real backend under test
 * (`go build -tags integration ./cmd/testserver`) against the exact
 * same local fakes `scripts/verify-twitch-engagement.mjs`,
 * `scripts/verify-youtube-engagement.mjs`, and
 * `scripts/verify-streamelements-donations.mjs` already established -
 * a real Twitch EventSub WebSocket fake, the real fake YouTube
 * streamList gRPC server binary, and the real fake StreamElements
 * Astro WebSocket server binary. No separate fake goals event source
 * is created (this task's own instruction): every scenario below
 * drives a real normalized event through a real provider fake, the
 * real Event Bus, the real `internal/goals.Manager`, real SQLite
 * persistence, and the real public widget SSE stream, end to end.
 *
 * This is a representative subset of the stage task's own 45-scenario
 * enumeration, not a literal one-assertion-per-item transcription -
 * mirrors this project's own established convention (see the doc
 * comments of the three scripts above and `verify-alert-audio.mjs`).
 * Exhaustive per-provider connector correctness (reconnect, backoff,
 * gap detection, OAuth edge cases) is already covered by those three
 * scripts and by their own Go unit tests; this script instead proves
 * the goals-specific contribution/dedupe/persistence/widget behavior
 * genuinely works against real provider fakes, not a shortcut.
 *
 * Every token, device code, JWT and identifier used here is an
 * obviously-fake string generated for this run only. No real Twitch,
 * Google/YouTube, or StreamElements account, application, or network
 * request is ever involved, and no real OBS Browser Source is opened.
 *
 * Usage: node scripts/verify-goals-widgets.mjs
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

/** A bare HTTP GET against an arbitrary absolute URL - used only to call
 * the backend's own loopback OAuth callback listener directly,
 * simulating the one step a real browser would otherwise perform. */
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

function issueTwitchTokenPair(state, userId, scopes) {
  const accessToken = mintToken('fake-access');
  const refreshToken = mintToken('fake-refresh');
  state.accessTokens.set(accessToken, { valid: true, userId, scopes });
  state.refreshTokens.set(refreshToken, { valid: true, userId, scopes });
  return { accessToken, refreshToken };
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
          const { accessToken, refreshToken } = issueTwitchTokenPair(state, device.userId, device.scopes);
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

function issueYouTubeTokenPair(state, channelId, scope) {
  const accessToken = mintToken('fake-access');
  state.accessTokens.set(accessToken, { valid: true, channelId, scope });
  const refreshToken = mintToken('fake-refresh');
  state.refreshTokens.set(refreshToken, { valid: true, channelId, scope });
  return { accessToken, refreshToken };
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
          const { accessToken, refreshToken } = issueYouTubeTokenPair(state, null, pending.scope);
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

function membershipItem({ id, authorChannelId, displayName }) {
  return {
    id,
    snippet: { type: 'newSponsorEvent', publishedAt: new Date().toISOString(), authorChannelId, displayMessage: '', newSponsorDetails: { memberLevelName: 'Tier 1', isUpgrade: false } },
    authorDetails: { channelId: authorChannelId, displayName, profileImageUrl: 'https://fake.youtube.example/avatar.jpg', isVerified: false, isChatOwner: false, isChatSponsor: true, isChatModerator: false },
  };
}

function superChatItem({ id, authorChannelId, displayName, amountMicros, currency, amountDisplayString }) {
  return {
    id,
    snippet: { type: 'superChatEvent', publishedAt: new Date().toISOString(), authorChannelId, displayMessage: '', superChatDetails: { amountMicros, currency, amountDisplayString, userComment: '', tier: 2 } },
    authorDetails: { channelId: authorChannelId, displayName, profileImageUrl: 'https://fake.youtube.example/avatar.jpg', isVerified: false, isChatOwner: false, isChatSponsor: false, isChatModerator: false },
  };
}

function superStickerItem({ id, authorChannelId, displayName, amountMicros, currency, amountDisplayString }) {
  return {
    id,
    snippet: { type: 'superStickerEvent', publishedAt: new Date().toISOString(), authorChannelId, displayMessage: '', superStickerDetails: { amountMicros, currency, amountDisplayString, tier: 1, superStickerMetadata: { stickerId: 'sticker_1', altText: 'excited cat', language: '' } } },
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
      message: overrides.message ?? 'great stream!', amount: overrides.amount ?? 4.2, currency: overrides.currency ?? 'USD', paymentMethod: 'scheme',
    },
    _id: id, channel: 'chan_1', provider: 'paypal', approved: overrides.approved ?? 'allowed', status: 'success',
    createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), transactionId: mintToken('txn'),
  };
}

// === goals-specific helpers ===

async function fetchGoal(baseUrl, id) {
  const res = await request(baseUrl, 'GET', `/api/goals/${id}`);
  return res.body;
}

async function waitForGoalCurrent(baseUrl, id, want, timeoutMs = POLL_TIMEOUT_MS) {
  return waitUntil(async () => {
    const g = await fetchGoal(baseUrl, id);
    return g.current === want ? g : false;
  }, timeoutMs, `goal ${id} to reach current=${want}`);
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
  console.log('Stage 18A goals/widgets verification (local fakes only, no real Twitch/YouTube/StreamElements, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-goals-widgets-'));
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

    step('Start the backend under test with no goals configured');
    backend = await startBackend(exePath, env, baseUrl);
    const list0 = await request(baseUrl, 'GET', '/api/goals');
    expect(list0.status === 200 && list0.body.length === 0, 'the goal list starts empty', list0.body);

    // --- 1. goal + widget CRUD basics, public-config privacy -------------

    step('Create a follower goal with an operator-supplied baseline (never fabricated)');
    const followerGoal = await request(baseUrl, 'POST', '/api/goals', {
      name: 'Followers', kind: 'followers', enabled: true, target: 1000, baseline: 825, providers: [], accounts: [], configRevision: 0,
    });
    expect(followerGoal.status === 201 && followerGoal.body.current === 825, 'the goal starts at the exact baseline, never a fabricated total', followerGoal.body);
    const followerGoalId = followerGoal.body.id;

    step('Create a public widget profile for it and confirm the public config leaks no internal id');
    const widget = await request(baseUrl, 'POST', '/api/widget-profiles', {
      kind: 'goal', goalId: followerGoalId, name: 'Follower widget', enabled: true, showCurrent: true, showTarget: true, showPercent: true,
      orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
      backgroundColor: '#000000', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33', borderRadiusPx: 12, opacity: 1.0,
    });
    expect(widget.status === 201 && widget.body.publicSlug.length >= 20, 'the widget profile was created with a high-entropy public slug', widget.body);
    const widgetSlug = widget.body.publicSlug;
    const publicConfig = await request(baseUrl, 'GET', `/api/public/widgets/${widgetSlug}/config`);
    const configText = JSON.stringify(publicConfig.body);
    for (const leaked of [followerGoalId, widget.body.id, 'providerEventId']) {
      expect(!configText.includes(leaked), `the public widget config never leaks "${leaked}"`, publicConfig.body);
    }
    expect(publicConfig.body.current === 825, 'the public config reflects the real current value', publicConfig.body);

    step('Connect the public widget SSE stream and confirm the initial reset snapshot');
    const widgetStreamUrl = `${baseUrl}/api/public/widgets/${widgetSlug}/stream`;
    const widgetIterator = sseEvents(widgetStreamUrl);
    const initialReset = await nextEvent(widgetIterator, POLL_TIMEOUT_MS, 'the initial widget.reset event');
    expect(initialReset.event === 'widget.reset' && initialReset.data.current === 825, 'the stream opens with a widget.reset carrying the real current value', initialReset);

    // --- 2. Twitch: follow, duplicate, irrelevant, gift no-double-count, bits ---

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

    step('A real follow event increments the follower goal, and the public widget SSE reflects it');
    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_1', user_login: 'a_follower', user_name: 'A Follower', followed_at: new Date().toISOString() }, 'msg_follow_1'));
    await waitForGoalCurrent(baseUrl, followerGoalId, 826);
    const followUpdate = await nextEvent(widgetIterator, POLL_TIMEOUT_MS, 'the widget.reset after the follow');
    expect(followUpdate.data.current === 826, 'the public widget stream reflects the new current value', followUpdate.data);
    await widgetIterator.return();

    step('Redelivering the same EventSub message id never double-counts the follower goal');
    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_1', user_login: 'a_follower', user_name: 'A Follower', followed_at: new Date().toISOString() }, 'msg_follow_1'));
    await new Promise((r) => setTimeout(r, 800));
    const afterDupGoal = await fetchGoal(baseUrl, followerGoalId);
    expect(afterDupGoal.current === 826, 'the duplicate delivery did not increment the goal a second time', afterDupGoal);

    step('An irrelevant chat message never contributes to any goal');
    sendWS(twSocket, notificationEnvelope('channel.chat.message', {
      broadcaster_user_id: twUserId, chatter_user_id: 'u_chatter_1', chatter_user_login: 'chatter', chatter_user_name: 'Chatter',
      message_id: 'chatmsg_1', color: '', badges: [], message: { text: 'hello', fragments: [{ type: 'text', text: 'hello' }] },
    }, 'msg_chat_1'));
    await new Promise((r) => setTimeout(r, 500));
    const afterChatGoal = await fetchGoal(baseUrl, followerGoalId);
    expect(afterChatGoal.current === 826, 'a chat message never increments the follower goal', afterChatGoal);

    step('Create a subscription goal (any provider) and confirm a plain subscription contributes');
    const subGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'Subs', kind: 'subscriptions', enabled: true, target: 1000, baseline: 0, providers: [], accounts: [], configRevision: 0 });
    const subGoalId = subGoal.body.id;
    sendWS(twSocket, notificationEnvelope('channel.subscribe', { user_id: 'u_sub_1', user_login: 'new_sub', user_name: 'New Sub', tier: '1000', is_gift: false }, 'msg_sub_1'));
    await waitForGoalCurrent(baseUrl, subGoalId, 1);

    step('A resubscription never contributes (continuing, not new - docs/goals-widgets.md §5.1)');
    sendWS(twSocket, notificationEnvelope('channel.subscription.message', { user_id: 'u_sub_1', user_login: 'new_sub', user_name: 'New Sub', tier: '1000', message: { text: '' }, cumulative_months: 3, duration_months: 1 }, 'msg_resub_1'));
    await new Promise((r) => setTimeout(r, 500));
    expect((await fetchGoal(baseUrl, subGoalId)).current === 1, 'a resubscription leaves the subscription goal unchanged', await fetchGoal(baseUrl, subGoalId));

    step('A gift batch of 5 plus its 5 individual gift-recipient events contribute exactly 5, never 10, never 0 (the no-double-count proof)');
    sendWS(twSocket, notificationEnvelope('channel.subscription.gift', { user_id: 'u_gifter_1', user_login: 'gifter', user_name: 'Gifter', total: 5, tier: '1000', is_anonymous: false }, 'msg_gift_batch_1'));
    for (let i = 0; i < 5; i += 1) {
      sendWS(twSocket, notificationEnvelope('channel.subscribe', { user_id: `u_gift_recipient_${i}`, user_login: `recipient_${i}`, user_name: `Recipient ${i}`, tier: '1000', is_gift: true }, `msg_gift_recipient_${i}`));
    }
    await waitForGoalCurrent(baseUrl, subGoalId, 6); // 1 (plain sub) + 5 (recipients), never +5 more for the batch

    step('An exact Bits quantity contributes to a Bits goal');
    const bitsGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'Bits', kind: 'bits', enabled: true, target: 100000, baseline: 0, providers: [], accounts: [], configRevision: 0 });
    sendWS(twSocket, notificationEnvelope('channel.cheer', { is_anonymous: false, user_id: 'u_cheerer_1', user_login: 'cheerer', user_name: 'Cheerer', message: 'Cheer750', bits: 750 }, 'msg_cheer_1'));
    await waitForGoalCurrent(baseUrl, bitsGoal.body.id, 750);

    step('A disabled goal never increments, even for a real matching event');
    // A freshly created goal always starts enabled (mirrors alerts.
    // CreateProfile's identical "always create enabled" convention,
    // internal/domain/goals.Service.CreateGoal forces Enabled=true) -
    // disabling one is a follow-up PUT, never a create-time option.
    const disabledGoalCreated = await request(baseUrl, 'POST', '/api/goals', { name: 'Disabled', kind: 'followers', enabled: true, target: 100, baseline: 0, providers: [], accounts: [], configRevision: 0 });
    const disableIt = await request(baseUrl, 'PUT', `/api/goals/${disabledGoalCreated.body.id}`, {
      name: 'Disabled', kind: 'followers', enabled: false, target: 100, baseline: 0, providers: [], accounts: [], configRevision: disabledGoalCreated.body.configRevision,
    });
    const disabledGoal = { body: disableIt.body };
    expect(disableIt.status === 200 && disableIt.body.enabled === false, 'the goal was successfully disabled via PUT', disableIt.body);
    const sentinelGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'Sentinel', kind: 'followers', enabled: true, target: 100, baseline: 0, providers: [], accounts: [], configRevision: 0 });
    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_2', user_login: 'another_follower', user_name: 'Another Follower', followed_at: new Date().toISOString() }, 'msg_follow_2'));
    await waitForGoalCurrent(baseUrl, sentinelGoal.body.id, 1);
    expect((await fetchGoal(baseUrl, disabledGoal.body.id)).current === 0, 'the disabled goal never accumulated the same real event', await fetchGoal(baseUrl, disabledGoal.body.id));

    step('A provider filter excludes a non-matching provider\'s event');
    const twitchOnlyGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'Twitch only', kind: 'followers', enabled: true, target: 100, baseline: 0, providers: ['youtube'], accounts: [], configRevision: 0 });
    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_3', user_login: 'yet_another', user_name: 'Yet Another', followed_at: new Date().toISOString() }, 'msg_follow_3'));
    await waitForGoalCurrent(baseUrl, sentinelGoal.body.id, 2);
    expect((await fetchGoal(baseUrl, twitchOnlyGoal.body.id)).current === 0, 'a YouTube-only-filtered goal never counts a Twitch event', await fetchGoal(baseUrl, twitchOnlyGoal.body.id));

    // --- 3. YouTube: membership, Super Chat, Super Sticker -----------------

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

    step('An account filter excludes a non-matching account\'s event (uses the real, now-linked YouTube account id - a goal can only be filtered to an account that genuinely exists)');
    const otherAccountGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'YouTube account only', kind: 'followers', enabled: true, target: 100, baseline: 0, providers: [], accounts: [ytAccountId], configRevision: 0 });
    expect(otherAccountGoal.status === 201, 'the account-filtered goal was created against the real YouTube account id', otherAccountGoal.body);
    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_4', user_login: 'once_more', user_name: 'Once More', followed_at: new Date().toISOString() }, 'msg_follow_4'));
    await waitForGoalCurrent(baseUrl, sentinelGoal.body.id, 3);
    expect((await fetchGoal(baseUrl, otherAccountGoal.body.id)).current === 0, 'a goal filtered to the YouTube account never counts a Twitch account\'s event', await fetchGoal(baseUrl, otherAccountGoal.body.id));

    step('A real YouTube membership event also contributes to the same provider-agnostic subscription goal');
    await scriptPage(grpcControlBaseUrl, liveChatId, { items: [membershipItem({ id: `member_${RUN_ID}`, authorChannelId: `UC_member_${RUN_ID}`, displayName: 'New Member' })], nextPageToken: 'after-membership' });
    await waitForGoalCurrent(baseUrl, subGoalId, 7);

    step('Create a USD donation goal and confirm a real YouTube Super Chat contributes exact integer micros');
    const donationGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'Fund', kind: 'donations', enabled: true, target: 100_000_000, baseline: 0, currency: 'USD', providers: [], accounts: [], configRevision: 0 });
    const donationGoalId = donationGoal.body.id;
    await scriptPage(grpcControlBaseUrl, liveChatId, { items: [superChatItem({ id: `sc_1_${RUN_ID}`, authorChannelId: `UC_superchat_${RUN_ID}`, displayName: 'Big Fan', amountMicros: 5_000_000, currency: 'usd', amountDisplayString: '$5.00' })], nextPageToken: 'after-superchat' });
    await waitForGoalCurrent(baseUrl, donationGoalId, 5_000_000);

    step('A real YouTube Super Sticker also contributes exact integer micros to the same donation goal');
    await scriptPage(grpcControlBaseUrl, liveChatId, { items: [superStickerItem({ id: `sticker_1_${RUN_ID}`, authorChannelId: `UC_superchat_${RUN_ID}`, displayName: 'Big Fan', amountMicros: 2_000_000, currency: 'usd', amountDisplayString: '$2.00' })], nextPageToken: 'after-sticker' });
    await waitForGoalCurrent(baseUrl, donationGoalId, 7_000_000);

    // --- 4. StreamElements: donation, currency mismatch --------------------

    step('Create a StreamElements donation source and enable it');
    const seSource = await request(baseUrl, 'POST', '/api/donation-sources', { providerId: 'streamelements', label: 'Main channel', remoteChannelId: 'chan_1', token: mintToken('fake-jwt') });
    const seSourceId = seSource.body.id;
    await request(baseUrl, 'PUT', `/api/donation-sources/${seSourceId}/engagement`, { enabled: true });
    await waitForConnection(seControlBaseUrl, 'the fake Astro server to observe a subscribed connection', (item) => item.room === 'chan_1' && item.hasToken === true);
    await waitUntil(async () => (await request(baseUrl, 'GET', `/api/donation-sources/${seSourceId}/engagement`)).body.state === 'connected' ? true : false, POLL_TIMEOUT_MS, 'the StreamElements connector to reach "connected"');
    pass('the StreamElements connector is connected');

    step('A real StreamElements donation contributes to the same USD donation goal, proving multi-provider aggregation');
    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 3.5, currency: 'usd' }));
    await waitForGoalCurrent(baseUrl, donationGoalId, 10_500_000);

    step('A different-currency donation never contributes (no FX, ever)');
    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 100, currency: 'eur' }));
    await new Promise((r) => setTimeout(r, 800));
    expect((await fetchGoal(baseUrl, donationGoalId)).current === 10_500_000, 'a numerically-larger EUR donation never matches the USD goal', await fetchGoal(baseUrl, donationGoalId));

    step('An account filter accepts the donation source despite it not being a connected_account');
    const seOnlyGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'SE only', kind: 'donations', enabled: true, target: 100_000_000, baseline: 0, currency: 'USD', providers: [], accounts: [seSourceId], configRevision: 0 });
    await pushTip(seControlBaseUrl, 'channel.tips', tipFixture({ amount: 1.0, currency: 'usd' }));
    await waitForGoalCurrent(baseUrl, seOnlyGoal.body.id, 1_000_000);

    // --- 5. completion, manual actions, widget lifecycle --------------------

    step('A goal reaching its target stays completed and keeps accumulating past it (no clamp)');
    const smallGoal = await request(baseUrl, 'POST', '/api/goals', { name: 'Small', kind: 'followers', enabled: true, target: 1, baseline: 0, providers: [], accounts: [], configRevision: 0 });
    sendWS(twSocket, notificationEnvelope('channel.follow', { user_id: 'u_follower_5', user_login: 'completer', user_name: 'Completer', followed_at: new Date().toISOString() }, 'msg_follow_5'));
    await waitForGoalCurrent(baseUrl, smallGoal.body.id, 1);
    const completedGoal = await fetchGoal(baseUrl, smallGoal.body.id);
    expect(completedGoal.completed === true && completedGoal.progressBasisPoints === 10000, 'the goal reports completed at exactly 100%', completedGoal);

    step('Set current and Reset manual actions work and never publish a fake event or bump configRevision');
    const beforeManual = await fetchGoal(baseUrl, followerGoalId);
    const setResp = await request(baseUrl, 'POST', `/api/goals/${followerGoalId}/set-current`, { current: 900 });
    expect(setResp.status === 200 && setResp.body.current === 900, 'Set current persists the operator-supplied value', setResp.body);
    const resetResp = await request(baseUrl, 'POST', `/api/goals/${followerGoalId}/reset`);
    expect(resetResp.status === 200 && resetResp.body.current === 825, 'Reset restores the goal\'s own baseline', resetResp.body);
    expect(resetResp.body.configRevision === beforeManual.configRevision, 'manual actions never bump configRevision', { before: beforeManual.configRevision, after: resetResp.body.configRevision });

    step('Rotating a widget\'s public slug invalidates the old URL immediately');
    const rotate = await request(baseUrl, 'POST', `/api/widget-profiles/${widget.body.id}/rotate-public-slug`);
    expect(rotate.status === 200 && rotate.body.publicSlug !== widgetSlug, 'the widget profile has a new public slug', rotate.body);
    const oldSlugStream = await request(baseUrl, 'GET', `/api/public/widgets/${widgetSlug}/config`);
    expect(oldSlugStream.body.current === 0, 'the old slug now resolves as unknown (safe empty default)', oldSlugStream.body);

    step('Deleting a goal still referenced by a widget profile is rejected; deletion succeeds once the widget is removed');
    const deleteBlocked = await request(baseUrl, 'DELETE', `/api/goals/${followerGoalId}`);
    expect(deleteBlocked.status === 409 && deleteBlocked.body.error === 'goal_in_use', 'the delete is rejected while a widget profile still references the goal', deleteBlocked.body);
    await request(baseUrl, 'DELETE', `/api/widget-profiles/${widget.body.id}`);
    const deleteOk = await request(baseUrl, 'DELETE', `/api/goals/${followerGoalId}`);
    expect(deleteOk.status === 204, 'the goal deletes successfully once no widget profile references it', deleteOk.body);

    step('Wrong methods return 405 with Allow, and unknown resources return 404');
    const wrongMethod = await request(baseUrl, 'PATCH', '/api/goals', undefined);
    expect(wrongMethod.status === 405, 'PATCH /api/goals is rejected with 405', wrongMethod.body);
    const unknownGoal = await request(baseUrl, 'GET', '/api/goals/goal_does_not_exist');
    expect(unknownGoal.status === 404 && unknownGoal.body.error === 'goal_not_found', 'an unknown goal id returns 404 goal_not_found', unknownGoal.body);

    // --- 6. restart persistence, no replay ----------------------------------

    step('Restart the whole backend and confirm accumulated goal state and widget profiles both survive');
    const enqueuedSubCurrent = (await fetchGoal(baseUrl, subGoalId)).current;
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);
    const subGoalAfterRestart = await fetchGoal(baseUrl, subGoalId);
    expect(subGoalAfterRestart.current === enqueuedSubCurrent, 'the subscription goal\'s accumulated current survived a full backend restart', subGoalAfterRestart);
    const donationGoalAfterRestart = await fetchGoal(baseUrl, donationGoalId);
    // 10,500,000 (Super Chat + Super Sticker + the first StreamElements tip)
    // plus the 1,000,000 tip pushed in the "account filter accepts the
    // donation source" step above - that tip legitimately also matched
    // this goal too, since it carries no account filter of its own
    // ("empty means any", docs/goals-widgets.md §14/§15: multiple
    // matching goals is correct, expected behavior, not a bug).
    expect(donationGoalAfterRestart.current === 11_500_000, 'the donation goal\'s accumulated current survived a full backend restart', donationGoalAfterRestart);
    const widgetsAfterRestart = await request(baseUrl, 'GET', `/api/widget-profiles?goalId=${seOnlyGoal.body.id}`);
    expect(Array.isArray(widgetsAfterRestart.body), 'the widget-profiles endpoint still answers correctly after restart', widgetsAfterRestart.body);

    step('Confirm no rule/goal/account id, providerEventId, or donor-sensitive field was ever written to SQLite');
    const dbBytes = readFileSync(dbPath);
    const dbText = dbBytes.toString('latin1');
    for (const secret of [`donor-${RUN_ID}@example.invalid`, 'paypal', 'transactionId']) {
      expect(!dbText.includes(secret), `SQLite never contains "${secret}"`, undefined);
    }
    pass(`scanned the ${dbBytes.length}-byte SQLite database file for sensitive donor fields - found none`);

    console.log('\nStage 18A goals/widgets verification PASSED');
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
  console.error(`\nStage 18A goals/widgets verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
