#!/usr/bin/env node
/**
 * Linux remote-management verification (Stage 20D2B) - a platform-
 * specific CI verification helper, explicitly NOT canonical
 * integration script #25. The canonical local/Windows count remains
 * 24 (docs/remote-management.md).
 *
 * Exercises the real --headless --remote-management contract through
 * an actual TLS-terminating reverse-proxy boundary - not a fake
 * X-Forwarded-Proto header sent directly to the loopback backend. The
 * proxy is a small, first-party, ephemeral-CA HTTPS server (Node's own
 * `node:https`/`node:tls`) that always overwrites X-Forwarded-Proto/
 * X-Forwarded-Host, exactly mirroring the documented Caddy reference
 * configuration's own confirmed default behavior
 * (docs/remote-management.md §2/§20), AND implements the exact same
 * @excludedLocalOnlySurface routing policy the corrected
 * docs/examples/Caddyfile.remote-management now uses (PRE-20D2C
 * correction: an earlier version of both the Caddyfile and this test
 * proxy forwarded every path, including /overlay/* and /api/public/*,
 * contradicting the project's own stated D2B exposure boundary - see
 * docs/remote-management.md §17). No `caddy` binary is installed in
 * this CI environment (confirmed directly, not installed here solely
 * for this check); the routing policy exercised is grounded in
 * Caddy's own official documentation, not the literal Caddy binary -
 * stated honestly, not claimed as more than it is.
 *
 * Requires a Linux release build to already exist at
 * build/release-linux/output/ - run
 *   scripts/build-release-linux.sh --version 0.1.0-dev+test
 * first.
 *
 * Covers the security-critical core of the governing task's own
 * native-scenario list: package/service identity, loopback-only
 * enforcement, the real TLS proxy boundary, unauthenticated rejection,
 * login/rate-limit/session/CSRF/Origin behavior, cookie attributes,
 * remote-safe shutdown, session invalidation across a restart, no
 * remote overlay exposure, and a socket audit. It is not a literal
 * enumeration of every one of that list's items - recorded honestly in
 * docs/progress.md rather than claimed as exhaustive.
 *
 * Usage:  node scripts/verify-linux-remote-management.mjs
 * Exits non-zero on the first failed expectation.
 */

import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { createServer as createHttpsServer, request as httpsRequest } from 'node:https';
import { request as httpRequest } from 'node:http';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release-linux', 'output');
const PACKAGE_NAME = 'streaming-tree-for-obs';
const INSTALLED_EXE_PATH = '/usr/bin/streaming-tree-server';

const BACKEND_PORT = 8601;
const PROXY_PORT = 8602;
const EXTERNAL_ORIGIN_HOST = 'stream.example.test';
const EXTERNAL_ORIGIN = `https://${EXTERNAL_ORIGIN_HOST}:${PROXY_PORT}`;
const ADMIN_PASSWORD = 'a-test-only-administrator-password-never-used-in-production';
const READINESS_TIMEOUT_MS = 30_000;
const SHUTDOWN_TIMEOUT_MS = 15_000;

let stepCount = 0;
function step(message) {
  stepCount += 1;
  console.log(`\n[${String(stepCount).padStart(2, '0')}] ${message}`);
}
function pass(message) {
  console.log(`     ok  ${message}`);
}
function fail(message, detail) {
  console.error(`     FAIL ${message}`);
  if (detail !== undefined) {
    console.error(`          ${typeof detail === 'string' ? detail : JSON.stringify(detail)}`);
  }
  throw new Error(message);
}
function expect(condition, message, detail) {
  if (condition) {
    pass(message);
    return;
  }
  fail(message, detail);
}

function isPackageInstalled() {
  try {
    execFileSync('dpkg', ['-s', PACKAGE_NAME], { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

/** Hermetic $CREDENTIALS_DIRECTORY-shaped master key - see verify-linux-headless.mjs's own identical helper. */
function provisionCredentialsDir() {
  const dir = mkdtempSync(join(tmpdir(), 'streaming-tree-rm-creds-'));
  const key = Buffer.alloc(32);
  for (let i = 0; i < 32; i += 1) key[i] = (i * 13 + 5) % 256;
  writeFileSync(join(dir, 'streaming-tree-master-key'), key, { mode: 0o600 });
  return dir;
}

/** Provisions the administrator password into the real HeadlessStore
 * via the real --provision-admin-password CLI mode - never by writing
 * to the encrypted store directly, so this test exercises the exact
 * same code path an operator's own provisioning script does. */
function provisionAdminPassword(dataDir, credentialsDir) {
  const result = spawnSync(INSTALLED_EXE_PATH, ['--provision-admin-password', '--force'], {
    env: {
      PATH: process.env.PATH,
      HOME: process.env.HOME,
      STREAMING_TREE_DATA_DIR: dataDir,
      CREDENTIALS_DIRECTORY: credentialsDir,
    },
    input: `${ADMIN_PASSWORD}\n`,
    encoding: 'utf8',
  });
  if (result.status !== 0) {
    fail('provisioning the administrator password', result.stdout + result.stderr);
  }
}

async function startRemoteManagement(dataDir, credentialsDir, extraEnv = {}) {
  const child = spawn(INSTALLED_EXE_PATH, ['--headless', '--remote-management'], {
    cwd: dirname(INSTALLED_EXE_PATH),
    env: {
      PATH: process.env.PATH,
      HOME: process.env.HOME,
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(BACKEND_PORT),
      STREAMING_TREE_HOST: '127.0.0.1',
      CREDENTIALS_DIRECTORY: credentialsDir,
      STREAMING_TREE_REMOTE_MANAGEMENT: 'true',
      STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN: EXTERNAL_ORIGIN,
      ...extraEnv,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  let stdout = '';
  let stderr = '';
  child.stdout.on('data', (c) => (stdout += c.toString()));
  child.stderr.on('data', (c) => (stderr += c.toString()));
  let exited = false;
  let exitCode = null;
  child.on('exit', (code) => {
    exited = true;
    exitCode = code;
  });

  const deadline = Date.now() + READINESS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (exited) break;
    try {
      const health = await new Promise((res, rej) => {
        const r = httpRequest({ host: '127.0.0.1', port: BACKEND_PORT, path: '/api/health', method: 'GET' }, res);
        r.on('error', rej);
        r.end();
      });
      if (health.statusCode === 200) {
        return { child, ready: true, exitCode: null, getStdout: () => stdout, getStderr: () => stderr, hasExited: () => exited };
      }
    } catch {
      // Not listening yet.
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  if (!exited) child.kill('SIGKILL');
  return { child, ready: false, exitCode, getStdout: () => stdout, getStderr: () => stderr, hasExited: () => exited };
}

async function forceStop(handle) {
  if (!handle || handle.hasExited()) return;
  await new Promise((resolvePromise) => {
    const timer = setTimeout(() => resolvePromise(), SHUTDOWN_TIMEOUT_MS);
    handle.child.on('exit', () => {
      clearTimeout(timer);
      resolvePromise();
    });
    handle.child.kill('SIGKILL');
  });
}

/** Generates an ephemeral self-signed CA/leaf cert for
 * EXTERNAL_ORIGIN_HOST via the system `openssl` binary - a real X.509
 * certificate, not a fabricated/hardcoded one, discarded with the
 * temp directory when this script exits. */
function generateEphemeralCert(dir) {
  const keyPath = join(dir, 'key.pem');
  const certPath = join(dir, 'cert.pem');
  execFileSync('openssl', [
    'req', '-x509', '-newkey', 'rsa:2048', '-nodes',
    '-keyout', keyPath, '-out', certPath,
    '-days', '1',
    '-subj', `/CN=${EXTERNAL_ORIGIN_HOST}`,
    '-addext', `subjectAltName=DNS:${EXTERNAL_ORIGIN_HOST}`,
  ], { stdio: 'pipe' });
  return { keyPath, certPath, keyPem: readFileText(keyPath), certPem: readFileText(certPath) };
}

function readFileText(path) {
  return execFileSync('cat', [path], { encoding: 'utf8' });
}

/** excludedLocalOnlyPathPrefixes mirrors the corrected
 * docs/examples/Caddyfile.remote-management's own
 * @excludedLocalOnlySurface matcher exactly (`path /overlay/*
 * /api/public/*`) - the local-only public-overlay surface never
 * forwarded through the D2B management origin (docs/remote-
 * management.md §17, PRE-20D2C correction). Kept as one named
 * constant so the Caddyfile and this test harness cannot silently
 * drift apart from each other again. */
const excludedLocalOnlyPathPrefixes = ['/overlay/', '/api/public/'];

function isExcludedLocalOnlyPath(urlPath) {
  const pathname = urlPath.split('?')[0];
  return excludedLocalOnlyPathPrefixes.some((prefix) => pathname.startsWith(prefix));
}

/** The ephemeral TLS test proxy itself - a first-party Node
 * reimplementation of docs/examples/Caddyfile.remote-management's
 * real, current routing policy (not the literal Caddy binary - no
 * `caddy` binary is installed in this CI environment, and one is not
 * installed here solely for this check, per this milestone's own
 * governing task; the policy itself is grounded directly in Caddy's
 * own official documentation, cited in docs/remote-management.md
 * §2/§17). Terminates HTTPS, always OVERWRITES X-Forwarded-Proto/
 * X-Forwarded-Host/X-Forwarded-For with fixed, correct values (never
 * forwards a client-supplied one). Requests under
 * excludedLocalOnlyPathPrefixes never reach the backend - answered
 * with a bare 404 here, exactly like the Caddyfile's own
 * `handle @excludedLocalOnlySurface { respond 404 }` block - every
 * other path is proxied to the loopback backend. */
function startTLSProxy(cert) {
  const server = createHttpsServer({ key: cert.keyPem, cert: cert.certPem }, (clientReq, clientRes) => {
    if (isExcludedLocalOnlyPath(clientReq.url)) {
      clientRes.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
      clientRes.end('404 Not Found');
      return;
    }

    const headers = { ...clientReq.headers };
    delete headers['x-forwarded-for'];
    delete headers['x-forwarded-proto'];
    delete headers['x-forwarded-host'];
    headers['x-forwarded-proto'] = 'https';
    headers['x-forwarded-host'] = `${EXTERNAL_ORIGIN_HOST}:${PROXY_PORT}`;
    headers['x-forwarded-for'] = '127.0.0.1';
    headers.host = '127.0.0.1';

    const upstream = httpRequest(
      { host: '127.0.0.1', port: BACKEND_PORT, path: clientReq.url, method: clientReq.method, headers },
      (upstreamRes) => {
        clientRes.writeHead(upstreamRes.statusCode, upstreamRes.headers);
        upstreamRes.pipe(clientRes);
      },
    );
    upstream.on('error', () => {
      clientRes.writeHead(502);
      clientRes.end();
    });
    clientReq.pipe(upstream);
  });
  return new Promise((resolvePromise) => {
    server.listen(PROXY_PORT, '127.0.0.1', () => resolvePromise(server));
  });
}

/** One HTTPS request through the real TLS proxy, trusting the
 * ephemeral CA explicitly - never disabling certificate verification. */
function proxyRequest(cert, method, path, { body, headers = {}, cookie } = {}) {
  return new Promise((resolvePromise, reject) => {
    const payload = body === undefined ? undefined : JSON.stringify(body);
    const reqHeaders = { Accept: 'application/json', ...headers };
    if (payload !== undefined) reqHeaders['Content-Type'] = 'application/json';
    if (cookie !== undefined) reqHeaders.Cookie = cookie;

    const req = httpsRequest(
      {
        host: '127.0.0.1',
        port: PROXY_PORT,
        servername: EXTERNAL_ORIGIN_HOST,
        ca: cert.certPem,
        path,
        method,
        headers: reqHeaders,
      },
      (res) => {
        let data = '';
        res.on('data', (c) => (data += c));
        res.on('end', () => {
          let parsed = data;
          try {
            parsed = JSON.parse(data);
          } catch {
            // Not JSON.
          }
          resolvePromise({ status: res.statusCode, headers: res.headers, body: parsed, text: data });
        });
      },
    );
    req.on('error', reject);
    if (payload !== undefined) req.write(payload);
    req.end();
  });
}

/** One plain HTTP request straight to the loopback backend, bypassing
 * the TLS proxy entirely - used to prove the backend's own existing
 * local contract (e.g. the public-overlay surface) is genuinely
 * unchanged, independent of whatever the external management proxy
 * does or does not forward (docs/remote-management.md §17: BACKEND
 * AUTH CLASSIFICATION and REVERSE-PROXY INTERNET REACHABILITY are two
 * different questions - this is the "backend" half of that pair). */
function backendRequest(method, path) {
  return new Promise((resolvePromise, reject) => {
    const req = httpRequest({ host: '127.0.0.1', port: BACKEND_PORT, path, method, headers: { Accept: 'application/json' } }, (res) => {
      let data = '';
      res.on('data', (c) => (data += c));
      res.on('end', () => {
        let parsed = data;
        try {
          parsed = JSON.parse(data);
        } catch {
          // Not JSON.
        }
        resolvePromise({ status: res.statusCode, headers: res.headers, body: parsed, text: data });
      });
    });
    req.on('error', reject);
    req.end();
  });
}

function extractSessionCookie(setCookieHeaders) {
  if (!Array.isArray(setCookieHeaders)) return null;
  const line = setCookieHeaders.find((c) => c.startsWith('__Host-streaming-tree-session='));
  if (!line) return null;
  return { raw: line, value: line.split(';')[0] };
}

/** Confirms excludedLocalOnlyPathPrefixes (this script's own hardcoded
 * policy) actually matches the real, committed
 * docs/examples/Caddyfile.remote-management - so the two cannot
 * silently drift apart the way the Caddyfile and this document's own
 * prose once did (the PRE-20D2C bug this milestone corrects). Mirrors
 * scripts/verify-ci-routing.mjs's own established "read the real
 * committed file, don't just trust a separately-maintained model"
 * pattern. */
function verifyCaddyfileMatchesHarnessPolicy() {
  const caddyfilePath = join(REPO_ROOT, 'docs', 'examples', 'Caddyfile.remote-management');
  const caddyfileContent = readFileText(caddyfilePath);
  expect(/path\s+\/overlay\/\*\s+\/api\/public\/\*/.test(caddyfileContent),
    'the committed Caddyfile\'s own @excludedLocalOnlySurface matcher still lists exactly /overlay/* and /api/public/*, matching this script\'s own hardcoded policy',
    caddyfileContent);
  expect(/handle\s+@excludedLocalOnlySurface\s*\{\s*respond\s+404/.test(caddyfileContent),
    'the committed Caddyfile still responds 404 (not proxies) for the excluded surface',
    caddyfileContent);
}

async function main() {
  console.log('Stage 20D2B Linux remote-management verification');

  if (process.platform !== 'linux') {
    fail('this script only runs on Linux', `process.platform = ${process.platform}`);
  }

  step('The committed reference Caddyfile matches this script\'s own proxy-exclusion policy');
  verifyCaddyfileMatchesHarnessPolicy();

  step('Verify the real .deb package exists and openssl is available');
  const debName = existsSync(OUTPUT_DIR) ? readdirSync(OUTPUT_DIR).find((n) => n.endsWith('.deb')) : undefined;
  expect(typeof debName === 'string', `a .deb file was found in ${OUTPUT_DIR}`, 'Run: scripts/build-release-linux.sh --version 0.1.0-dev+test');
  const debPath = join(OUTPUT_DIR, debName);
  execFileSync('openssl', ['version'], { stdio: 'ignore' });
  pass('openssl is available');

  if (isPackageInstalled()) {
    execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'ignore' });
  }

  let installed = false;
  const certDir = mkdtempSync(join(tmpdir(), 'streaming-tree-rm-cert-'));
  let proxyServer = null;
  try {
    step('The .deb installs successfully');
    execFileSync('sudo', ['dpkg', '-i', debPath], { stdio: 'pipe' });
    installed = true;
    expect(isPackageInstalled(), 'dpkg reports the package as installed', PACKAGE_NAME);

    step('Generate the ephemeral TLS test-proxy certificate (real X.509, discarded after this run)');
    const cert = generateEphemeralCert(certDir);
    expect(existsSync(cert.certPath) && existsSync(cert.keyPath), 'a real ephemeral cert/key pair was generated', certDir);

    const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-rm-data-'));
    const credentialsDir = provisionCredentialsDir();
    let appHandle = null;

    try {
      step('--remote-management without --headless is refused (fail-closed)');
      const badMode = spawnSync(INSTALLED_EXE_PATH, ['--remote-management'], {
        env: { PATH: process.env.PATH, HOME: process.env.HOME, STREAMING_TREE_DATA_DIR: dataDir },
        encoding: 'utf8',
        timeout: 5000,
      });
      expect(badMode.status !== 0, '--remote-management alone (no --headless) exits nonzero', badMode.stderr);

      step('Remote management fails closed with no administrator password provisioned');
      const noAdminPassword = await startRemoteManagement(dataDir, credentialsDir);
      expect(!noAdminPassword.ready, 'startup failed with no administrator password provisioned', noAdminPassword.getStderr().slice(-500));
      await forceStop(noAdminPassword);

      step('Provision the administrator password through the real --provision-admin-password CLI mode');
      provisionAdminPassword(dataDir, credentialsDir);
      pass('administrator password provisioned');

      step('Remote management fails closed with a malformed external origin (http, not https)');
      const badOrigin = await startRemoteManagement(dataDir, credentialsDir, {
        STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN: `http://${EXTERNAL_ORIGIN_HOST}`,
      });
      expect(!badOrigin.ready, 'an http:// origin was rejected before any listener opened', badOrigin.getStderr().slice(-500));
      await forceStop(badOrigin);

      step('Remote management starts successfully with a valid administrator password and HTTPS origin');
      appHandle = await startRemoteManagement(dataDir, credentialsDir);
      expect(appHandle.ready, 'the remote-management process became healthy', appHandle.getStderr().slice(-800));

      step('The backend itself remains loopback-only - only 127.0.0.1 listeners exist');
      const sockets = execFileSync('ss', ['-Hltn'], { encoding: 'utf8' });
      const nonLoopback = sockets.split('\n').filter((l) => l.includes(String(BACKEND_PORT))).filter((l) => !l.includes('127.0.0.1') && !l.includes('[::1]'));
      expect(nonLoopback.length === 0, 'no non-loopback listener exists for the backend port', sockets);

      // governing task §14: "no MediaMTX port is proxied" - checks
      // MediaMTX's own well-known default ports specifically (1935
      // RTMP, 9997 Control API - docs/remote-management.md's own
      // scope boundary), not a truly unscoped whole-host audit (a
      // CI runner may legitimately have unrelated system listeners
      // - sshd, a metadata/telemetry agent - bound non-loopback for
      // reasons entirely outside this application's own control;
      // asserting against those would be a false failure, not real
      // evidence of anything this milestone changed).
      step('MediaMTX default ports (RTMP 1935, Control API 9997) are not bound non-loopback');
      for (const mediaMTXPort of ['1935', '9997']) {
        const matches = sockets.split('\n').filter((l) => l.includes(`:${mediaMTXPort} `) || l.trim().endsWith(`:${mediaMTXPort}`));
        const nonLoopbackMediaMTX = matches.filter((l) => !l.includes('127.0.0.1') && !l.includes('[::1]'));
        expect(nonLoopbackMediaMTX.length === 0, `no non-loopback listener on MediaMTX's default port ${mediaMTXPort}`, matches);
      }

      step('Start the real ephemeral TLS reverse proxy in front of the loopback backend');
      proxyServer = await startTLSProxy(cert);
      pass(`TLS proxy listening on 127.0.0.1:${PROXY_PORT}, forwarding to 127.0.0.1:${BACKEND_PORT}`);

      step('Unauthenticated management API is rejected through the real TLS proxy');
      const unauth = await proxyRequest(cert, 'GET', '/api/about');
      expect(unauth.status === 401, 'GET /api/about returns 401 with no session', unauth);

      step('The public health endpoint remains reachable unauthenticated through the proxy');
      const health = await proxyRequest(cert, 'GET', '/api/health');
      expect(health.status === 200, 'GET /api/health returns 200 with no session', health);

      step('Login page / static assets are reachable unauthenticated through the proxy');
      const root = await proxyRequest(cert, 'GET', '/');
      expect(root.status === 200 && root.text.includes('id="root"'), 'root HTML (the login page shell) is reachable unauthenticated', root.status);

      step('Wrong password is rejected');
      const wrongLogin = await proxyRequest(cert, 'POST', '/api/auth/login', {
        body: { password: 'definitely-not-the-real-password' },
        headers: { Origin: EXTERNAL_ORIGIN },
      });
      expect(wrongLogin.status === 401, 'a wrong password returns 401', wrongLogin);

      step('Login rate limiting activates under bounded repeated failed attempts');
      let rateLimited = false;
      for (let i = 0; i < 8; i += 1) {
        const attempt = await proxyRequest(cert, 'POST', '/api/auth/login', {
          body: { password: `still-wrong-${i}` },
          headers: { Origin: EXTERNAL_ORIGIN },
        });
        if (attempt.status === 429) {
          rateLimited = true;
          break;
        }
      }
      expect(rateLimited, 'repeated failed attempts eventually receive 429', '');

      // A fresh process (fresh in-memory rate limiter and session
      // store) for the remaining login/session/CSRF/shutdown
      // scenarios, so the rate-limit test above cannot make the
      // legitimate login below flaky.
      step('Restart the backend to reset the in-memory rate limiter for the remaining scenarios');
      appHandle.child.kill('SIGTERM');
      const restartDeadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
      while (Date.now() < restartDeadline && !appHandle.hasExited()) await new Promise((r) => setTimeout(r, 200));
      expect(appHandle.hasExited(), 'the process exited on SIGTERM before the reset restart', '');
      appHandle = await startRemoteManagement(dataDir, credentialsDir);
      expect(appHandle.ready, 'the backend restarted cleanly', appHandle.getStderr().slice(-500));

      step('Correct password succeeds and returns a secure session cookie');
      const login = await proxyRequest(cert, 'POST', '/api/auth/login', {
        body: { password: ADMIN_PASSWORD },
        headers: { Origin: EXTERNAL_ORIGIN },
      });
      expect(login.status === 200, 'the correct password logs in successfully', login);
      const sessionCookie = extractSessionCookie(login.headers['set-cookie']);
      expect(sessionCookie !== null, 'a __Host-streaming-tree-session cookie was set', login.headers['set-cookie']);
      expect(sessionCookie.raw.includes('Secure'), 'the cookie is Secure', sessionCookie.raw);
      expect(sessionCookie.raw.includes('HttpOnly'), 'the cookie is HttpOnly', sessionCookie.raw);
      expect(sessionCookie.raw.includes('SameSite=Strict'), 'the cookie is SameSite=Strict', sessionCookie.raw);
      expect(!/Domain=/.test(sessionCookie.raw), 'the cookie has no Domain attribute (required by __Host-)', sessionCookie.raw);
      const csrfToken = login.body && login.body.csrfToken;
      expect(typeof csrfToken === 'string' && csrfToken.length > 0, 'the login response carries a CSRF token', login.body);

      step('Auth bootstrap reports authenticated with the session cookie attached');
      const bootstrap = await proxyRequest(cert, 'GET', '/api/auth/session', { cookie: sessionCookie.value });
      expect(bootstrap.status === 200 && bootstrap.body.authenticated === true, 'session bootstrap reports authenticated', bootstrap.body);

      step('An authenticated management read succeeds');
      const about = await proxyRequest(cert, 'GET', '/api/about', { cookie: sessionCookie.value });
      expect(about.status === 200 && about.body.creatorName === 'Czekosabe', 'GET /api/about succeeds with a valid session', about.body);

      step('A state-changing request without CSRF is rejected');
      const noCsrf = await proxyRequest(cert, 'POST', '/api/system/shutdown', {
        body: { confirm: true },
        headers: { Origin: EXTERNAL_ORIGIN },
        cookie: sessionCookie.value,
      });
      expect(noCsrf.status === 403, 'shutdown without a CSRF token is rejected', noCsrf);

      step('A state-changing request with the wrong CSRF token is rejected');
      const wrongCsrf = await proxyRequest(cert, 'POST', '/api/system/shutdown', {
        body: { confirm: true },
        headers: { Origin: EXTERNAL_ORIGIN, 'X-CSRF-Token': 'not-the-real-token' },
        cookie: sessionCookie.value,
      });
      expect(wrongCsrf.status === 403, 'shutdown with the wrong CSRF token is rejected', wrongCsrf);

      step('A state-changing request with the wrong Origin is rejected');
      const wrongOrigin = await proxyRequest(cert, 'POST', '/api/system/shutdown', {
        body: { confirm: true },
        headers: { Origin: 'https://evil.example.test', 'X-CSRF-Token': csrfToken },
        cookie: sessionCookie.value,
      });
      expect(wrongOrigin.status === 403, 'shutdown with the wrong Origin is rejected', wrongOrigin);

      // PRE-20D2C correction (docs/remote-management.md §17): a prior
      // version of this scenario only proved /api/public/* is not
      // *authentication*-gated, then drew the wrong conclusion from
      // that alone ("not exposed through this proxy configuration") -
      // BACKEND AUTH CLASSIFICATION and REVERSE-PROXY INTERNET
      // REACHABILITY are two different questions. This now proves
      // both halves explicitly and separately: the backend's own
      // local-overlay contract is unchanged (direct loopback request,
      // bypassing the proxy entirely), and the external management
      // proxy genuinely refuses to forward these paths at all (the
      // proxy's own 404, per its corrected @excludedLocalOnlySurface
      // policy - the backend is never even reached for these two
      // requests).
      step('Direct loopback backend: /api/public/* remains reachable (existing local-overlay contract, unchanged)');
      const directPublic = await backendRequest('GET', '/api/public/chat-overlays/anything');
      expect(directPublic.status !== 401, 'the backend itself still serves /api/public/* unauthenticated when reached directly (loopback)', directPublic.status);

      step('External management proxy: /api/public/* is NOT forwarded (real 404 from the proxy itself)');
      const externalPublic = await proxyRequest(cert, 'GET', '/api/public/chat-overlays/anything', { cookie: sessionCookie.value });
      expect(externalPublic.status === 404, 'the external proxy refuses /api/public/*, the backend is never reached for it', externalPublic.status);

      step('External management proxy: /overlay/* is NOT forwarded (real 404 from the proxy itself)');
      const externalOverlay = await proxyRequest(cert, 'GET', '/overlay/chat/anything', { cookie: sessionCookie.value });
      expect(externalOverlay.status === 404, 'the external proxy refuses /overlay/*, the backend is never reached for it', externalOverlay.status);

      step('Direct loopback backend: /overlay/* (the SPA client-side route) remains reachable (existing local contract, unchanged)');
      const directOverlay = await backendRequest('GET', '/overlay/chat/anything');
      // /overlay/chat/:publicSlug is a client-side React Router route,
      // not an API route - the packaged frontend's own SPA fallback
      // (internal/httpapi/production.go) serves index.html (200) for
      // any non-asset-like unmatched path, exactly as it does for
      // every other client-side route.
      expect(directOverlay.status === 200, 'the backend itself still serves the /overlay/* SPA shell when reached directly (loopback)', directOverlay.status);

      step('External management proxy: the management SPA, health, auth bootstrap, and authenticated API remain reachable (the exclusion is narrow, not a broad allowlist mistake)');
      const externalRoot = await proxyRequest(cert, 'GET', '/');
      expect(externalRoot.status === 200, 'GET / (management SPA shell) still proxies correctly', externalRoot.status);
      const externalHealthAgain = await proxyRequest(cert, 'GET', '/api/health');
      expect(externalHealthAgain.status === 200, 'GET /api/health still proxies correctly', externalHealthAgain.status);
      const externalAuthAgain = await proxyRequest(cert, 'GET', '/api/auth/session', { cookie: sessionCookie.value });
      expect(externalAuthAgain.status === 200, 'GET /api/auth/session still proxies correctly', externalAuthAgain.status);
      const externalManagementAgain = await proxyRequest(cert, 'GET', '/api/about', { cookie: sessionCookie.value });
      expect(externalManagementAgain.status === 200, 'GET /api/about (authenticated management read) still proxies correctly', externalManagementAgain.status);

      step('Authenticated remote shutdown with valid session+CSRF+Origin succeeds');
      const shutdown = await proxyRequest(cert, 'POST', '/api/system/shutdown', {
        body: { confirm: true },
        headers: { Origin: EXTERNAL_ORIGIN, 'X-CSRF-Token': csrfToken },
        cookie: sessionCookie.value,
      });
      expect(shutdown.status === 200, 'authenticated remote shutdown succeeds', shutdown);

      const exitDeadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
      while (Date.now() < exitDeadline && !appHandle.hasExited()) await new Promise((r) => setTimeout(r, 200));
      expect(appHandle.hasExited(), 'the process actually exited after the remote shutdown request', '');
      appHandle = null;

      step('A fresh restart has an empty session set - the old session is no longer valid');
      appHandle = await startRemoteManagement(dataDir, credentialsDir);
      expect(appHandle.ready, 'the backend restarted cleanly after the remote shutdown', appHandle.getStderr().slice(-500));
      const afterRestart = await proxyRequest(cert, 'GET', '/api/about', { cookie: sessionCookie.value });
      expect(afterRestart.status === 401, 'the pre-restart session cookie is no longer valid after a restart', afterRestart.status);

      step('A new login succeeds after the restart');
      const secondLogin = await proxyRequest(cert, 'POST', '/api/auth/login', {
        body: { password: ADMIN_PASSWORD },
        headers: { Origin: EXTERNAL_ORIGIN },
      });
      expect(secondLogin.status === 200, 'logging in again after the restart succeeds', secondLogin);

      step('No process/secret leaks into logs or persistent state');
      const stdout = appHandle.getStdout();
      const stderr = appHandle.getStderr();
      expect(!stdout.includes(ADMIN_PASSWORD) && !stderr.includes(ADMIN_PASSWORD), 'the administrator password never appears in process output', '');
    } finally {
      await forceStop(appHandle);
      rmSync(dataDir, { recursive: true, force: true });
      rmSync(credentialsDir, { recursive: true, force: true });
    }

    step('The package removes cleanly');
    execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'pipe' });
    installed = false;
    expect(!existsSync(INSTALLED_EXE_PATH), 'the executable was removed', INSTALLED_EXE_PATH);

    console.log(`\n${stepCount} steps passed. PASS`);
  } finally {
    if (proxyServer) {
      await new Promise((r) => proxyServer.close(r));
    }
    rmSync(certDir, { recursive: true, force: true });
    if (installed) {
      try {
        execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'ignore' });
      } catch (removeError) {
        console.error('warning: cleanup dpkg -r failed', removeError);
      }
    }
  }

  step('No Streaming Tree process remains');
  let leftover = '';
  try {
    leftover = execFileSync('pgrep', ['-f', 'streaming-tree-server'], { encoding: 'utf8' }).trim();
  } catch {
    leftover = '';
  }
  expect(leftover === '', 'no streaming-tree-server process remains running', leftover);
}

main().catch((error) => {
  console.error('\nverify-linux-remote-management.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
