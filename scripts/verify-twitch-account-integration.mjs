#!/usr/bin/env node
/**
 * Local, no-real-Twitch verification of the stage 7A connected-account and
 * Twitch metadata-publish integration.
 *
 * This script never contacts real Twitch. It runs the real backend under
 * test (`go build -tags integration ./cmd/testserver` - the same
 * credential-store-swapped twin verify-ffmpeg-branches.mjs uses) against two
 * small in-process fake HTTP servers that reproduce only the Twitch response
 * shapes this application actually parses:
 *
 *   fake OAuth server  (id.twitch.tv/oauth2 equivalent)
 *     /device, /token, /validate, /revoke
 *   fake Helix server  (api.twitch.tv/helix equivalent)
 *     /users, /channels, /search/categories
 *
 * The backend is pointed at them via STREAMING_TREE_TEST_TWITCH_OAUTH_BASE_URL
 * and STREAMING_TREE_TEST_TWITCH_API_BASE_URL - env vars that exist only in
 * the `-tags integration` binary (see cmd/testserver/main.go), never in a
 * production build.
 *
 * Every token, device code, user code and client ID used here is an
 * obviously-fake string generated for this run only. No real Twitch account,
 * application, or network request to Twitch is ever involved.
 *
 * Usage: node scripts/verify-twitch-account-integration.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { createServer as createHttpServer } from 'node:http';
import { mkdtempSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomUUID, randomBytes } from 'node:crypto';

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
    await new Promise((r) => setTimeout(r, 250));
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

// --- fake Twitch servers ----------------------------------------------------

/**
 * Shared in-memory state for both fake servers. Both run in this same Node
 * process (not spawned), so they read and mutate this object directly - no
 * IPC or test-only HTTP control endpoints are needed to force scenarios like
 * an expired access token, keeping the fake surface limited to what a real
 * Twitch response actually looks like.
 */
function newTwitchFakeState() {
  return {
    devices: new Map(), // deviceCode -> { userCode, pollCount, authorized, userId, scopes }
    accessTokens: new Map(), // token -> { valid, userId, scopes, refreshToken }
    refreshTokens: new Map(), // token -> { valid, userId, scopes }
    currentAccessTokenByUser: new Map(), // userId -> the most recently issued access token
    users: new Map(), // userId -> { id, login, displayName, avatarUrl }
    channels: new Map(), // userId -> { title, gameId, gameName, language, tags }
    categories: [
      { id: 'cat_509658', name: 'Just Chatting', boxArtUrl: 'https://fake.twitch.example/just-chatting.jpg' },
      { id: 'cat_1469308723', name: 'Software and Game Development', boxArtUrl: 'https://fake.twitch.example/dev.jpg' },
      { id: 'cat_32399', name: 'Counter-Strike', boxArtUrl: 'https://fake.twitch.example/cs.jpg' },
    ],
    refreshCallCount: 0,
    lastModifyBody: null,
    lastModifyKeys: null,
    lastSuccessfulHelixToken: null,
  };
}

function mintToken(prefix) {
  return `${prefix}-${RUN_ID}-${randomBytes(12).toString('hex')}`;
}

function issueTokenPair(state, userId, scopes) {
  const accessToken = mintToken('fake-access');
  const refreshToken = mintToken('fake-refresh');
  state.accessTokens.set(accessToken, { valid: true, userId, scopes, refreshToken });
  state.refreshTokens.set(refreshToken, { valid: true, userId, scopes });
  // The single source of truth for "whichever access token this user's
  // account currently holds" - a plain valid-flag scan is ambiguous once a
  // user has been issued more than one pair across the run (refresh,
  // reconnect), since nothing here retroactively invalidates an old pair
  // that was never explicitly revoked or rotated away.
  state.currentAccessTokenByUser.set(userId, accessToken);
  return { accessToken, refreshToken };
}

async function readBody(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  return Buffer.concat(chunks).toString('utf8');
}

// Every fake-server response closes its connection instead of keeping it
// alive for reuse: sparing a Go http.Client from ever reusing a pooled
// connection at the exact moment this Node server's own default
// keep-alive timeout tears it down, which otherwise shows up as an
// intermittent connection-reset on whichever poll happens to land after an
// idle gap (e.g. right after a slow_down backoff).
function sendJSON(res, status, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(status, { 'Content-Type': 'application/json', Connection: 'close' });
  res.end(body);
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
          if (device.pollCount === 2) {
            sendJSON(res, 400, { status: 400, message: 'slow_down' });
            return;
          }
          if (!device.authorized) {
            sendJSON(res, 400, { status: 400, message: 'authorization_pending' });
            return;
          }
          state.devices.delete(deviceCode); // a device code is exchanged at most once
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
          entry.valid = false; // one-time use: rotated away
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

function createFakeHelixServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');
      const auth = req.headers.authorization ?? '';
      const token = auth.startsWith('Bearer ') ? auth.slice('Bearer '.length) : '';
      const entry = state.accessTokens.get(token);
      const rateLimitHeaders = {
        'Ratelimit-Limit': '800', 'Ratelimit-Remaining': '799',
        'Ratelimit-Reset': String(Math.floor(Date.now() / 1000) + 60),
        Connection: 'close',
      };

      if (entry === undefined || !entry.valid) {
        res.writeHead(401, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({ error: 'Unauthorized', status: 401, message: 'Invalid OAuth token' }));
        return;
      }
      state.lastSuccessfulHelixToken = token;

      if (req.method === 'GET' && url.pathname === '/users') {
        const user = state.users.get(entry.userId);
        res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({
          data: [{
            id: user.id, login: user.login, display_name: user.displayName,
            profile_image_url: user.avatarUrl,
            // Unknown field: the real API adds fields over time; the client
            // must tolerate one it does not recognize.
            broadcaster_type: 'affiliate',
          }],
        }));
        return;
      }

      if (req.method === 'GET' && url.pathname === '/channels') {
        const broadcasterId = url.searchParams.get('broadcaster_id');
        const ch = state.channels.get(broadcasterId) ?? {
          title: '', gameId: '', gameName: '', language: '', tags: [],
        };
        res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({
          data: [{
            broadcaster_id: broadcasterId, broadcaster_login: '', broadcaster_name: '',
            broadcaster_language: ch.language, game_id: ch.gameId, game_name: ch.gameName,
            title: ch.title, delay: 0, tags: ch.tags,
          }],
        }));
        return;
      }

      if (req.method === 'PATCH' && url.pathname === '/channels') {
        const broadcasterId = url.searchParams.get('broadcaster_id');
        const raw = await readBody(req);
        record(raw); // never contains a token, but scan it anyway for defense in depth
        const parsed = JSON.parse(raw === '' ? '{}' : raw);
        state.lastModifyBody = parsed;
        state.lastModifyKeys = Object.keys(parsed).sort();
        const current = state.channels.get(broadcasterId) ?? { title: '', gameId: '', gameName: '', language: '', tags: [] };
        if (typeof parsed.title === 'string') current.title = parsed.title;
        if (typeof parsed.game_id === 'string') {
          current.gameId = parsed.game_id;
          const match = state.categories.find((c) => c.id === parsed.game_id);
          current.gameName = match?.name ?? '';
        }
        if (typeof parsed.broadcaster_language === 'string') current.language = parsed.broadcaster_language;
        if (Array.isArray(parsed.tags)) current.tags = parsed.tags;
        state.channels.set(broadcasterId, current);
        res.writeHead(204, { Connection: 'close' });
        res.end();
        return;
      }

      if (req.method === 'GET' && url.pathname === '/search/categories') {
        const query = (url.searchParams.get('query') ?? '').toLowerCase();
        const data = state.categories
          .filter((c) => c.name.toLowerCase().includes(query))
          .map((c) => ({ id: c.id, name: c.name, box_art_url: c.boxArtUrl }));
        res.writeHead(200, { 'Content-Type': 'application/json', ...rateLimitHeaders });
        res.end(JSON.stringify({ data }));
        return;
      }

      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { status: 500, message: String(error) });
    }
  });
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

async function main() {
  console.log('Twitch connected-account integration verification (local fakes only, no real Twitch)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-twitch-account-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const state = newTwitchFakeState();
  const oauthServer = createFakeOAuthServer(state);
  const helixServer = createFakeHelixServer(state);

  let backend = null;
  let envBackend = null;
  let baseUrl;
  let envBaseUrl;

  try {
    step('Build the integration-only test server (go build -tags integration ./cmd/testserver)');
    const build = spawnCaptured('go-build', 'go', ['build', '-tags', 'integration', '-o', exePath, './cmd/testserver'], { cwd: SERVER_DIR });
    const buildExit = await new Promise((r) => {
      const timer = setTimeout(() => r(-1), BUILD_TIMEOUT_MS);
      build.child.on('exit', (code) => { clearTimeout(timer); r(code); });
    });
    expect(buildExit === 0, 'the integration test server built successfully', build.getOutput());

    step('Reserve dynamic loopback ports and start the fake Twitch OAuth and Helix servers');
    const [backendPort, oauthPort, helixPort, envPort] = await reservePorts(4);
    baseUrl = `http://127.0.0.1:${backendPort}`;
    envBaseUrl = `http://127.0.0.1:${envPort}`;
    await listen(oauthServer, oauthPort);
    await listen(helixServer, helixPort);
    pass(`backend :${backendPort}  fake oauth :${oauthPort}  fake helix :${helixPort}`);

    const baseEnv = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
      STREAMING_TREE_TEST_TWITCH_OAUTH_BASE_URL: `http://127.0.0.1:${oauthPort}`,
      STREAMING_TREE_TEST_TWITCH_API_BASE_URL: `http://127.0.0.1:${helixPort}`,
    };

    // --- environment-sourced Client ID, in an isolated throwaway instance ---
    step('Confirm an environment-sourced Client ID is reported as such and cannot be edited (isolated instance)');
    const envDataDir = join(tempDir, 'env-data');
    mkdirSync(envDataDir, { recursive: true });
    envBackend = await startBackend(exePath, {
      ...baseEnv, STREAMING_TREE_DATA_DIR: envDataDir, STREAMING_TREE_PORT: String(envPort),
      STREAMING_TREE_TWITCH_CLIENT_ID: `${CLIENT_ID}-env`,
    }, envBaseUrl);
    const envConfig = await request(envBaseUrl, 'GET', '/api/integrations/twitch/config');
    expect(envConfig.status === 200 && envConfig.body.source === 'environment', 'config source is "environment"', envConfig.body);
    expect(!('clientId' in envConfig.body), 'the env-sourced Client ID value itself is not echoed back (database-managed values only)', envConfig.body);
    const envPut = await request(envBaseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: 'something-else' });
    expect(envPut.status === 409, 'editing an environment-sourced Client ID is rejected with 409', envPut);
    await stopBackend(envBackend, envBaseUrl);
    envBackend = null;
    pass('isolated environment-config instance stopped');

    // --- main flow: database-managed Client ID ---
    step('Start the backend under test (no Client ID configured yet)');
    backend = await startBackend(exePath, { ...baseEnv, STREAMING_TREE_PORT: String(backendPort) }, baseUrl);
    const config0 = await request(baseUrl, 'GET', '/api/integrations/twitch/config');
    expect(config0.status === 200 && config0.body.configured === false && config0.body.source === 'missing',
      'Twitch is reported unconfigured on first start', config0.body);

    step('Reject a client secret field on the integration-config endpoint');
    const rejectSecret = await request(baseUrl, 'PUT', '/api/integrations/twitch/config', {
      clientId: CLIENT_ID, clientSecret: 'must-be-rejected',
    });
    expect(rejectSecret.status === 400, 'a clientSecret field is rejected (400 unknown_field)', rejectSecret.body);
    expect(rejectSecret.body?.error === 'unknown_field', 'the error code is unknown_field', rejectSecret.body);

    step('Configure a database-managed Client ID');
    const setConfig = await request(baseUrl, 'PUT', '/api/integrations/twitch/config', { clientId: CLIENT_ID });
    expect(setConfig.status === 200 && setConfig.body.source === 'database' && setConfig.body.clientId === CLIENT_ID,
      'Client ID saved with source "database"', setConfig.body);

    step('Start a Twitch device-flow attempt');
    const start = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
    expect(start.status === 202, 'device-flow start is accepted (202)', start.body);
    const attemptId = start.body.attemptId;
    expect(typeof start.body.userCode === 'string' && start.body.userCode.length > 0, 'a user code is present', start.body.userCode);
    expect(!('deviceCode' in start.body), 'no deviceCode field exists in the response at all', Object.keys(start.body));

    step('A second concurrent device-flow attempt is rejected as a conflict');
    const conflict = await request(baseUrl, 'POST', '/api/integrations/twitch/device-flow');
    expect(conflict.status === 409, 'the duplicate attempt is rejected with 409', conflict.body);
    expect(conflict.body?.error === 'oauth_attempt_conflict', 'the error code is oauth_attempt_conflict', conflict.body);

    step('Poll the attempt: pending, then slow_down, both honored before authorization');
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'attempt to reach "polling"');
    pass('attempt reached "polling" after pending/slow_down responses from the fake server');

    step('Simulate the user completing authorization on Twitch (fake identity + real required scope)');
    const fakeUserId = `u_${RUN_ID}`;
    state.users.set(fakeUserId, {
      id: fakeUserId, login: `fakestreamer_${RUN_ID}`, displayName: `Fake Streamer ${RUN_ID}`,
      avatarUrl: 'https://fake.twitch.example/avatar.png',
    });
    state.channels.set(fakeUserId, { title: 'Old remote title', gameId: '', gameName: '', language: 'en', tags: ['old-tag'] });
    const device = [...state.devices.values()].find((d) => d.userCode === start.body.userCode);
    expect(device !== undefined, 'the fake server still has the matching device entry', start.body.userCode);
    device.userId = fakeUserId;
    device.scopes = ['channel:manage:broadcast'];
    device.authorized = true;

    step('Confirm the attempt reaches "authorized" and yields a connected account');
    const authorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${attemptId}`);
      if (snap.body.state === 'error') {
        throw new Error(`device-flow attempt entered an error state: ${snap.body.errorCode} - ${snap.body.errorMessage}`);
      }
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'attempt to reach "authorized"');
    expect(typeof authorized.connectedAccountId === 'string' && authorized.connectedAccountId.length > 0,
      'a connectedAccountId is present', authorized);
    const accountId = authorized.connectedAccountId;

    step('Confirm the account appears with no secret field anywhere in its representation');
    const accountsList = await request(baseUrl, 'GET', '/api/connected-accounts');
    expect(accountsList.status === 200 && accountsList.body.accounts.length === 1, 'exactly one connected account exists', accountsList.body);
    const acc = accountsList.body.accounts[0];
    expect(acc.id === accountId, 'the listed account matches the finalized attempt', acc);
    expect(acc.status === 'connected', 'the account status is connected', acc.status);
    expect(acc.login === `fakestreamer_${RUN_ID}`, 'the fake identity login was resolved via GET /users', acc.login);
    expect(acc.scopes.includes('channel:manage:broadcast'), 'the required scope is recorded as granted', acc.scopes);
    for (const forbidden of ['access_token', 'accessToken', 'refresh_token', 'refreshToken', 'deviceCode', 'device_code']) {
      expect(!(forbidden in acc), `the account response has no "${forbidden}" field`, Object.keys(acc));
    }

    step('Link the account to the seeded Twitch destination');
    const platforms = await request(baseUrl, 'GET', '/api/platforms');
    const twitchPlatform = platforms.body.platforms.find((p) => p.providerId === 'twitch');
    const kickPlatform = platforms.body.platforms.find((p) => p.providerId === 'kick');
    expect(twitchPlatform !== undefined, 'a seeded Twitch platform exists', platforms.body.platforms);

    if (kickPlatform !== undefined) {
      step('Reject linking a Twitch account to a non-Twitch destination');
      const mismatch = await request(baseUrl, 'PUT', `/api/platforms/${kickPlatform.id}/connected-account`, { accountId });
      expect(mismatch.status === 422, 'a provider mismatch is rejected (422)', mismatch.body);
      expect(mismatch.body?.error === 'account_provider_mismatch', 'the error code is account_provider_mismatch', mismatch.body);
    }

    const link = await request(baseUrl, 'PUT', `/api/platforms/${twitchPlatform.id}/connected-account`, { accountId });
    expect(link.status === 200 && link.body.accountId === accountId, 'the destination is linked to the account', link.body);

    step('Search Twitch categories and confirm the normalized shape');
    const search = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/twitch/categories?query=Software`);
    expect(search.status === 200 && search.body.items.length === 1, 'the category search returns the expected match', search.body);
    const category = search.body.items[0];
    expect(category.id === 'cat_1469308723' && category.name === 'Software and Game Development', 'category id and name are normalized', category);
    expect(!('boxArtUrl' in category) || typeof category.boxArtUrl === 'string', 'boxArtUrl, if present, is a plain string', category);

    step('Save local metadata with the selected category ID (Save is separate from Publish)');
    const saveMetadata = await request(baseUrl, 'PUT', `/api/platforms/${twitchPlatform.id}/metadata`, {
      title: `Live coding ${RUN_ID}`, description: '', category: category.name, categoryId: category.id,
      tags: ['coding', 'automated-test'], language: 'en', visibility: '', matureContent: false, dvr: false, latencyMode: '',
    });
    expect(saveMetadata.status === 200 && saveMetadata.body.categoryId === category.id, 'metadata saved with categoryId', saveMetadata.body);

    step('Publish preview reflects the real remote channel and the pending local change');
    const preview = await request(baseUrl, 'GET', `/api/platforms/${twitchPlatform.id}/metadata/publish-preview`);
    expect(preview.status === 200 && preview.body.allowed === true, 'publishing is allowed', preview.body);
    const titleDiff = preview.body.fields.find((f) => f.field === 'title');
    expect(titleDiff !== undefined && titleDiff.changed === true && titleDiff.remote === 'Old remote title',
      'the preview shows the real remote title and marks it changed', titleDiff);
    expect(preview.body.skipped.includes('description') && preview.body.skipped.includes('matureContent'),
      'unsupported fields are reported as skipped, never silently sent', preview.body.skipped);

    step('Publish rejects a request body');
    const publishWithBody = await request(baseUrl, 'POST', `/api/platforms/${twitchPlatform.id}/metadata/publish`, { title: 'should be rejected' });
    expect(publishWithBody.status === 400, 'a publish request body is rejected (400)', publishWithBody.body);

    step('Publish sends only the verified fields to the fake Helix channel endpoint');
    const publish = await request(baseUrl, 'POST', `/api/platforms/${twitchPlatform.id}/metadata/publish`);
    expect(publish.status === 200 && publish.body.status === 'published', 'publish succeeded', publish.body);
    expect(new Set(publish.body.fieldsChanged).has('title'), 'fieldsChanged reports title', publish.body.fieldsChanged);
    expect(JSON.stringify(state.lastModifyKeys) === JSON.stringify(['broadcaster_language', 'game_id', 'tags', 'title']),
      'the fake Helix server received exactly the four verified fields, nothing else', state.lastModifyKeys);
    expect(state.lastModifyBody.title === `Live coding ${RUN_ID}` && state.lastModifyBody.game_id === category.id,
      'the fake Helix server received the expected title and game_id', state.lastModifyBody);

    step('Confirm the remote channel now reflects the publish (fetched again through a fresh preview)');
    const previewAfterPublish = await request(baseUrl, 'GET', `/api/platforms/${twitchPlatform.id}/metadata/publish-preview`);
    const titleAfter = previewAfterPublish.body.fields.find((f) => f.field === 'title');
    expect(titleAfter.changed === false && titleAfter.remote === `Live coding ${RUN_ID}`,
      'the remote title now matches the local value', titleAfter);

    step('Force a 401 on the next Twitch call and confirm a single transparent refresh-and-retry');
    const staleToken = state.currentAccessTokenByUser.get(fakeUserId);
    expect(staleToken !== undefined, 'found the currently active access token to invalidate', null);
    state.accessTokens.get(staleToken).valid = false;
    const refreshCallsBefore = state.refreshCallCount;
    const searchAfter401 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/twitch/categories?query=Just`);
    expect(searchAfter401.status === 200 && searchAfter401.body.items.length === 1,
      'the category search still succeeds after the forced 401 (transparent refresh)', searchAfter401.body);
    expect(state.refreshCallCount === refreshCallsBefore + 1, 'exactly one refresh call was made', {
      before: refreshCallsBefore, after: state.refreshCallCount,
    });
    expect(state.lastSuccessfulHelixToken !== staleToken, 'the retry used the newly rotated access token, not the stale one', null);

    step('Validate the account explicitly and confirm it stays healthy');
    const validate = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/validate`);
    expect(validate.status === 200 && validate.body.status === 'connected', 'explicit validation reports connected', validate.body);
    expect(typeof validate.body.lastValidatedAt === 'string' && validate.body.lastValidatedAt.length > 0,
      'lastValidatedAt was updated', validate.body.lastValidatedAt);

    step('Restart the backend and confirm the account row and its destination link both persisted');
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, { ...baseEnv, STREAMING_TREE_PORT: String(backendPort) }, baseUrl);
    const linkAfterRestart = await request(baseUrl, 'GET', `/api/platforms/${twitchPlatform.id}/connected-account`);
    expect(linkAfterRestart.body?.accountId === accountId, 'the platform link persisted across a restart', linkAfterRestart.body);
    const accountAfterRestart = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}`);
    expect(accountAfterRestart.status === 200, 'the connected account row persisted across a restart', accountAfterRestart.body);
    // The token bundle itself does NOT survive this restart: cmd/testserver
    // deliberately backs it with an in-memory fake secret store (see its own
    // doc comment), never the real OS keychain a production restart would
    // use. That makes the account temporarily unusable for a Twitch call
    // until reconnected - exactly what the next step exercises.
    const staleAfterRestart = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/twitch/categories?query=Just`);
    expect(staleAfterRestart.status >= 400, 'a Twitch call fails cleanly (not a crash) with the in-memory bundle gone', staleAfterRestart.body);

    step('Reconnect the same account (same Twitch identity) and confirm no duplicate account is created');
    const reconnect = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/reconnect`);
    expect(reconnect.status === 202, 'reconnect starts a new device-flow attempt', reconnect.body);
    const reconnectAttemptId = reconnect.body.attemptId;
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${reconnectAttemptId}`);
      return snap.body.state === 'polling' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'reconnect attempt to reach "polling"');
    const reconnectDevice = [...state.devices.values()].find((d) => d.userId === null);
    expect(reconnectDevice !== undefined, 'the fake server has the reconnect device entry', null);
    reconnectDevice.userId = fakeUserId; // same identity as before
    reconnectDevice.scopes = ['channel:manage:broadcast'];
    reconnectDevice.authorized = true;
    const reconnected = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/twitch/device-flow/${reconnectAttemptId}`);
      if (snap.body.state === 'error') {
        throw new Error(`reconnect attempt entered an error state: ${snap.body.errorCode} - ${snap.body.errorMessage}`);
      }
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'reconnect attempt to reach "authorized"');
    expect(reconnected.connectedAccountId === accountId, 'reconnect resolved to the same account id, not a duplicate', reconnected);
    const accountsAfterReconnect = await request(baseUrl, 'GET', '/api/connected-accounts');
    expect(accountsAfterReconnect.body.accounts.length === 1, 'still exactly one connected account after reconnect', accountsAfterReconnect.body.accounts.length);

    step('Explicitly unlink the destination and confirm the account itself still exists');
    const unlink = await request(baseUrl, 'DELETE', `/api/platforms/${twitchPlatform.id}/connected-account`);
    expect(unlink.status === 204, 'unlink succeeds (204)', unlink);
    const linkAfterUnlink = await request(baseUrl, 'GET', `/api/platforms/${twitchPlatform.id}/connected-account`);
    expect(linkAfterUnlink.body === null, 'the destination now shows no linked account', linkAfterUnlink.body);
    const accountAfterUnlink = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}`);
    expect(accountAfterUnlink.status === 200, 'the account itself is unaffected by unlinking', accountAfterUnlink.body);

    step('Re-link, then disconnect: revoke, remove, and cascade the link automatically');
    await request(baseUrl, 'PUT', `/api/platforms/${twitchPlatform.id}/connected-account`, { accountId });
    const activeTokenBeforeDisconnect = state.currentAccessTokenByUser.get(fakeUserId);
    const disconnect = await request(baseUrl, 'DELETE', `/api/connected-accounts/${accountId}`);
    expect(disconnect.status === 204, 'disconnect succeeds (204)', disconnect);
    expect(state.accessTokens.get(activeTokenBeforeDisconnect)?.valid === false, 'the fake server actually received and honored the revoke call', null);
    const accountsAfterDisconnect = await request(baseUrl, 'GET', '/api/connected-accounts');
    expect(accountsAfterDisconnect.body.accounts.length === 0, 'no connected accounts remain', accountsAfterDisconnect.body);
    const linkAfterDisconnect = await request(baseUrl, 'GET', `/api/platforms/${twitchPlatform.id}/connected-account`);
    expect(linkAfterDisconnect.body === null, 'the platform link was cascade-removed by disconnect', linkAfterDisconnect.body);

    step('Search every captured HTTP body and backend log line for real secret material');
    const haystack = secretScanChunks.join('\n');
    const everyIssuedToken = [
      ...[...state.accessTokens.keys()],
      ...[...state.refreshTokens.keys()],
    ];
    for (const token of everyIssuedToken) {
      const index = haystack.indexOf(token);
      expect(index === -1, `token ${token.slice(0, 12)}... never appears in the backend's own responses or logs`,
        index === -1 ? undefined : haystack.slice(Math.max(0, index - 200), index + 200));
    }
    pass(`scanned ${haystack.length} bytes of backend stdout/stderr and HTTP response bodies for ${everyIssuedToken.length} issued tokens`);

    console.log('\nTwitch connected-account integration verification PASSED');
  } finally {
    if (backend !== null && baseUrl !== undefined) {
      try {
        await stopBackend(backend, baseUrl);
      } catch {
        // Already reporting a failure if we get here.
      }
    }
    if (envBackend !== null && envBaseUrl !== undefined) {
      try {
        await stopBackend(envBackend, envBaseUrl);
      } catch {
        // Already reporting a failure if we get here.
      }
    }
    await close(oauthServer);
    await close(helixServer);
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nTwitch connected-account integration verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
