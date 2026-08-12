#!/usr/bin/env node
/**
 * Stage 15A: local, no-real-Google, no-real-OBS verification of the
 * YouTube Live Chat engagement connector, its integration through the
 * existing shared operator chat / Engagement Event Bus / alerts
 * pipelines, and the YouTube outbound chat adapter.
 *
 * This script never contacts real Google or YouTube. It runs the real
 * backend under test (`go build -tags integration ./cmd/testserver` -
 * the same binary every other verify-*.mjs script in this directory
 * uses) against one small in-process fake HTTP server reproducing only
 * the Google OAuth and YouTube Data/Live-Chat API response shapes this
 * application actually parses:
 *
 *   fake OAuth server   (oauth2.googleapis.com equivalent)
 *     /token, /tokeninfo, /revoke
 *   fake YouTube server (www.googleapis.com/youtube/v3 equivalent)
 *     /channels, /liveBroadcasts, /liveChat/messages (GET+POST)
 *
 * pointed at via STREAMING_TREE_TEST_YOUTUBE_OAUTH_BASE_URL and
 * STREAMING_TREE_TEST_YOUTUBE_API_BASE_URL - env vars that exist only
 * in the `-tags integration` binary, never in a production build.
 *
 * Account linking reuses the exact same simulated-browser PKCE dance
 * scripts/verify-youtube-account-integration.mjs already established
 * (there is no other seam to inject a connected account) - see that
 * script's own doc comment for why the loopback-callback fetch is a
 * real HTTP call into the backend under test, not a fake.
 *
 * What this script proves, end to end, against the real backend:
 *   - the connector reaches "connected" only once a destination is
 *     linked, a broadcast with a live chat is selected, and engagement
 *     is enabled - the same real HTTP routes the frontend uses;
 *   - the first ("baseline") poll's own messages are NEVER published,
 *     even when the fake server's first response already contains
 *     history - proven by seeding history before the very first poll
 *     and confirming none of it ever reaches the Engagement Event Bus;
 *   - a real chat message, Super Chat, and Super Sticker are each
 *     normalized correctly (integer-micros money, no floats) and reach
 *     BOTH the Engagement Event Bus AND the existing operator-chat
 *     projection - the same shared pipeline Twitch already uses, no
 *     parallel YouTube-only copy;
 *   - a monetary alert rule actually triggers off a real Super Chat,
 *     with the real amount/currency rendered into the alert text;
 *   - outbound sending works through the existing shared dispatcher,
 *     a reply attempt is rejected outright (YouTube's API has no reply
 *     concept), and the connected account's own echoed message never
 *     triggers a chat-automation command (self-loop protection, keyed
 *     on the stable channel id, never a display-name comparison);
 *   - an explicit connector restart re-baselines and never replays a
 *     message already delivered before the restart;
 *   - a real backend process restart preserves the destination link
 *     and selected broadcast (SQLite-backed), exactly like every other
 *     verify-*.mjs script's own persistence proof.
 *
 * Deliberately out of scope for this script (covered instead by the
 * Go unit/HTTP test suites already exercising them, per this project's
 * own "representative subset with documented deferral" convention):
 * membership/membership-milestone field-level assertions beyond "the
 * event type appears", the transient waiting_for_broadcast auto-
 * recovery timing (a real ~10s fixed retry interval - already proven
 * at the unit level in internal/runtime/youtubeengagement), and the
 * outbound "chat unavailable" (no broadcast selected) HTTP path -
 * already proven directly by
 * TestSendOutboundChatMessageChatUnavailable.
 *
 * Every token, channel ID, and Client ID used here is an obviously-fake
 * string generated for this run only. No real Google account, Google
 * Cloud project, or network request to Google/YouTube is ever involved,
 * and no real OBS Browser Source is ever opened - alert delivery is
 * verified via the same real HTTP/SSE API OBS itself would use.
 *
 * Usage: node scripts/verify-youtube-engagement.mjs
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

/** A bare HTTP GET against an arbitrary absolute URL - used only to call
 * the backend's own loopback OAuth callback listener directly,
 * simulating the one step a real browser would otherwise perform. */
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

function mintToken(prefix) {
  return `${prefix}-${RUN_ID}-${randomBytes(12).toString('hex')}`;
}

// --- fake Google OAuth + YouTube API server ---------------------------

function newYouTubeFakeState() {
  return {
    accessTokens: new Map(), // token -> { valid, channelId, scope }
    refreshTokens: new Map(),
    currentAccessToken: null,
    channels: new Map(), // channelId -> { id, title, country }
    broadcasts: new Map(), // broadcastId -> { id, snippet, status }
    pendingCodes: new Map(), // code -> { scope }
    revokedTokens: new Set(),
    // liveChat.queue: a FIFO of pages to serve NEXT, one per real poll
    // this script wants to inject content on. liveChat.steadyPage is
    // served whenever the queue is empty - the connector's own
    // baseline call always lands here unless a page was queued first,
    // exactly mirroring "nothing new" in a real quiet chat.
    liveChat: {
      queue: [],
      steadyPage: { items: [], nextPageToken: 'steady', offlineAt: '' },
    },
    insertCallCount: 0,
    lastInsertBody: null,
  };
}

function issueTokenPair(state, channelId, scope) {
  const accessToken = mintToken('fake-access');
  state.accessTokens.set(accessToken, { valid: true, channelId, scope });
  state.currentAccessToken = accessToken;
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
        const grantType = form.get('grant_type');

        if (grantType === 'authorization_code') {
          if (form.has('client_secret')) {
            sendJSON(res, 400, { error: 'invalid_request', error_description: 'client_secret not expected for this client' });
            return;
          }
          const code = form.get('code');
          const pending = state.pendingCodes.get(code);
          if (pending === undefined) {
            sendJSON(res, 400, { error: 'invalid_grant', error_description: 'unknown code' });
            return;
          }
          const { accessToken, refreshToken } = issueTokenPair(state, null, pending.scope);
          sendJSON(res, 200, { access_token: accessToken, refresh_token: refreshToken, scope: pending.scope, token_type: 'Bearer', expires_in: 3600 });
          return;
        }

        if (grantType === 'refresh_token') {
          const oldRefresh = form.get('refresh_token');
          const entry = state.refreshTokens.get(oldRefresh);
          if (entry === undefined || !entry.valid) {
            sendJSON(res, 400, { error: 'invalid_grant', error_description: 'Token has been expired or revoked.' });
            return;
          }
          const { accessToken, refreshToken } = issueTokenPair(state, entry.channelId, entry.scope);
          sendJSON(res, 200, { access_token: accessToken, refresh_token: refreshToken, scope: entry.scope, token_type: 'Bearer', expires_in: 3600 });
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

      if (req.method === 'GET' && url.pathname === '/channels') {
        let items;
        if (url.searchParams.get('mine') === 'true') {
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
            snippet: { title: b.snippet.title, liveChatId: b.snippet.liveChatId ?? '' },
            status: { lifeCycleStatus: b.status.lifeCycleStatus, privacyStatus: b.status.privacyStatus },
          })),
        });
        return;
      }

      if (req.method === 'GET' && url.pathname === '/liveChat/messages') {
        const page = state.liveChat.queue.length > 0 ? state.liveChat.queue.shift() : state.liveChat.steadyPage;
        sendJSON(res, 200, {
          nextPageToken: page.nextPageToken ?? `token-${randomUUID().slice(0, 6)}`,
          pollingIntervalMillis: 2000,
          offlineAt: page.offlineAt ?? '',
          items: page.items,
        });
        return;
      }

      if (req.method === 'POST' && url.pathname === '/liveChat/messages') {
        const raw = await readBody(req);
        record(raw);
        const parsed = JSON.parse(raw === '' ? '{}' : raw);
        state.insertCallCount += 1;
        state.lastInsertBody = parsed;
        sendJSON(res, 200, { id: mintToken('sent'), snippet: { type: parsed.snippet?.type ?? 'textMessageEvent' } });
        return;
      }

      res.writeHead(404, { Connection: 'close' });
      res.end();
    } catch (error) {
      sendJSON(res, 500, { error: { code: 500, message: String(error) } });
    }
  });
}

// --- liveChatMessages.list item fixtures --------------------------------
// Exact wire shapes mirroring apps/server/internal/provider/youtube/
// models.go's own liveChatMessageResource/snippet struct tags.

function textMessageItem({ id, authorChannelId, displayName, text }) {
  return {
    id,
    snippet: {
      type: 'textMessageEvent',
      publishedAt: new Date().toISOString(),
      authorChannelId,
      hasDisplayContent: true,
      displayMessage: text,
      textMessageDetails: { messageText: text },
    },
    authorDetails: {
      channelId: authorChannelId, channelUrl: `https://youtube.com/channel/${authorChannelId}`,
      displayName, profileImageUrl: 'https://fake.youtube.example/avatar.jpg',
      isVerified: false, isChatOwner: false, isChatSponsor: false, isChatModerator: false,
    },
  };
}

function superChatItem({ id, authorChannelId, displayName, amountMicros, currency, amountDisplayString, comment }) {
  return {
    id,
    snippet: {
      type: 'superChatEvent',
      publishedAt: new Date().toISOString(),
      authorChannelId,
      hasDisplayContent: true,
      displayMessage: comment ?? '',
      superChatDetails: { amountMicros, currency, amountDisplayString, userComment: comment ?? '', tier: 2 },
    },
    authorDetails: {
      channelId: authorChannelId, channelUrl: `https://youtube.com/channel/${authorChannelId}`,
      displayName, profileImageUrl: 'https://fake.youtube.example/avatar.jpg',
      isVerified: false, isChatOwner: false, isChatSponsor: false, isChatModerator: false,
    },
  };
}

function superStickerItem({ id, authorChannelId, displayName, amountMicros, currency, amountDisplayString }) {
  return {
    id,
    snippet: {
      type: 'superStickerEvent',
      publishedAt: new Date().toISOString(),
      authorChannelId,
      hasDisplayContent: true,
      displayMessage: '',
      superStickerDetails: {
        amountMicros, currency, amountDisplayString, tier: 1,
        superStickerMetadata: { stickerId: 'sticker_1', altText: 'excited cat', language: '' },
      },
    },
    authorDetails: {
      channelId: authorChannelId, channelUrl: `https://youtube.com/channel/${authorChannelId}`,
      displayName, profileImageUrl: 'https://fake.youtube.example/avatar.jpg',
      isVerified: false, isChatOwner: false, isChatSponsor: false, isChatModerator: false,
    },
  };
}

async function findEventOfType(baseUrl, type, timeoutMs = 15_000) {
  return waitUntil(async () => {
    const events = await request(baseUrl, 'GET', '/api/engagement/events?limit=100');
    const match = events.body.items.find((item) => item.type === type);
    return match ?? false;
  }, timeoutMs, `an event of type "${type}" to appear`);
}

/** Waits for the specific event carrying providerEventId - never
 * satisfied by an earlier, different event of the same type already on
 * the bus (findEventOfType alone would be, since it returns the first
 * match). Used whenever a step needs to confirm THIS particular
 * message was actually normalized/published before asserting on its
 * downstream effects (or lack thereof). */
async function findEventByProviderEventId(baseUrl, providerEventId, timeoutMs = 15_000) {
  return waitUntil(async () => {
    const events = await request(baseUrl, 'GET', '/api/engagement/events?limit=200');
    const match = events.body.items.find((item) => item.providerEventId === providerEventId);
    return match ?? false;
  }, timeoutMs, `the event with providerEventId "${providerEventId}" to appear`);
}

async function findOperatorChatItem(baseUrl, predicate, timeoutMs = 15_000) {
  return waitUntil(async () => {
    const items = await request(baseUrl, 'GET', '/api/operator-chat/items?limit=200');
    const match = (items.body.items ?? []).find(predicate);
    return match ?? false;
  }, timeoutMs, 'a matching operator-chat item to appear');
}

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

function findAlert(status, substring) {
  const items = [status.current, ...(status.nextQueued ?? [])].filter(Boolean);
  return items.find((a) => a.renderedText.includes(substring));
}

async function main() {
  console.log('Stage 15A YouTube engagement (Live Chat connector + operator chat + outbound chat + alerts) verification (local fakes only, no real Google/YouTube, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-youtube-engagement-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');
  const state = newYouTubeFakeState();
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

    const env = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_PORT: String(backendPort),
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
      STREAMING_TREE_TEST_YOUTUBE_OAUTH_BASE_URL: `http://127.0.0.1:${oauthPort}`,
      STREAMING_TREE_TEST_YOUTUBE_API_BASE_URL: `http://127.0.0.1:${apiPort}`,
    };

    step('Start the backend under test with no connectors enabled');
    backend = await startBackend(exePath, env, baseUrl);
    const status0 = await request(baseUrl, 'GET', '/api/engagement/status');
    expect(status0.status === 200 && status0.body.connectors.length === 0, 'the Event Bus starts empty with no connectors', status0.body);

    step('Configure the YouTube Client ID and link a connected account via the real (simulated-browser) PKCE flow');
    const configResp = await request(baseUrl, 'PUT', '/api/integrations/youtube/config', { clientId: CLIENT_ID });
    expect(configResp.status === 200, 'the Client ID was configured', configResp.body);
    const start = await request(baseUrl, 'POST', '/api/integrations/youtube/oauth-attempts');
    expect(start.status === 202, 'the OAuth attempt starts', start.body);
    const authUrl = new URL(start.body.authorizationUrl);
    const redirectUri = authUrl.searchParams.get('redirect_uri');
    const realState = authUrl.searchParams.get('state');

    const code = mintToken('fake-code');
    const scope = 'https://www.googleapis.com/auth/youtube.force-ssl';
    state.pendingCodes.set(code, { scope });
    const channelId = `UC_${RUN_ID}`;
    state.channels.set(channelId, { id: channelId, title: `Channel ${RUN_ID}`, country: 'US' });

    const callback = await requestAbsolute(`${redirectUri}?code=${code}&state=${realState}`);
    expect(callback.status === 200, 'the OAuth callback was accepted', callback.text);
    // With exactly one fake channel owned by this identity, the backend
    // auto-selects it rather than pausing for explicit channel selection
    // (that step - already covered by
    // verify-youtube-account-integration.mjs - only applies when more
    // than one channel is offered) - the attempt goes straight to
    // "authorized".
    const authorized = await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/integrations/youtube/oauth-attempts/${start.body.attemptId}`);
      if (snap.body.state === 'error') throw new Error(`attempt error: ${snap.body.errorCode} - ${snap.body.errorMessage}`);
      return snap.body.state === 'authorized' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the attempt to reach "authorized"');
    const accountId = authorized.connectedAccountId;
    expect(typeof accountId === 'string' && accountId.length > 0, 'a connectedAccountId is present', authorized);

    step('Link the account to the seeded YouTube destination and select a live broadcast with a real liveChatId');
    const platforms = await request(baseUrl, 'GET', '/api/platforms');
    const ytPlatform = platforms.body.platforms.find((p) => p.providerId === 'youtube');
    expect(ytPlatform !== undefined, 'a seeded YouTube platform exists', platforms.body.platforms);
    const link = await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/connected-account`, { accountId });
    expect(link.status === 200, 'the destination is linked to the account', link.body);

    const liveChatId = `chat_${RUN_ID}`;
    state.broadcasts.set('bcast_1', {
      id: 'bcast_1', snippet: { title: 'Live now', liveChatId }, status: { lifeCycleStatus: 'live', lifeCycleStatusFilter: 'active', privacyStatus: 'public' },
    });
    const setTarget = await request(baseUrl, 'PUT', `/api/platforms/${ytPlatform.id}/remote-target`, { resourceId: 'bcast_1' });
    expect(setTarget.status === 200 && setTarget.body.resourceId === 'bcast_1', 'the live broadcast is selected as the remote target', setTarget.body);

    step('Confirm the engagement endpoint reports YouTube (never requiring a permission upgrade) before it is enabled');
    const engagement0 = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(engagement0.status === 200 && engagement0.body.state === 'disabled', 'the connector state is "disabled" before it is ever enabled', engagement0.body);
    expect(engagement0.body.permissionUpgradeRequired === false, 'YouTube never requires a permission upgrade (single scope covers everything)', engagement0.body);

    step('Seed pre-existing "history" in the fake liveChat response, before ever enabling engagement (the baseline-safety scenario)');
    const historyId = `hist_${RUN_ID}`;
    state.liveChat.queue.push({
      items: [textMessageItem({ id: historyId, authorChannelId: `UC_history_${RUN_ID}`, displayName: 'History Viewer', text: 'this is old history' })],
      nextPageToken: 'after-history',
    });

    step('Enable engagement: the connector polls the fake liveChat endpoint and reaches "connected"');
    const enableResp = await request(baseUrl, 'PUT', `/api/connected-accounts/${accountId}/engagement`, { enabled: true });
    expect(enableResp.status === 200 && enableResp.body.enabled === true, 'the enable request succeeds', enableResp.body);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      if (snap.body.state === 'error') throw new Error(`connector entered error state: ${snap.body.lastError}`);
      return snap.body.state === 'connected' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to reach "connected"');
    expect(state.liveChat.queue.length === 0, 'the seeded "history" page was consumed by the baseline call', state.liveChat.queue.length);

    step('Confirm the seeded history NEVER appears as a real engagement event (baseline-first cutover)');
    await new Promise((r) => setTimeout(r, 3_000)); // a couple of real poll cycles
    const eventsAfterBaseline = await request(baseUrl, 'GET', '/api/engagement/events?limit=100');
    const historyLeaked = eventsAfterBaseline.body.items.some((e) => e.providerEventId === historyId);
    expect(!historyLeaked, 'the seeded "history" message never became a real event - baseline never publishes', eventsAfterBaseline.body.items);

    step('Create an alert profile and a currency-aware Super Chat rule before the real Super Chat arrives, so it triggers naturally');
    const profile = await request(baseUrl, 'POST', '/api/alert-profiles', { name: `YouTube ${RUN_ID}` });
    expect(profile.status === 201, 'the alert profile was created', profile.body);
    const profileId = profile.body.id;
    await request(baseUrl, 'POST', `/api/alert-profiles/${profileId}/queue/pause`);
    const rule = await request(baseUrl, 'POST', `/api/alert-profiles/${profileId}/rules`, {
      name: 'Super Chat', enabled: true, eventType: 'youtube_super_chat', priority: 50, durationMs: 1000,
      currency: 'USD', minimumAmountMicros: 1_000_000, maximumAmountMicros: null,
      requiredRole: 'everyone', showPlatform: true, showUsername: true, showMessage: true, showAmount: true,
      textTemplate: '{username} sent a Super Chat: {amount} {currency} - {message}',
      entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400, providers: [], accounts: [],
    });
    expect(rule.status === 201, 'the Super Chat monetary rule was created', rule.body);
    const ruleBelowThreshold = await request(baseUrl, 'POST', `/api/alert-profiles/${profileId}/rules`, {
      name: 'Bad quantity on a money-only type', enabled: true, eventType: 'youtube_super_chat', priority: 50, durationMs: 1000,
      minimumQuantity: 5,
      requiredRole: 'everyone', showPlatform: true, showUsername: true, showMessage: false, showQuantity: true,
      textTemplate: '{username}', entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400, providers: [], accounts: [],
    });
    expect(ruleBelowThreshold.status === 422, 'a quantity condition on a money-only event type is rejected (422)', ruleBelowThreshold.body);

    step('Send a real chat message and confirm it reaches BOTH the Engagement Event Bus and the existing operator-chat projection');
    const chatterChannelId = `UC_chatter_${RUN_ID}`;
    state.liveChat.queue.push({
      items: [textMessageItem({ id: `msg_${RUN_ID}`, authorChannelId: chatterChannelId, displayName: 'A Chatter', text: 'hello from YouTube' })],
      nextPageToken: 'after-chat',
    });
    const chatEvent = await findEventOfType(baseUrl, 'chat.message');
    expect(chatEvent.message?.text === 'hello from YouTube', 'the chat message text was normalized correctly', chatEvent.message);
    expect(chatEvent.providerId === 'youtube', 'the event is attributed to the youtube provider', chatEvent);
    const operatorItem = await findOperatorChatItem(baseUrl, (item) => item.kind === 'message' && item.message?.plainText === 'hello from YouTube');
    expect(operatorItem.providerId === 'youtube', 'the same message reached the existing shared operator-chat projection', operatorItem);

    step('Send a real Super Chat: confirm integer-micros money on the Event Bus, on operator chat, AND that it triggers the real monetary alert');
    const superChatterChannelId = `UC_superchat_${RUN_ID}`;
    state.liveChat.queue.push({
      items: [superChatItem({
        id: `sc_1_${RUN_ID}`, authorChannelId: superChatterChannelId, displayName: 'Big Fan',
        amountMicros: 5_000_000, currency: 'usd', amountDisplayString: '$5.00', comment: 'great stream!',
      })],
      nextPageToken: 'after-superchat-1',
    });
    const superChatEvent = await findEventOfType(baseUrl, 'youtube.super_chat');
    expect(superChatEvent.amountMicros === 5_000_000, 'the Super Chat amount is integer micros, never a float', superChatEvent);
    expect(superChatEvent.currency === 'USD', 'the currency was uppercased by the normalizer', superChatEvent);
    expect(superChatEvent.displayAmount === '$5.00', 'the provider-rendered display amount is preserved', superChatEvent);
    const superChatActivity = await findOperatorChatItem(baseUrl, (item) => item.kind === 'activity' && item.activity?.activityType === 'youtube.super_chat');
    expect(superChatActivity.activity.amountMicros === 5_000_000 && superChatActivity.activity.currency === 'USD',
      'the Super Chat also carries its real money fields in the shared operator-chat projection', superChatActivity.activity);

    const alertTriggered = await waitForQueue(baseUrl, profileId,
      (st) => findAlert(st, '$5.00 USD') !== undefined, POLL_TIMEOUT_MS, 'the Super Chat alert to be queued');
    const superChatAlert = findAlert(alertTriggered, '$5.00 USD');
    expect(superChatAlert.renderedText.includes('great stream!'), 'the alert text includes the real comment', superChatAlert.renderedText);
    expect(superChatAlert.synthetic === false, 'the triggered alert is a real one, never synthetic', superChatAlert);

    step('A Super Chat below the rule\'s minimum amount threshold does not trigger a second alert');
    const totalBefore = alertTriggered.totalEnqueued;
    state.liveChat.queue.push({
      items: [superChatItem({
        id: `sc_below_${RUN_ID}`, authorChannelId: superChatterChannelId, displayName: 'Big Fan',
        amountMicros: 500_000, currency: 'usd', amountDisplayString: '$0.50', comment: 'small one',
      })],
      nextPageToken: 'after-superchat-below',
    });
    await findEventByProviderEventId(baseUrl, `sc_below_${RUN_ID}`); // wait for THIS event specifically to land on the bus
    await new Promise((r) => setTimeout(r, 1_000));
    const statusAfterBelow = await queueStatus(baseUrl, profileId);
    expect(statusAfterBelow.totalEnqueued === totalBefore, 'a below-threshold Super Chat never queues an alert', {
      before: totalBefore, after: statusAfterBelow.totalEnqueued,
    });

    step('A different currency never matches the USD-only rule (no FX conversion, ever)');
    state.liveChat.queue.push({
      items: [superChatItem({
        id: `sc_eur_${RUN_ID}`, authorChannelId: superChatterChannelId, displayName: 'Big Fan',
        amountMicros: 100_000_000, currency: 'eur', amountDisplayString: '€100.00', comment: 'big but wrong currency',
      })],
      nextPageToken: 'after-superchat-eur',
    });
    await findEventByProviderEventId(baseUrl, `sc_eur_${RUN_ID}`);
    await new Promise((r) => setTimeout(r, 1_000));
    const statusAfterEUR = await queueStatus(baseUrl, profileId);
    expect(statusAfterEUR.totalEnqueued === totalBefore, 'a numerically-larger EUR Super Chat never matches a USD threshold rule', {
      before: totalBefore, after: statusAfterEUR.totalEnqueued,
    });

    step('Send a real Super Sticker (no comment field at all) and confirm it normalizes with money but no message');
    state.liveChat.queue.push({
      items: [superStickerItem({
        id: `sticker_1_${RUN_ID}`, authorChannelId: superChatterChannelId, displayName: 'Big Fan',
        amountMicros: 2_000_000, currency: 'usd', amountDisplayString: '$2.00',
      })],
      nextPageToken: 'after-sticker',
    });
    const stickerEvent = await findEventOfType(baseUrl, 'youtube.super_sticker');
    expect(stickerEvent.amountMicros === 2_000_000, 'the Super Sticker amount is integer micros', stickerEvent);
    expect(stickerEvent.message === undefined, 'a Super Sticker never fabricates a message/comment field', stickerEvent);

    step('Outbound send: a plain text message reaches the fake liveChat insert endpoint with the exact expected body, no reply field');
    const sendResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, { message: 'hello chat from the operator' });
    expect(sendResp.status === 200 && sendResp.body.sent === true, 'the send succeeds', sendResp.body);
    expect(!JSON.stringify(sendResp.body).includes('hello chat from the operator'), 'the response never echoes the sent text', sendResp.body);
    expect(state.lastInsertBody?.snippet?.liveChatId === liveChatId, 'the insert targeted the real selected broadcast\'s liveChatId', state.lastInsertBody);
    expect(state.lastInsertBody?.snippet?.type === 'textMessageEvent', 'the insert used a plain textMessageEvent', state.lastInsertBody);
    expect(state.lastInsertBody?.snippet?.textMessageDetails?.messageText === 'hello chat from the operator', 'the exact message text was sent', state.lastInsertBody);
    expect(!('replyParentMessageId' in (state.lastInsertBody?.snippet ?? {})), 'no reply field was ever sent - YouTube\'s API has no such concept', state.lastInsertBody);

    step('Outbound send: a reply attempt is rejected outright (422 outbound_chat_reply_unsupported), never silently downgraded');
    const replyResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/outbound-chat/messages`, {
      message: 'this should never send', replyParentMessageId: `msg_${RUN_ID}`,
    });
    expect(replyResp.status === 422, 'the reply attempt is rejected with 422', replyResp.body);
    expect(replyResp.body.error === 'outbound_chat_reply_unsupported', 'the stable error code is outbound_chat_reply_unsupported', replyResp.body);
    expect(state.insertCallCount === 1, 'the rejected reply attempt never reached the fake YouTube API at all', state.insertCallCount);

    step('Chat automation: a real viewer\'s message triggers a configured command (proving the mechanism actually works before testing self-loop protection)');
    const createCommand = await request(baseUrl, 'POST', '/api/chat-automation/commands', {
      name: 'echo', enabled: true, responseTemplate: 'pong', requiredRole: 'everyone',
      globalCooldownSeconds: 0, userCooldownSeconds: 0, aliases: [], targets: [{ accountId }],
    });
    expect(createCommand.status === 201, 'the test command was created', createCommand.body);
    const insertCallsBeforeViewer = state.insertCallCount;
    const viewerChannelId = `UC_viewer_${RUN_ID}`;
    state.liveChat.queue.push({
      items: [textMessageItem({ id: `viewer_${RUN_ID}`, authorChannelId: viewerChannelId, displayName: 'A Viewer', text: '!echo' })],
      nextPageToken: 'after-viewer-command',
    });
    await waitUntil(() => (state.insertCallCount > insertCallsBeforeViewer ? true : false), POLL_TIMEOUT_MS, 'the command response to be sent for a real viewer');
    pass('a real viewer\'s "!echo" triggered the command and sent a response through the real outbound adapter (never rejected as an unsupported reply)');

    step('Self-loop protection: the same command never fires for the connected account\'s own echoed message');
    const insertCallsBeforeSelf = state.insertCallCount;
    state.liveChat.queue.push({
      // authorChannelId equals the CONNECTED ACCOUNT's own channel id -
      // exactly the stable-provider-id comparison self-loop protection
      // must use, never a display-name comparison.
      items: [textMessageItem({ id: `self_${RUN_ID}`, authorChannelId: channelId, displayName: `Channel ${RUN_ID}`, text: '!echo' })],
      nextPageToken: 'after-self',
    });
    await findEventByProviderEventId(baseUrl, `self_${RUN_ID}`); // confirm the echoed message still reaches the bus...
    await new Promise((r) => setTimeout(r, 1_500));
    expect(state.insertCallCount === insertCallsBeforeSelf,
      'the account\'s own echoed "!echo" message never triggered the command (hard self-loop-protection rule, keyed on the stable channel id)',
      { before: insertCallsBeforeSelf, after: state.insertCallCount });

    step('An explicit connector restart re-baselines and never replays a message already delivered before the restart');
    const enqueuedBeforeRestart = (await queueStatus(baseUrl, profileId)).totalEnqueued;
    const restartResp = await request(baseUrl, 'POST', `/api/connected-accounts/${accountId}/engagement/restart`);
    expect(restartResp.status === 200, 'the connector restart request succeeds', restartResp.body);
    await waitUntil(async () => {
      const snap = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
      return snap.body.state === 'connected' ? snap.body : false;
    }, POLL_TIMEOUT_MS, 'the connector to reach "connected" again after the restart');
    // No new page queued yet - the restarted connector's own baseline call
    // (and its first steady-state poll) must consume the fake server's
    // steady (empty) page, never re-deliver sc_1/msg_1/sticker_1 as new.
    await new Promise((r) => setTimeout(r, 3_000));
    const eventsAfterRestart = await request(baseUrl, 'GET', '/api/engagement/events?limit=200');
    const superChatEventCount = eventsAfterRestart.body.items.filter((e) => e.type === 'youtube.super_chat' && e.providerEventId === `sc_1_${RUN_ID}`).length;
    expect(superChatEventCount === 1, 'the original Super Chat was never re-published after the restart (still exactly one)', superChatEventCount);
    const statusAfterConnectorRestart = await queueStatus(baseUrl, profileId);
    expect(statusAfterConnectorRestart.totalEnqueued === enqueuedBeforeRestart,
      'no new alert was queued purely from the connector restart itself', {
        before: enqueuedBeforeRestart, after: statusAfterConnectorRestart.totalEnqueued,
      });

    step('A genuinely new Super Chat sent after the restart still triggers a fresh alert (the connector still works, it just never replays)');
    state.liveChat.queue.push({
      items: [superChatItem({
        id: `sc_2_${RUN_ID}`, authorChannelId: superChatterChannelId, displayName: 'Big Fan',
        amountMicros: 10_000_000, currency: 'usd', amountDisplayString: '$10.00', comment: 'after the restart!',
      })],
      nextPageToken: 'after-superchat-2',
    });
    const alertAfterRestart = await waitForQueue(baseUrl, profileId,
      (st) => findAlert(st, '$10.00 USD') !== undefined, POLL_TIMEOUT_MS, 'a fresh post-restart Super Chat alert to be queued');
    expect(findAlert(alertAfterRestart, '$10.00 USD').renderedText.includes('after the restart!'),
      'the fresh post-restart alert renders correctly', findAlert(alertAfterRestart, '$10.00 USD'));

    step('Restart the whole backend process and confirm the destination link and selected broadcast both persisted');
    const preRestartBackendOutput = backend.getOutput();
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);
    const linkAfterRestart = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/connected-account`);
    expect(linkAfterRestart.body?.accountId === accountId, 'the platform link persisted across a full backend restart', linkAfterRestart.body);
    const targetAfterRestart = await request(baseUrl, 'GET', `/api/platforms/${ytPlatform.id}/remote-target`);
    expect(targetAfterRestart.body?.resourceId === 'bcast_1', 'the selected broadcast persisted across a full backend restart', targetAfterRestart.body);
    const engagementAfterRestart = await request(baseUrl, 'GET', `/api/connected-accounts/${accountId}/engagement`);
    expect(engagementAfterRestart.body.enabled === true, 'the engagement-enabled flag persisted across a full backend restart', engagementAfterRestart.body);

    step('Search every captured HTTP body, callback response, and backend log line for real secret material');
    const haystack = secretScanChunks.join('\n');
    const everyIssuedToken = [...state.accessTokens.keys(), ...state.refreshTokens.keys()];
    for (const token of everyIssuedToken) {
      const index = haystack.indexOf(token);
      expect(index === -1, `token ${token.slice(0, 12)}... never appears in the backend's own responses, callback pages, or logs`,
        index === -1 ? undefined : haystack.slice(Math.max(0, index - 200), index + 200));
    }
    const allBackendOutput = preRestartBackendOutput + backend.getOutput();
    expect(!allBackendOutput.includes(realState), 'the real OAuth state value never appears in the backend\'s own stdout/stderr', null);
    pass(`scanned ${haystack.length} bytes of backend stdout/stderr, callback responses, and HTTP response bodies for ${everyIssuedToken.length} issued tokens and the OAuth state value`);

    console.log('\nStage 15A YouTube engagement verification PASSED');
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
  console.error(`\nStage 15A YouTube engagement verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
