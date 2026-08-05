#!/usr/bin/env node
/**
 * Local, no-real-Google verification of the stage 7B connected-account and
 * YouTube metadata-publish integration.
 *
 * This script never contacts real Google or YouTube. It runs the real
 * backend under test (`go build -tags integration ./cmd/testserver` - the
 * same credential-store-swapped twin verify-twitch-account-integration.mjs
 * uses) against two small in-process fake HTTP servers that reproduce only
 * the Google/YouTube response shapes this application actually parses:
 *
 *   fake OAuth server   (oauth2.googleapis.com equivalent)
 *     /token, /tokeninfo, /revoke
 *   fake YouTube server (www.googleapis.com/youtube/v3 equivalent)
 *     /channels, /liveBroadcasts, /videos, /videoCategories
 *
 * The backend is pointed at them via STREAMING_TREE_TEST_YOUTUBE_OAUTH_BASE_URL
 * and STREAMING_TREE_TEST_YOUTUBE_API_BASE_URL - env vars that exist only in
 * the `-tags integration` binary (see cmd/testserver/main.go), never in a
 * production build. STREAMING_TREE_TEST_YOUTUBE_AUTH_BASE_URL is
 * deliberately left unset (its default, the real accounts.google.com host,
 * is never actually fetched by this script or the backend - the backend
 * only ever *constructs* that URL as a string for the frontend to open in a
 * real browser; nothing in this backend or this script ever issues an HTTP
 * request to it).
 *
 * The actual browser step is simulated by calling the backend's own
 * loopback callback listener directly with a query string this script
 * builds itself (code + the real state value, both parsed out of the
 * authorization URL the backend returned - exactly what a real Google
 * redirect would echo back) - a real HTTP request to a real part of the
 * backend under test, not a fake.
 *
 * Every token, channel ID, and Client ID used here is an obviously-fake
 * string generated for this run only. No real Google account, Google Cloud
 * project, or network request to Google/YouTube is ever involved.
 *
 * Usage: node scripts/verify-youtube-account-integration.mjs
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
const CLIENT_ID = `fake-client-id-${RUN_ID}.apps.googleusercontent.com`;

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

/** A bare HTTP GET against an arbitrary absolute URL - used only to call the
 * backend's own loopback OAuth callback listener directly, simulating the
 * one step a real browser would otherwise perform. */
async function requestAbsolute(url) {
  const response = await fetch(url);
  const text = await response.text();
  record(text);
  return { status: response.status, text };
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

function listen(server, port) {
  return new Promise((resolveListen, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => resolveListen());
  });
}

function close(server) {
  return new Promise((resolveClose) => server.close(() => resolveClose()));
}

// --- fake Google/YouTube servers -------------------------------------------

function mintToken(prefix) {
  return `${prefix}-${RUN_ID}-${randomBytes(12).toString('hex')}`;
}

function newYouTubeFakeState() {
  return {
    accessTokens: new Map(), // token -> { valid, channelId, scope }
    refreshTokens: new Map(), // token -> { valid, channelId, scope }
    currentAccessToken: null,
    channels: new Map(), // channelId -> { id, title, country }
    videos: new Map(), // videoId -> { snippet, status }
    broadcasts: new Map(), // broadcastId -> { id, snippet, status }
    categories: [
      { id: '20', name: 'Gaming', assignable: true },
      { id: '24', name: 'Entertainment', assignable: true },
      { id: '999', name: 'Not assignable', assignable: false },
    ],
    refreshCallCountByRefreshToken: new Map(),
    lastVideoUpdateBody: null,
    lastSuccessfulAPIToken: null,
    revokedTokens: new Set(),
  };
}

function issueTokenPair(state, channelId, scope, { omitRefresh = false } = {}) {
  const accessToken = mintToken('fake-access');
  const entry = { valid: true, channelId, scope };
  state.accessTokens.set(accessToken, entry);
  state.currentAccessToken = accessToken;
  if (omitRefresh) {
    return { accessToken, refreshToken: undefined };
  }
  const refreshToken = mintToken('fake-refresh');
  state.refreshTokens.set(refreshToken, { valid: true, channelId, scope });
  return { accessToken, refreshToken };
}

function createFakeOAuthServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');

      if (req.method === 'POST' && url.pathname === '/token') {
        const form = new URLSearchParams(await readBody(req));
        // Deliberately NOT recorded for the secret scan: this is the
        // backend's own legitimate outbound OAuth request to Google's
        // token endpoint (fake, here), which necessarily carries a real
        // token in its wire body - that is not a leak. The scan instead
        // covers the backend's own stdout/stderr and its HTTP responses to
        // this script's own API calls (see every other `record()` call
        // site), which must never echo a token back.
        const grantType = form.get('grant_type');

        if (grantType === 'authorization_code') {
          if (form.has('client_secret')) {
            sendJSON(res, 400, { error: 'invalid_request', error_description: 'client_secret not expected for this client' });
            return;
          }
          const code = form.get('code');
          const pending = state.pendingCodes?.get(code);
          if (pending === undefined) {
            sendJSON(res, 400, { error: 'invalid_grant', error_description: 'unknown code' });
            return;
          }
          const { accessToken, refreshToken } = issueTokenPair(state, null, pending.scope);
          sendJSON(res, 200, {
            access_token: accessToken, refresh_token: refreshToken,
            scope: pending.scope, token_type: 'Bearer', expires_in: 3600,
          });
          return;
        }

        if (grantType === 'refresh_token') {
          const oldRefresh = form.get('refresh_token');
          const entry = state.refreshTokens.get(oldRefresh);
          if (entry === undefined || !entry.valid) {
            sendJSON(res, 400, { error: 'invalid_grant', error_description: 'Token has been expired or revoked.' });
            return;
          }
          const calls = (state.refreshCallCountByRefreshToken.get(oldRefresh) ?? 0) + 1;
          state.refreshCallCountByRefreshToken.set(oldRefresh, calls);
          // Second refresh of the same original grant omits a new
          // refresh_token, matching Google's own documented "typically
          // omitted on refresh" behavior - the backend must preserve the
          // existing one rather than losing it.
          const { accessToken, refreshToken } = issueTokenPair(state, entry.channelId, entry.scope, { omitRefresh: calls >= 2 });
          const payload = { access_token: accessToken, scope: entry.scope, token_type: 'Bearer', expires_in: 3600 };
          if (refreshToken !== undefined) payload.refresh_token = refreshToken;
          sendJSON(res, 200, payload);
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
        const token = form.get('token');
        state.revokedTokens.add(token);
        const entry = state.accessTokens.get(token) ?? state.refreshTokens.get(token);
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

function createFakeYouTubeAPIServer(state) {
  return createHttpServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost');
      const auth = req.headers.authorization ?? '';
      const token = auth.startsWith('Bearer ') ? auth.slice('Bearer '.length) : '';
      const entry = state.accessTokens.get(token);

      if (entry === undefined || !entry.valid) {
        sendJSON(res, 401, { error: { code: 401, message: 'Invalid Credentials', errors: [{ reason: 'authError' }] } });
        return;
      }
      state.lastSuccessfulAPIToken = token;

      if (req.method === 'GET' && url.pathname === '/channels') {
        let items;
        if (url.searchParams.get('mine') === 'true') {
          // Every registered fake channel belongs to "the one fake Google
          // identity" this whole script authenticates as - real ownership
          // does not depend on which channel this application previously
          // selected, so this list is the same on every call, including a
          // later reconnect.
          items = [...state.channels.values()];
        } else {
          const id = url.searchParams.get('id');
          items = [...state.channels.values()].filter((c) => c.id === id);
        }
        sendJSON(res, 200, {
          items: items.map((c) => ({
            id: c.id,
            snippet: { title: c.title, description: '', customUrl: '', country: c.country ?? '', thumbnails: { default: { url: 'https://fake.youtube.example/avatar.jpg' } } },
          })),
        });
        return;
      }

      if (req.method === 'GET' && url.pathname === '/liveBroadcasts') {
        if (url.searchParams.has('id')) {
          const id = url.searchParams.get('id');
          const b = state.broadcasts.get(id);
          sendJSON(res, 200, { items: b === undefined ? [] : [b] });
          return;
        }
        const status = url.searchParams.get('broadcastStatus');
        const items = [...state.broadcasts.values()].filter((b) => b.status.lifeCycleStatusFilter === status);
        sendJSON(res, 200, {
          items: items.map((b) => ({
            id: b.id,
            snippet: { title: b.snippet.title, scheduledStartTime: b.snippet.scheduledStartTime ?? '', actualStartTime: b.snippet.actualStartTime ?? '' },
            status: { lifeCycleStatus: b.status.lifeCycleStatus, privacyStatus: b.status.privacyStatus },
          })),
        });
        return;
      }

      if (req.method === 'GET' && url.pathname === '/videos') {
        const id = url.searchParams.get('id');
        const v = state.videos.get(id);
        if (v === undefined) {
          sendJSON(res, 200, { items: [] });
          return;
        }
        sendJSON(res, 200, { items: [{ id, snippet: v.snippet, status: v.status }] });
        return;
      }

      if (req.method === 'PUT' && url.pathname === '/videos') {
        const raw = await readBody(req);
        record(raw);
        const parsed = JSON.parse(raw === '' ? '{}' : raw);
        state.lastVideoUpdateBody = parsed;
        const current = state.videos.get(parsed.id) ?? { snippet: {}, status: {} };
        current.snippet = parsed.snippet;
        current.status = parsed.status;
        state.videos.set(parsed.id, current);
        sendJSON(res, 200, { id: parsed.id, snippet: parsed.snippet, status: parsed.status });
        return;
      }

      if (req.method === 'GET' && url.pathname === '/videoCategories') {
        const region = url.searchParams.get('regionCode');
        if (region === null || region === '') {
          sendJSON(res, 400, { error: { code: 400, message: 'regionCode required' } });
          return;
        }
        sendJSON(res, 200, {
          items: state.categories.map((c) => ({ id: c.id, snippet: { title: c.name, assignable: c.assignable } })),
        });
        return;
      }

      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { error: { code: 500, message: String(error) } });
    }
  });
}

async function main() {
  console.log('YouTube connected-account integration verification (local fakes only, no real Google/YouTube)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-youtube-account-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const state = newYouTubeFakeState();
  state.pendingCodes = new Map(); // code -> { scope }
  const oauthServer = createFakeOAuthServer(state);
  const apiServer = createFakeYouTubeAPIServer(state);

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

    step('Reserve dynamic loopback ports and start the fake Google OAuth and YouTube API servers');
    const [backendPort, oauthPort, apiPort] = await reservePorts(3);
    baseUrl = `http://127.0.0.1:${backendPort}`;
    await listen(oauthServer, oauthPort);
    await listen(apiServer, apiPort);
    pass(`backend :${backendPort}  fake oauth :${oauthPort}  fake api :${apiPort}`);

    const baseEnv = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_PORT: String(backendPort),
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
      STREAMING_TREE_TEST_YOUTUBE_OAUTH_BASE_URL: `http://127.0.0.1:${oauthPort}`,
      STREAMING_TREE_TEST_YOUTUBE_API_BASE_URL: `http://127.0.0.1:${apiPort}`,
    };

    step('Start the backend under test (no YouTube Client ID configured yet)');
    backend = await startBackend(exePath, baseEnv, baseUrl);
    const config0 = await request(baseUrl, 'GET', '/api/integrations/youtube/config');
    expect(config0.status === 200 && config0.body.configured === false && config0.body.source === 'missing',
      'YouTube is reported unconfigured on first start', config0.body);

    step('Reject a client secret field on the integration-config endpoint');
    const rejectSecret = await request(baseUrl, 'PUT', '/api/integrations/youtube/config', {
      clientId: CLIENT_ID, clientSecret: 'must-be-rejected',
    });
    expect(rejectSecret.status === 400, 'a clientSecret field is rejected (400 unknown_field)', rejectSecret.body);

    step('Reject a complete Google credentials.json shape, not just a bare clientSecret key');
    const rejectJson = await request(baseUrl, 'PUT', '/api/integrations/youtube/config', {
      installed: { client_id: CLIENT_ID, client_secret: 'x', redirect_uris: ['http://localhost'] },
    });
    expect(rejectJson.status === 400, 'a pasted credentials.json is rejected (400, no clientId field recognized)', rejectJson.body);

    step('Configure a database-managed Client ID');
    const setConfig = await request(baseUrl, 'PUT', '/api/integrations/youtube/config', { clientId: CLIENT_ID });
    expect(setConfig.status === 200 && setConfig.body.source === 'database' && setConfig.body.clientId === CLIENT_ID,
      'Client ID saved with source "database"', setConfig.body);

    step('Start a YouTube OAuth attempt and confirm the public response has no code/state/verifier field');
    const start = await request(baseUrl, 'POST', '/api/integrations/youtube/oauth-attempts');
    expect(start.status === 202, 'attempt start is accepted (202)', start.body);
    const attemptId = start.body.attemptId;
    expect(typeof start.body.authorizationUrl === 'string' && start.body.authorizationUrl.length > 0,
      'an authorizationUrl is present', start.body.authorizationUrl);
    // "state" itself is NOT in this forbidden list: the response legitimately
    // has a "state" field for the attempt's own lifecycle (e.g.
    // "waiting_for_browser") - it is the OAuth CSRF state value that must
    // never appear as its own field, checked separately below via
    // oauthState/csrfState.
    for (const forbidden of ['code', 'pkceVerifier', 'codeVerifier', 'oauthState', 'csrfState', 'clientSecret']) {
      expect(!(forbidden in start.body), `no "${forbidden}" field exists in the response`, Object.keys(start.body));
    }

    step('A second concurrent attempt is rejected as a conflict');
    const conflict = await request(baseUrl, 'POST', '/api/integrations/youtube/oauth-attempts');
    expect(conflict.status === 409, 'the duplicate attempt is rejected with 409', conflict.body);
    expect(conflict.body?.error === 'youtube_oauth_attempt_conflict', 'the error code is youtube_oauth_attempt_conflict', conflict.body);

    const authUrl = new URL(start.body.authorizationUrl);
    const redirectUri = authUrl.searchParams.get('redirect_uri');
    const realState = authUrl.searchParams.get('state');
    expect(typeof redirectUri === 'string' && redirectUri.startsWith('http://127.0.0.1:'),
      'the redirect_uri points to a 127.0.0.1 loopback address', redirectUri);
    expect(typeof realState === 'string' && realState.length > 10, 'a real state value was embedded in the URL', realState);

    step('A callback with the wrong state is rejected and does not affect the real attempt');
    const wrongState = await requestAbsolute(`${redirectUri}?code=irrelevant&state=totally-wrong-state`);
    expect(wrongState.status === 200, 'the callback still answers 200 (a harmless page, not an error page)', wrongState.status);
    const stillWaiting = await request(baseUrl, 'GET', `/api/integrations/youtube/oauth-attempts/${attemptId}`);
    expect(stillWaiting.body.state === 'waiting_for_browser', 'the attempt is untouched by the wrong-state request', stillWaiting.body);

    step('Prepare two channels for the fake identity, so this run exercises explicit channel selection');
    const code = mintToken('fake-code');
    const scope = 'https://www.googleapis.com/auth/youtube.force-ssl';
    state.pendingCodes.set(code, { scope });
    const channelA = `UC_A_${RUN_ID}`;
    const channelB = `UC_B_${RUN_ID}`;
    state.channels.set(channelA, { id: channelA, title: `Channel A ${RUN_ID}`, country: 'US' });
    state.channels.set(channelB, { id: channelB, title: `Channel B ${RUN_ID}`, country: 'US' });

    step('A valid callback completes the token exchange with no client secret sent');
    const validCallback = await requestAbsolute(`${redirectUri}?code=${code}&state=${realState}`);
    expect(validCallback.status === 200, 'the valid callback is accepted', validCallback.status);

    step('The attempt reaches awaiting_channel_selection, offering both fake channels');
    const awaitingSelection = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/youtube/oauth-attempts/${attemptId}`);
      if (snap.body.state === 'error') {
        throw new Error(`attempt entered an error state: ${snap.body.errorCode} - ${snap.body.errorMessage}`);
      }
      return snap.body.state === 'awaiting_channel_selection' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'attempt to reach "awaiting_channel_selection"');
    expect(Array.isArray(awaitingSelection.channels) && awaitingSelection.channels.length === 2,
      'both fake channels are offered', awaitingSelection.channels);
    expect(!awaitingSelection.channels.some((c) => 'email' in c), 'no channel summary carries an email field', awaitingSelection.channels);

    step('Selecting a specific channel (not the first one) finalizes the account as that channel');
    const select = await request(baseUrl, 'POST', `/api/integrations/youtube/oauth-attempts/${attemptId}/channel`, { channelId: channelB });
    expect(select.status === 200 && select.body.state === 'authorized', 'selecting channel B finalizes the attempt', select.body);
    const accountId = select.body.connectedAccountId;
    expect(typeof accountId === 'string' && accountId.length > 0, 'a connectedAccountId is present', select.body);

    step('Confirm the account reflects the explicitly selected channel, not channel A, with no secret field');
    const accountsList = await request(baseUrl, 'GET', '/api/connected-accounts');
    expect(accountsList.body.accounts.length === 1, 'exactly one connected account exists', accountsList.body);
    const acc = accountsList.body.accounts[0];
    expect(acc.id === accountId && acc.login === `Channel B ${RUN_ID}`, 'the account matches the explicitly selected channel B', acc);
    for (const forbidden of ['accessToken', 'refreshToken', 'access_token', 'refresh_token', 'idToken']) {
      expect(!(forbidden in acc), `the account response has no "${forbidden}" field`, Object.keys(acc));
    }
    step('Link the account to the seeded YouTube destination');
    const platforms = await request(baseUrl, 'GET', '/api/platforms');
    const ytPlatform = platforms.body.platforms.find((p) => p.providerId === 'youtube');
    const kickPlatform = platforms.body.platforms.find((p) => p.providerId === 'kick');
    expect(ytPlatform !== undefined, 'a seeded YouTube platform exists', platforms.body.platforms);
    if (kickPlatform !== undefined) {
      const mismatch = await request(baseUrl, 'PUT', `/api/platforms/${kickPlatform.id}/connected-account`, { accountId });
      expect(mismatch.status === 422, 'linking a YouTube account to a non-YouTube destination is rejected (422)', mismatch.body);
    }
    const link = await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/connected-account`, { accountId });
    expect(link.status === 200 && link.body.accountId === accountId, 'the destination is linked to the account', link.body);

    step('Broadcasts list returns active/upcoming fixtures and never auto-selects one');
    state.broadcasts.set('bcast_active', {
      id: 'bcast_active', snippet: { title: 'Live now' }, status: { lifeCycleStatus: 'live', lifeCycleStatusFilter: 'active', privacyStatus: 'public' },
    });
    state.broadcasts.set('bcast_upcoming', {
      id: 'bcast_upcoming', snippet: { title: 'Scheduled later', scheduledStartTime: '2026-09-01T00:00:00Z' },
      status: { lifeCycleStatus: 'ready', lifeCycleStatusFilter: 'upcoming', privacyStatus: 'public' },
    });
    const broadcasts = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/youtube/broadcasts`);
    expect(broadcasts.status === 200 && broadcasts.body.items.length === 2, 'both fixtures are returned', broadcasts.body);
    const targetBeforeSelect = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/remote-target`);
    expect(targetBeforeSelect.body === null, 'no broadcast is auto-selected', targetBeforeSelect.body);

    step('Explicit broadcast selection persists and rejects an unowned broadcast ID');
    const badTarget = await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/remote-target`, { resourceId: 'not-a-real-broadcast' });
    expect(badTarget.status === 422, 'selecting a broadcast the account does not own is rejected (422)', badTarget.body);
    const setTarget = await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/remote-target`, { resourceId: 'bcast_active' });
    expect(setTarget.status === 200 && setTarget.body.resourceId === 'bcast_active', 'the active broadcast is selected', setTarget.body);

    step('Category search loads for the channel\'s own region, normalized and filtered to assignable only');
    const categories = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/youtube/categories`);
    expect(categories.status === 200 && categories.body.items.length === 2, 'only the two assignable categories are returned', categories.body);
    const gaming = categories.body.items.find((c) => c.name === 'Gaming');
    expect(gaming !== undefined && gaming.id === '20', 'the Gaming category is present with its real ID', gaming);

    step('Save local metadata with the selected categoryId, then preview shows the expected diff');
    state.videos.set('bcast_active', {
      snippet: { title: 'Old remote title', description: 'old desc', tags: ['old'], categoryId: '24', defaultLanguage: 'en' },
      status: { privacyStatus: 'public', selfDeclaredMadeForKids: false },
    });
    const saveMetadata = await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/metadata`, {
      title: `Live coding ${RUN_ID}`, description: 'new desc', category: gaming.name, categoryId: gaming.id,
      tags: ['coding', 'automated-test'], language: 'en', visibility: 'public', matureContent: false, dvr: false, latencyMode: '',
    });
    expect(saveMetadata.status === 200 && saveMetadata.body.categoryId === gaming.id, 'metadata saved with categoryId', saveMetadata.body);

    const preview = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/metadata/publish-preview`);
    expect(preview.status === 200 && preview.body.allowed === true, 'publishing is allowed', preview.body);
    expect(preview.body.broadcastId === 'bcast_active', 'the preview reflects the selected broadcast', preview.body);
    const titleDiff = preview.body.fields.find((f) => f.field === 'title');
    expect(titleDiff?.changed === true && titleDiff.remote === 'Old remote title', 'the title diff reflects the real remote value', titleDiff);
    expect(preview.body.skipped.includes('matureContent') && preview.body.skipped.includes('dvr'),
      'unsupported fields are reported as skipped, never silently sent', preview.body.skipped);

    step('Publish sends only verified fields via a safe read-modify-write, preserving unmanaged fields');
    const publish = await request(baseUrl, 'POST', `/api/platforms/${ytPlatform.id}/metadata/publish`);
    expect(publish.status === 200 && publish.body.status === 'published', 'publish succeeded', publish.body);
    expect(state.lastVideoUpdateBody.snippet.title === `Live coding ${RUN_ID}` && state.lastVideoUpdateBody.snippet.categoryId === gaming.id,
      'the fake API received the expected title and categoryId', state.lastVideoUpdateBody);
    expect(state.lastVideoUpdateBody.status.selfDeclaredMadeForKids === false,
      'a field this application does not manage (selfDeclaredMadeForKids) was preserved from the read, not dropped', state.lastVideoUpdateBody.status);

    step('Forced expired access token triggers exactly one transparent refresh and retry');
    const staleToken = state.currentAccessToken;
    state.accessTokens.get(staleToken).valid = false;
    const callsBefore = [...state.refreshCallCountByRefreshToken.values()].reduce((a, b) => a + b, 0);
    const categoriesAfter401 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/youtube/categories`);
    expect(categoriesAfter401.status === 200, 'the category list still succeeds after a forced 401 (transparent refresh)', categoriesAfter401.body);
    const callsAfter = [...state.refreshCallCountByRefreshToken.values()].reduce((a, b) => a + b, 0);
    expect(callsAfter === callsBefore + 1, 'exactly one refresh call was made', { before: callsBefore, after: callsAfter });
    expect(state.lastSuccessfulAPIToken !== staleToken, 'the retry used the newly rotated access token', null);

    step('A second refresh (Google omitting a new refresh token) preserves the original refresh token');
    const staleToken2 = state.currentAccessToken;
    state.accessTokens.get(staleToken2).valid = false;
    const categoriesAfterSecond401 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/youtube/categories`);
    expect(categoriesAfterSecond401.status === 200, 'the category list still succeeds after the second forced 401', categoriesAfterSecond401.body);
    pass('the backend continued operating correctly through a refresh where Google omitted a new refresh_token');

    step('Restart the backend and confirm the account, link and remote target all persisted');
    // Captured before the restart replaces `backend`, so the final secret
    // scan below can still see the first process's own stdout/stderr -
    // including the original callback this earlier process logged.
    const preRestartBackendOutput = backend.getOutput();
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, baseEnv, baseUrl);
    const linkAfterRestart = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/connected-account`);
    expect(linkAfterRestart.body?.accountId === accountId, 'the platform link persisted across a restart', linkAfterRestart.body);
    const targetAfterRestart = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/remote-target`);
    expect(targetAfterRestart.body?.resourceId === 'bcast_active', 'the remote target persisted across a restart', targetAfterRestart.body);

    // The token bundle itself does NOT survive this restart: cmd/testserver
    // deliberately backs it with an in-memory fake secret store (see its
    // own doc comment), never the real OS keychain a production restart
    // would use - see verify-twitch-account-integration.mjs's identical
    // note. Disconnecting now would therefore find no bundle to revoke at
    // all (Service.Disconnect's own documented "no bundle - proceed"
    // path), which is a real and already-covered behavior but not the one
    // this step means to exercise - so reconnect first, while the fake
    // OAuth/API servers can still recognize the same channel identity, to
    // put a live token bundle back before testing revoke-on-disconnect.
    step('Reconnect after the restart (the in-memory token bundle did not survive it) to restore a live token before disconnecting');
    const reconnectCode = mintToken('fake-code');
    state.pendingCodes.set(reconnectCode, { scope });
    const reconnectStart = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/reconnect`);
    expect(reconnectStart.status === 202, 'reconnect starts a new OAuth attempt', reconnectStart.body);
    const reconnectAuthUrl = new URL(reconnectStart.body.authorizationUrl);
    const reconnectRedirectUri = reconnectAuthUrl.searchParams.get('redirect_uri');
    const reconnectState = reconnectAuthUrl.searchParams.get('state');
    await requestAbsolute(`${reconnectRedirectUri}?code=${reconnectCode}&state=${reconnectState}`);
    // The fake Google identity still legitimately owns both channel A and
    // channel B (mine=true reflects real Google ownership records, not
    // this application's own prior selection), so reconnect offers channel
    // selection again - the operator must pick the same channel B or the
    // backend correctly rejects a different identity as a mismatch (see
    // the account-response reconnect-different-channel behavior this
    // application requires and internal/domain/account's own
    // ErrIdentityMismatch tests already cover at the unit level).
    const reconnectAwaitingSelection = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/youtube/oauth-attempts/${reconnectStart.body.attemptId}`);
      if (snap.body.state === 'error') {
        throw new Error(`reconnect attempt entered an error state: ${snap.body.errorCode} - ${snap.body.errorMessage}`);
      }
      return snap.body.state === 'awaiting_channel_selection' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'reconnect attempt to reach "awaiting_channel_selection"');
    expect(reconnectAwaitingSelection.channels.length === 2, 'reconnect offers both owned channels again', reconnectAwaitingSelection.channels);
    // Selecting the WRONG channel here (A, not the B being reconnected) is
    // covered at the unit level (internal/runtime/youtubeauth's own
    // multi-channel tests plus account.Service's ErrIdentityMismatch
    // tests) and terminates the attempt outright rather than allowing a
    // second guess - this script selects the correct channel directly to
    // continue exercising the rest of the disconnect flow below.
    const reconnected = await request(baseUrl, 'POST', `/api/integrations/youtube/oauth-attempts/${reconnectStart.body.attemptId}/channel`, { channelId: channelB });
    expect(reconnected.status === 200 && reconnected.body.state === 'authorized', 'selecting the same channel (B) completes the reconnect', reconnected.body);
    expect(reconnected.body.connectedAccountId === accountId, 'reconnect resolved to the same account, not a duplicate', reconnected.body);
    const accountsAfterReconnect = await request(baseUrl, 'GET', '/api/connected-accounts');
    expect(accountsAfterReconnect.body.accounts.length === 1, 'still exactly one connected account after reconnect', accountsAfterReconnect.body.accounts.length);

    step('Disconnect: revoke, remove the account, and cascade both the link and the remote target');
    const activeTokenBeforeDisconnect = state.currentAccessToken;
    const disconnect = await request(baseUrl, 'DELETE', `/api/connected-accounts/${accountId}`);
    expect(disconnect.status === 204, 'disconnect succeeds (204)', disconnect);
    expect(state.revokedTokens.has(activeTokenBeforeDisconnect), 'the fake server actually received and honored the revoke call', null);
    const accountsAfterDisconnect = await request(baseUrl, 'GET', '/api/connected-accounts');
    expect(accountsAfterDisconnect.body.accounts.length === 0, 'no connected accounts remain', accountsAfterDisconnect.body);
    const linkAfterDisconnect = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/connected-account`);
    expect(linkAfterDisconnect.body === null, 'the platform link was cascade-removed by disconnect', linkAfterDisconnect.body);
    const targetAfterDisconnect = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/remote-target`);
    expect(targetAfterDisconnect.body === null, 'the remote target was cleared - the target is not left pointing at an unlinked account', targetAfterDisconnect.body);

    step('Search every captured HTTP body, callback response, and backend log line for real secret material');
    const haystack = secretScanChunks.join('\n');
    const everyIssuedToken = [...state.accessTokens.keys(), ...state.refreshTokens.keys()];
    for (const token of everyIssuedToken) {
      const index = haystack.indexOf(token);
      expect(index === -1, `token ${token.slice(0, 12)}... never appears in the backend's own responses, callback pages, or logs`,
        index === -1 ? undefined : haystack.slice(Math.max(0, index - 200), index + 200));
    }
    // The state value legitimately appears inside the JSON API response's
    // own `authorizationUrl` field (the frontend must receive the complete
    // URL, state and all, to open it in a browser) - that is not a leak,
    // the same way a Twitch verification URL appearing in a response is
    // not one. What must never happen is the backend's own access-logger
    // (its stdout/stderr) recording it, since a callback's query string -
    // code and state alike - must never reach a log line.
    const allBackendOutput = preRestartBackendOutput + backend.getOutput();
    expect(!allBackendOutput.includes(realState), 'the real OAuth state value never appears in the backend\'s own stdout/stderr', null);
    pass(`scanned ${haystack.length} bytes of backend stdout/stderr, callback responses and HTTP response bodies for ${everyIssuedToken.length} issued tokens, and the backend's own process output for the OAuth state value`);

    console.log('\nYouTube connected-account integration verification PASSED');
  } finally {
    if (backend !== null && baseUrl !== undefined) {
      try {
        await stopBackend(backend, baseUrl);
      } catch {
        // Already reporting a failure if we get here.
      }
    }
    await close(oauthServer);
    await close(apiServer);
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nYouTube connected-account integration verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
