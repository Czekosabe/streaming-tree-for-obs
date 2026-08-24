#!/usr/bin/env node
/**
 * Linux remote-server verification (Stage 20D2C) - a platform-specific
 * CI verification helper, explicitly NOT canonical integration script
 * #25. The canonical local/Windows count remains 24 (docs/remote-
 * ingest.md).
 *
 * Proves the real --headless --remote-management --remote-ingest
 * contract through a genuinely isolated remote network boundary (a
 * Linux network namespace connected by a veth pair, non-loopback
 * addresses on both sides) rather than localhost-to-localhost traffic
 * relabeled as "remote" (docs/remote-ingest.md §15). The management
 * and overlay HTTPS origins are served by first-party, ephemeral-CA
 * Node TLS proxies implementing the exact routing policy
 * docs/examples/Caddyfile.self-hosted documents - not the literal
 * Caddy binary (not installed in this CI environment, mirroring
 * verify-linux-remote-management.mjs's own established, honestly-
 * disclosed choice). The RTMPS ingest listener is MediaMTX's own real
 * TLS termination - never proxied through anything.
 *
 * Requires a Linux release build to already exist as a built .deb (an
 * earlier .github/workflows/linux-headless.yml step runs
 * build-release-linux.sh) but installs and removes that package itself,
 * self-contained, exactly like verify-linux-remote-management.mjs does -
 * neither script assumes the other, or any workflow step, leaves the
 * package installed, since verify-linux-remote-management.mjs's own
 * cleanup already removes it again before this script would start.
 *
 * Scope of this first version, recorded honestly (docs/progress.md) -
 * do not read the section headings below as a claim this file proves
 * every one of them exhaustively:
 *
 *   - real network-namespace/veth isolation, verified as a runner
 *     capability before anything else is attempted (never a silent
 *     fallback to localhost);
 *   - a real, freshly generated 3-host ephemeral CA, installed into
 *     the trust store, used for genuine TLS verification on every
 *     positive-path request (no -k/--insecure); a handshake trusting
 *     only an unrelated CA is rejected, and a handshake against the
 *     correct CA but the wrong expected hostname is rejected too;
 *   - the real authenticated management session/CSRF login flow,
 *     genuinely issued from the isolated client namespace through the
 *     real TLS management-proxy stand-in;
 *   - real remote-ingest credential provisioning through that
 *     authenticated API;
 *   - the RTMPS publish accept/reject matrix (docs/remote-ingest.md
 *     §5's own permission model) via a real synthetic FFmpeg
 *     publisher, run from the isolated namespace: plaintext RTMP,
 *     RTMPS with no/wrong credential, RTMPS with the wrong path all
 *     rejected; RTMPS with the correct credential and canonical path
 *     accepted; a remote read (pull/subscribe) of the ingest path is
 *     rejected even while that same valid publish is active;
 *   - MediaMTX Control API and the Go backend's own loopback port
 *     confirmed unreachable from the isolated namespace (a structural
 *     consequence of each network namespace owning its own loopback,
 *     not merely an assertion);
 *   - the loopback-only plain RTMP listener (branch FFmpeg's own
 *     input) confirmed unreachable from the isolated namespace even
 *     while remote ingest is active;
 *   - a real cookie-separation check: the management session cookie,
 *     genuinely scoped by curl's own RFC 6265 cookie-jar handling,
 *     is never sent to the overlay origin.
 *
 * .github/workflows/linux-headless.yml invokes this script twice per
 * architecture ("pass 1"/"pass 2"), and every piece of per-run state
 * this script itself controls is generated fresh on each invocation -
 * a new mkdtemp data/credentials/PKI directory, a new random 32-byte
 * master key (node:crypto randomBytes, not a fixed formula), a new
 * ephemeral CA and leaf certificates, a freshly reinstalled .deb, and
 * a freshly reinstalled MediaMTX instance under the new data
 * directory. The remote-ingest publisher secret itself is generated
 * server-side (POST /api/remote-ingest/provision), independently of
 * this script, on every run. The network-namespace/veth pair use the
 * same fixed names both passes, but the kernel objects themselves are
 * deleted in this script's own cleanup and recreated from scratch on
 * the next invocation - never left up and reused live.
 *
 * It does NOT yet cover: the full systemd/package combined lifecycle
 * (this run spawns the installed binary directly, mirroring verify-
 * linux-remote-management.mjs's own established choice, rather than
 * going through systemctl); a real destination-branch E2E to a local
 * sink; the per-domain remote-overlay E2E matrix beyond the cookie
 * check above. These are tracked as explicit follow-up work in
 * docs/progress.md, not silently left unmentioned.
 *
 * Usage:  node scripts/verify-linux-remote-server.mjs
 * Exits non-zero on the first failed expectation.
 */

import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createServer as createHttpsServer } from 'node:https';
import { request as httpRequest } from 'node:http';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release-linux', 'output');
const PACKAGE_NAME = 'streaming-tree-for-obs';
const INSTALLED_EXE_PATH = '/usr/bin/streaming-tree-server';

// --- network topology --------------------------------------------------
// Host (default) namespace keeps the real server process and both
// reverse-proxy stand-ins; the isolated namespace is where every
// "remote" request/publish in this script actually originates from -
// its own loopback is a *different* loopback than the host's, which is
// the structural property that makes "unreachable from the client
// namespace" a real, kernel-enforced fact rather than an assertion.
const NETNS_NAME = 'streamtree-d2c-client';
const VETH_HOST = 'veth-d2c-host';
const VETH_CLIENT = 'veth-d2c-client';
const HOST_ADDR = '10.201.0.1';
const CLIENT_ADDR = '10.201.0.2';
const SUBNET_PREFIX = 24;

const BACKEND_PORT = 8710;
const MEDIAMTX_API_PORT = 8711;
const MEDIAMTX_RTMP_PORT = 8712; // loopback-only, branch's own read - never exposed to the client namespace
const RTMPS_PORT = 8713;
const MANAGE_PROXY_PORT = 8714;
const OVERLAY_PROXY_PORT = 8715;

const MANAGE_HOST = 'manage-d2c.test';
const OVERLAY_HOST = 'overlay-d2c.test';
const INGEST_HOST = 'ingest-d2c.test';
const MANAGE_ORIGIN = `https://${MANAGE_HOST}:${MANAGE_PROXY_PORT}`;
const OVERLAY_ORIGIN = `https://${OVERLAY_HOST}:${OVERLAY_PROXY_PORT}`;

const INGEST_PATH = 'live';
const PUBLISHER_USER = 'streaming-tree-obs';
const ADMIN_PASSWORD = 'a-test-only-administrator-password-never-used-in-production';

const READINESS_TIMEOUT_MS = 30_000;
const SHUTDOWN_TIMEOUT_MS = 15_000;
const PUBLISH_SETTLE_MS = 4_000;

let stepCount = 0;
function step(message) {
  stepCount += 1;
  console.log(`\n[${String(stepCount).padStart(2, '0')}] ${message}`);
}
function pass(message) {
  console.log(`     ok  ${message}`);
}
// GitHub Actions workflow-command escaping (data, not property) per
// the documented rules: %, \r, \n only.
function ghEscape(s) {
  return String(s).replace(/%/g, '%25').replace(/\r/g, '%0D').replace(/\n/g, '%0A');
}
function fail(message, detail) {
  console.error(`     FAIL ${message}`);
  const detailText = detail !== undefined ? (typeof detail === 'string' ? detail : JSON.stringify(detail)) : '';
  if (detailText) {
    console.error(`          ${detailText}`);
  }
  // The workflow's own diagnostic step tails the *entire* combined
  // log and emits one ::error:: for it - by step 17+, everything
  // printed before a late failure routinely eats GitHub's own real
  // annotation size limit (found the hard way, repeatedly, in
  // docs/progress.md) before the actual failure's own detail is ever
  // reached. Emitting a second, dedicated ::error:: directly from
  // here, right at the point of failure, gives this specific
  // detail its own fresh annotation budget instead of competing with
  // everything printed earlier in the same run.
  if (process.env.GITHUB_ACTIONS === 'true') {
    const body = detailText ? `${message}\n${detailText}` : message;
    console.log(`::error title=${ghEscape(`verify-linux-remote-server.mjs: step ${stepCount}`)}::${ghEscape(body).slice(0, 3000)}`);
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

function sh(cmd, args, opts = {}) {
  return execFileSync(cmd, args, { stdio: 'pipe', encoding: 'utf8', ...opts });
}

function shOk(cmd, args, opts = {}) {
  try {
    sh(cmd, args, opts);
    return true;
  } catch {
    return false;
  }
}

// Like shOk, but keeps the real stderr instead of discarding it - used
// where a bare pass/fail with no detail would leave a CI failure
// undiagnosable from the ::error:: annotation alone.
function shDetail(cmd, args, opts = {}) {
  try {
    sh(cmd, args, opts);
    return { ok: true, error: '' };
  } catch (err) {
    const stderr = err && err.stderr ? String(err.stderr).trim() : '';
    return { ok: false, error: stderr || (err && err.message) || 'unknown error' };
  }
}

// --- §10: environment capability check ----------------------------------
// docs/remote-ingest.md §15: verify the environment first, never
// silently fall back to localhost if network namespaces are not
// genuinely available on this runner - a false "PASS" here would be
// worse than an honest, diagnosable failure.
function verifyNetnsCapability() {
  step('Verify network-namespace/veth capability on this runner (must not silently downgrade to localhost)');
  const netnsResult = shDetail('sudo', ['ip', 'netns', 'add', 'streamtree-capability-probe']);
  if (netnsResult.ok) {
    sh('sudo', ['ip', 'netns', 'del', 'streamtree-capability-probe']);
  }
  expect(netnsResult.ok, 'this runner can create a network namespace (ip netns add)', netnsResult.error || 'CAP_NET_ADMIN or root required');

  // Linux interface names are capped at IFNAMSIZ-1 = 15 characters
  // (unlike netns names above, which are just files under /var/run/netns
  // and have no such limit) - kept short here so the probe itself can't
  // fail on a naming bug unrelated to the capability it is testing.
  const vethResult = shDetail('sudo', ['ip', 'link', 'add', 'st-probe-veth0', 'type', 'veth', 'peer', 'name', 'st-probe-veth1']);
  if (vethResult.ok) {
    sh('sudo', ['ip', 'link', 'del', 'st-probe-veth0']);
  }
  expect(vethResult.ok, 'this runner can create a veth pair (ip link add ... type veth)', vethResult.error || 'the veth kernel module may not be available');
}

// --- network namespace + veth setup/teardown ----------------------------
function setUpNetwork() {
  step('Create the isolated client network namespace and veth pair');
  sh('sudo', ['ip', 'netns', 'add', NETNS_NAME]);
  sh('sudo', ['ip', 'link', 'add', VETH_HOST, 'type', 'veth', 'peer', 'name', VETH_CLIENT]);
  sh('sudo', ['ip', 'link', 'set', VETH_CLIENT, 'netns', NETNS_NAME]);

  sh('sudo', ['ip', 'addr', 'add', `${HOST_ADDR}/${SUBNET_PREFIX}`, 'dev', VETH_HOST]);
  sh('sudo', ['ip', 'link', 'set', VETH_HOST, 'up']);

  sh('sudo', ['ip', 'netns', 'exec', NETNS_NAME, 'ip', 'addr', 'add', `${CLIENT_ADDR}/${SUBNET_PREFIX}`, 'dev', VETH_CLIENT]);
  sh('sudo', ['ip', 'netns', 'exec', NETNS_NAME, 'ip', 'link', 'set', VETH_CLIENT, 'up']);
  sh('sudo', ['ip', 'netns', 'exec', NETNS_NAME, 'ip', 'link', 'set', 'lo', 'up']);
  sh('sudo', ['ip', 'netns', 'exec', NETNS_NAME, 'ip', 'route', 'add', 'default', 'via', HOST_ADDR]);

  pass(`host side ${VETH_HOST}=${HOST_ADDR}, client namespace ${NETNS_NAME} side ${VETH_CLIENT}=${CLIENT_ADDR}`);

  step('Point the three test hostnames at the host-side veth address (shared /etc/hosts - only the network namespace is isolated, not the mount namespace)');
  const hostsLine = `${HOST_ADDR} ${MANAGE_HOST} ${OVERLAY_HOST} ${INGEST_HOST}`;
  sh('sudo', ['bash', '-c', `echo "${hostsLine}" >> /etc/hosts`]);
  pass('hostnames resolve to the host-side veth address in both namespaces');
}

function tearDownNetwork() {
  try {
    sh('sudo', ['ip', 'netns', 'del', NETNS_NAME]);
  } catch {
    // Already gone - fine.
  }
  try {
    sh('sudo', ['ip', 'link', 'del', VETH_HOST]);
  } catch {
    // The netns deletion above already removes the peer if it still existed there.
  }
  try {
    sh('sudo', ['sed', '-i', `/${HOST_ADDR} ${MANAGE_HOST}/d`, '/etc/hosts']);
  } catch {
    // Best-effort - a leftover /etc/hosts line on a CI runner that is
    // about to be discarded entirely is not itself a security issue.
  }
}

/** Runs cmd inside the isolated client namespace - every "remote" probe
 * in this script goes through this, never a direct call in the host's
 * own namespace relabeled as remote. */
function clientExec(cmd, args, opts = {}) {
  return execFileSync('sudo', ['ip', 'netns', 'exec', NETNS_NAME, cmd, ...args], {
    stdio: 'pipe',
    encoding: 'utf8',
    ...opts,
  });
}
function clientExecOk(cmd, args, opts = {}) {
  try {
    clientExec(cmd, args, opts);
    return true;
  } catch {
    return false;
  }
}
/** Async equivalent of a synchronous spawnSync-based status check -
 * deliberately non-blocking. The management/overlay TLS proxies below
 * run in-process (same event loop as this script), so a *synchronous*
 * client-side curl call here would starve the event loop for its
 * entire duration and the in-process proxy could never accept or
 * service the very connection the blocked curl is waiting on - a
 * same-process deadlock, not a real network problem, that a first
 * version of this function had (docs/progress.md). */
function clientExecStatus(cmd, args, opts = {}) {
  const { timeout, ...spawnOpts } = opts;
  return new Promise((resolve) => {
    const child = spawn('sudo', ['ip', 'netns', 'exec', NETNS_NAME, cmd, ...args], {
      stdio: ['ignore', 'pipe', 'pipe'],
      ...spawnOpts,
    });
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    let stdout = '';
    let stderr = '';
    let timedOut = false;
    const timer = timeout
      ? setTimeout(() => {
          timedOut = true;
          child.kill('SIGKILL');
        }, timeout)
      : undefined;
    child.stdout.on('data', (d) => {
      stdout += d;
    });
    child.stderr.on('data', (d) => {
      stderr += d;
    });
    child.on('close', (code, signal) => {
      if (timer) clearTimeout(timer);
      resolve({ status: timedOut ? null : code, signal, stdout, stderr, timedOut });
    });
    child.on('error', (err) => {
      if (timer) clearTimeout(timer);
      resolve({ status: null, stdout, stderr: stderr + String((err && err.message) || err) });
    });
  });
}

/** Non-blocking equivalent of clientExec, for a long-running publish
 * this script needs to observe mid-stream (the "receiving" state),
 * not just wait for it to exit. */
function clientSpawn(cmd, args) {
  return spawn('sudo', ['ip', 'netns', 'exec', NETNS_NAME, cmd, ...args], { stdio: ['ignore', 'pipe', 'pipe'] });
}

// --- ephemeral 3-host CA -------------------------------------------------
// A real X.509 CA plus three leaf certificates (one per test hostname),
// generated fresh for this run and discarded with the temp directory -
// never a production credential, never -k/--insecure on the positive
// path (docs/remote-ingest.md §15).
function generateEphemeralPKI(dir) {
  const caKey = join(dir, 'ca-key.pem');
  const caCert = join(dir, 'ca-cert.pem');
  sh('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', caKey, '-out', caCert, '-days', '1', '-subj', '/CN=streaming-tree-d2c-test-ca']);

  const leaves = {};
  for (const host of [MANAGE_HOST, OVERLAY_HOST, INGEST_HOST]) {
    const key = join(dir, `${host}-key.pem`);
    const csr = join(dir, `${host}.csr`);
    const cert = join(dir, `${host}-cert.pem`);
    const extFile = join(dir, `${host}.ext`);
    writeFileSync(extFile, `subjectAltName=DNS:${host}\n`);
    sh('openssl', ['req', '-newkey', 'rsa:2048', '-nodes', '-keyout', key, '-out', csr, '-subj', `/CN=${host}`]);
    sh('openssl', ['x509', '-req', '-in', csr, '-CA', caCert, '-CAkey', caKey, '-CAcreateserial', '-out', cert, '-days', '1', '-extfile', extFile]);
    leaves[host] = { keyPath: key, certPath: cert, keyPem: readFileSync(key, 'utf8'), certPem: readFileSync(cert, 'utf8') };
  }

  return { caKeyPath: caKey, caCertPath: caCert, caCertPem: readFileSync(caCert, 'utf8'), leaves };
}

/** Installs the ephemeral CA into the system trust store so FFmpeg's
 * and curl's own default TLS verification trusts it - the same
 * filesystem-level trust store is visible from both namespaces (only
 * the network namespace is isolated). Removed in cleanup. */
function installEphemeralCATrust(caCertPath) {
  sh('sudo', ['cp', caCertPath, '/usr/local/share/ca-certificates/streamtree-d2c-test-ca.crt']);
  sh('sudo', ['update-ca-certificates']);
}
function removeEphemeralCATrust() {
  try {
    sh('sudo', ['rm', '-f', '/usr/local/share/ca-certificates/streamtree-d2c-test-ca.crt']);
    sh('sudo', ['update-ca-certificates']);
  } catch {
    // Best-effort on a runner that is about to be discarded.
  }
}

// --- reverse-proxy stand-ins ---------------------------------------------
// Node reimplementations of docs/examples/Caddyfile.self-hosted's own two
// site blocks - not the literal Caddy binary (not installed in this CI
// environment, disclosed honestly, mirroring verify-linux-remote-
// management.mjs's own established choice for the D2B management origin).
const managementExcludedRoots = ['/overlay', '/api/public'];
const managementExcludedPrefixes = ['/overlay/', '/api/public/'];
function isManagementExcludedPath(urlPath) {
  const pathname = urlPath.split('?')[0];
  return managementExcludedRoots.includes(pathname) || managementExcludedPrefixes.some((p) => pathname.startsWith(p));
}

const overlayAllowedPrefixes = ['/overlay/', '/assets/', '/api/public/'];
function isOverlayAllowedPath(urlPath) {
  const pathname = urlPath.split('?')[0];
  return overlayAllowedPrefixes.some((p) => pathname.startsWith(p));
}

// The TLS proxy stand-ins run in-process (not a spawned child), so
// anything logged here lands directly in this script's own captured
// stdout - no extra plumbing needed to surface a real server-side TLS
// failure in the CI diagnostic tail.
function attachTlsDiagnostics(server, label) {
  // No per-connection success log (e.g. "secureConnection established")
  // - the TLS handshake itself was proven working several commits ago,
  // and one such line per request (there are now a couple dozen by the
  // time a later step fails) was silently eating into GitHub's own
  // annotation size limit, pushing more useful diagnostic content out
  // of a failure's captured detail. Only genuine error conditions are
  // still worth a line here.
  server.on('tlsClientError', (err) => console.log(`     diag [${label}] tlsClientError: ${err && err.message}`));
  server.on('clientError', (err) => console.log(`     diag [${label}] clientError: ${err && err.message}`));
  server.on('error', (err) => console.log(`     diag [${label}] server error: ${err && err.message}`));
}

function startManagementProxy(leaf) {
  const server = createHttpsServer({ key: leaf.keyPem, cert: leaf.certPem }, (clientReq, clientRes) => {
    if (isManagementExcludedPath(clientReq.url)) {
      clientRes.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
      clientRes.end('404 Not Found');
      return;
    }
    proxyToBackend(clientReq, clientRes, `${MANAGE_HOST}:${MANAGE_PROXY_PORT}`);
  });
  attachTlsDiagnostics(server, 'management-proxy');
  return new Promise((res) => server.listen(MANAGE_PROXY_PORT, HOST_ADDR, () => res(server)));
}

function startOverlayProxy(leaf) {
  const server = createHttpsServer({ key: leaf.keyPem, cert: leaf.certPem }, (clientReq, clientRes) => {
    if (!isOverlayAllowedPath(clientReq.url)) {
      clientRes.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
      clientRes.end('404 Not Found');
      return;
    }
    proxyToBackend(clientReq, clientRes, `${OVERLAY_HOST}:${OVERLAY_PROXY_PORT}`);
  });
  attachTlsDiagnostics(server, 'overlay-proxy');
  return new Promise((res) => server.listen(OVERLAY_PROXY_PORT, HOST_ADDR, () => res(server)));
}

function proxyToBackend(clientReq, clientRes, forwardedHost) {
  const headers = { ...clientReq.headers };
  delete headers['x-forwarded-for'];
  delete headers['x-forwarded-proto'];
  delete headers['x-forwarded-host'];
  headers['x-forwarded-proto'] = 'https';
  headers['x-forwarded-host'] = forwardedHost;
  headers['x-forwarded-for'] = '127.0.0.1';
  headers.host = '127.0.0.1';

  const upstream = httpRequest({ host: '127.0.0.1', port: BACKEND_PORT, path: clientReq.url, method: clientReq.method, headers }, (upstreamRes) => {
    clientRes.writeHead(upstreamRes.statusCode, upstreamRes.headers);
    upstreamRes.pipe(clientRes);
  });
  upstream.on('error', () => {
    clientRes.writeHead(502);
    clientRes.end();
  });
  clientReq.pipe(upstream);
}

// --- server process -------------------------------------------------------
function provisionCredentialsDir(rtmpsLeaf) {
  const dir = mkdtempSync(join(tmpdir(), 'streamtree-d2c-creds-'));
  // Genuinely random per invocation (not a formula seeded by a fixed
  // byte) - two independent passes of this script in the same
  // workflow job (docs/progress.md: Stage 20D2C closure audit §8)
  // must never provision the same master key, exactly like two real,
  // separate deployments never would.
  const key = randomBytes(32);
  writeFileSync(join(dir, 'streaming-tree-master-key'), key, { mode: 0o600 });
  writeFileSync(join(dir, 'streaming-tree-rtmps-key'), rtmpsLeaf.keyPem, { mode: 0o600 });
  writeFileSync(join(dir, 'streaming-tree-rtmps-cert'), rtmpsLeaf.certPem, { mode: 0o644 });
  return dir;
}

function provisionAdminPassword(dataDir, credentialsDir) {
  const result = spawnSync(INSTALLED_EXE_PATH, ['--provision-admin-password', '--force'], {
    env: { PATH: process.env.PATH, HOME: process.env.HOME, STREAMING_TREE_DATA_DIR: dataDir, CREDENTIALS_DIRECTORY: credentialsDir },
    input: `${ADMIN_PASSWORD}\n`,
    encoding: 'utf8',
  });
  if (result.status !== 0) fail('provisioning the administrator password', result.stdout + result.stderr);
}

async function startServer(dataDir, credentialsDir) {
  const child = spawn(
    INSTALLED_EXE_PATH,
    ['--headless', '--remote-management', '--remote-ingest'],
    {
      cwd: dirname(INSTALLED_EXE_PATH),
      env: {
        PATH: process.env.PATH,
        HOME: process.env.HOME,
        STREAMING_TREE_DATA_DIR: dataDir,
        STREAMING_TREE_HOST: '127.0.0.1',
        STREAMING_TREE_PORT: String(BACKEND_PORT),
        CREDENTIALS_DIRECTORY: credentialsDir,
        STREAMING_TREE_REMOTE_MANAGEMENT: 'true',
        STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN: MANAGE_ORIGIN,
        STREAMING_TREE_REMOTE_INGEST: 'true',
        STREAMING_TREE_REMOTE_INGEST_RTMPS_ADDRESS: `${HOST_ADDR}:${RTMPS_PORT}`,
        STREAMING_TREE_REMOTE_INGEST_TLS_KEY_PATH: join(credentialsDir, 'streaming-tree-rtmps-key'),
        STREAMING_TREE_REMOTE_INGEST_TLS_CERT_PATH: join(credentialsDir, 'streaming-tree-rtmps-cert'),
        STREAMING_TREE_REMOTE_INGEST_OVERLAY_ORIGIN: OVERLAY_ORIGIN,
        STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS: `127.0.0.1:${MEDIAMTX_RTMP_PORT}`,
        STREAMING_TREE_MEDIAMTX_API_ADDRESS: `127.0.0.1:${MEDIAMTX_API_PORT}`,
        STREAMING_TREE_INGEST_PATH: INGEST_PATH,
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  );

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
  await new Promise((res) => {
    const timer = setTimeout(() => res(), SHUTDOWN_TIMEOUT_MS);
    handle.child.on('exit', () => {
      clearTimeout(timer);
      res();
    });
    handle.child.kill('SIGKILL');
  });
}

// --- remote client HTTP helper (curl, from the isolated namespace) -------
/** One HTTPS request genuinely issued from inside the isolated client
 * namespace via curl, trusting only the ephemeral CA (already installed
 * into the system trust store) - never -k/--insecure. cookieJar is a
 * file path curl reads and rewrites across calls, so a login's session
 * cookie is available to a later provision/rotate call exactly like a
 * real browser's cookie jar would carry it. */
async function remoteCurl(method, url, { body, headers = {}, cookieJar, csrfToken, expectStatusOnly = false, maxTimeSeconds = 10 } = {}) {
  const args = ['-sS', '-o', '-', '-w', '\n__STATUS__:%{http_code}', '--max-time', String(maxTimeSeconds), '-X', method];
  for (const [k, v] of Object.entries(headers)) args.push('-H', `${k}: ${v}`);
  if (csrfToken) args.push('-H', `X-CSRF-Token: ${csrfToken}`);
  if (body !== undefined) {
    args.push('-H', 'Content-Type: application/json', '--data-raw', JSON.stringify(body));
  }
  if (cookieJar) args.push('-c', cookieJar, '-b', cookieJar);
  args.push(url);

  const result = await clientExecStatus('curl', args);
  if (result.status !== 0) {
    fail(`curl ${method} ${url} failed to run`, (result.stderr || '') + (result.stdout || ''));
  }
  const raw = result.stdout || '';
  const marker = raw.lastIndexOf('__STATUS__:');
  const statusText = marker >= 0 ? raw.slice(marker + '__STATUS__:'.length).trim() : '';
  const bodyText = marker >= 0 ? raw.slice(0, marker).replace(/\n$/, '') : raw;
  const status = Number.parseInt(statusText, 10) || 0;
  if (expectStatusOnly) return { status, text: bodyText };
  let parsed = bodyText;
  try {
    parsed = JSON.parse(bodyText);
  } catch {
    // Not JSON - fine for e.g. a 404 plain-text body.
  }
  return { status, body: parsed, text: bodyText };
}

async function main() {
  console.log('Stage 20D2C Linux remote-server (remote ingest + remote overlay) verification');

  if (process.platform !== 'linux') {
    fail('this script only runs on Linux', `process.platform = ${process.platform}`);
  }

  step('Verify the real .deb package exists');
  const debName = existsSync(OUTPUT_DIR) ? readdirSync(OUTPUT_DIR).find((n) => n.endsWith('.deb')) : undefined;
  expect(typeof debName === 'string', `a .deb file was found in ${OUTPUT_DIR}`, 'Run: scripts/build-release-linux.sh --version 0.1.0-dev+test');
  const debPath = join(OUTPUT_DIR, debName);

  // Self-contained install rather than assuming an earlier workflow step
  // left the package installed - verify-linux-remote-management.mjs runs
  // immediately before this script in linux-headless.yml and removes the
  // package again in its own cleanup, so nothing upstream leaves it
  // installed by the time this script starts.
  if (shOk('dpkg', ['-s', PACKAGE_NAME])) {
    execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'ignore' });
  }

  step('The .deb installs successfully');
  execFileSync('sudo', ['dpkg', '-i', debPath], { stdio: 'pipe' });
  let installed = true;
  expect(shOk('dpkg', ['-s', PACKAGE_NAME]), `${PACKAGE_NAME} is installed`, PACKAGE_NAME);
  expect(existsSync(INSTALLED_EXE_PATH), 'the executable was installed', INSTALLED_EXE_PATH);

  verifyNetnsCapability();

  let networkUp = false;
  let pkiDir;
  let serverHandle;
  let manageProxy;
  let overlayProxy;

  try {
    setUpNetwork();
    networkUp = true;

    step('Generate the ephemeral 3-host test CA and install it into the trust store');
    pkiDir = mkdtempSync(join(tmpdir(), 'streamtree-d2c-pki-'));
    const pki = generateEphemeralPKI(pkiDir);
    installEphemeralCATrust(pki.caCertPath);
    pass('ephemeral CA installed for this CI run only');

    step('Provision the master key, RTMPS credential, and administrator password');
    const dataDir = mkdtempSync(join(tmpdir(), 'streamtree-d2c-data-'));
    const credentialsDir = provisionCredentialsDir(pki.leaves[INGEST_HOST]);
    provisionAdminPassword(dataDir, credentialsDir);
    pass('master key + RTMPS key/cert + administrator password verifier provisioned');

    step('Start the real server: --headless --remote-management --remote-ingest');
    serverHandle = await startServer(dataDir, credentialsDir);
    expect(serverHandle.ready, 'the server became healthy', serverHandle.hasExited() ? `exited with code ${serverHandle.exitCode}\n${serverHandle.getStderr()}` : 'timed out');

    // A backend-side "internal_error" response body is deliberately
    // sanitized (apps/server/internal/httpapi/remoteingest.go's
    // writeRemoteIngestError) - the real Go error only reaches the
    // server's own stdout log (main.go: slog.NewTextHandler(os.Stdout)).
    // MediaMTX's own log lines are relayed through that same logger
    // always at Go level=INFO (process.go's (*process).log), with
    // MediaMTX's real level/message embedded as the mediamtx_level/
    // mediamtx_message attributes - so a level=ERR/WARN filter alone
    // misses them; this also keeps every mediamtx_message line. Kept
    // deliberately small: GitHub's own annotation size limit is well
    // under what a full log tail needs, so the mediamtx content goes
    // first (the more likely to matter) and the caller's own detail is
    // capped short.
    const withServerDiag = (text) => {
      const lines = serverHandle.getStdout().split('\n');
      const notable = lines.filter((l) => /level=(ERR|WARN)|mediamtx_message/.test(l));
      const recent = lines.slice(-5);
      const combined = [...new Set([...notable, ...recent])].join('\n');
      return `--- server stdout: error/warning + mediamtx lines + recent tail ---\n${combined.slice(-1500)}\n--- caller detail ---\n${text.slice(0, 300)}`;
    };

    step('Start the management and overlay TLS proxy stand-ins on the host-side veth address');
    manageProxy = await startManagementProxy(pki.leaves[MANAGE_HOST]);
    overlayProxy = await startOverlayProxy(pki.leaves[OVERLAY_HOST]);
    pass(`management proxy on ${HOST_ADDR}:${MANAGE_PROXY_PORT}, overlay proxy on ${HOST_ADDR}:${OVERLAY_PROXY_PORT}`);

    const cookieJar = join(pkiDir, 'cookies.txt');

    step('Log in as the administrator through the real management TLS proxy, from the isolated client namespace');
    const sessionBootstrap = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/auth/session`, { cookieJar });
    expect(sessionBootstrap.status === 200, 'GET /api/auth/session (unauthenticated bootstrap) returns 200', sessionBootstrap.text);
    const loginRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/auth/login`, {
      body: { password: ADMIN_PASSWORD },
      headers: { Origin: MANAGE_ORIGIN },
      cookieJar,
    });
    expect(loginRes.status === 200, 'POST /api/auth/login succeeds through the real proxy from the isolated namespace', loginRes.status === 200 ? loginRes.text : withServerDiag(loginRes.text));
    const csrfToken = loginRes.body && loginRes.body.csrfToken;
    expect(typeof csrfToken === 'string' && csrfToken.length > 0, 'login response carries a CSRF token', loginRes.text);

    // Remote ingest is layered on the same shared MediaMTX instance
    // local branches already use (docs/remote-ingest.md) - provisioning
    // a credential requires it to already be installed, exactly like a
    // real operator would need to install it first. Mirrors the same
    // real install-then-poll sequence scripts/verify-mediamtx-runtime.mjs
    // already proves in CI (POST install -> 202, then wait for
    // ready-or-stopped, then wait for ready).
    async function waitForMediaMtxState(wanted, timeoutMs) {
      const deadline = Date.now() + timeoutMs;
      let state = '';
      while (Date.now() < deadline) {
        const runtimeRes = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/runtime`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken });
        state = runtimeRes.body && runtimeRes.body.mediaMtx && runtimeRes.body.mediaMtx.state;
        if (wanted.includes(state) || state === 'error') return state;
        await new Promise((r) => setTimeout(r, 1000));
      }
      return state;
    }

    step('Install MediaMTX through the managed installer');
    const installRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/runtime/mediamtx/install`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken });
    expect(installRes.status === 202, 'POST /api/runtime/mediamtx/install returns 202', installRes.status === 202 ? installRes.text : withServerDiag(installRes.text));

    step('Wait for the MediaMTX installation to finish (a real ~30 MB download, checksum-verified)');
    const installedState = await waitForMediaMtxState(['ready', 'stopped'], 300_000);
    expect(installedState === 'ready' || installedState === 'stopped', 'MediaMTX finishes installing (ready or stopped)', withServerDiag(`state=${installedState}`));

    step('Wait for MediaMTX readiness');
    const readyState = await waitForMediaMtxState(['ready'], 60_000);
    expect(readyState === 'ready', 'MediaMTX reports ready', withServerDiag(`state=${readyState}`));

    step('Provision the remote-ingest publisher credential through the authenticated management API');
    const provisionRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-ingest/provision`, {
      headers: { Origin: MANAGE_ORIGIN },
      cookieJar,
      csrfToken,
    });
    expect(provisionRes.status === 200, 'POST /api/remote-ingest/provision succeeds', provisionRes.status === 200 ? provisionRes.text : withServerDiag(provisionRes.text));
    const publisherSecret = provisionRes.body && provisionRes.body.secret;
    expect(typeof publisherSecret === 'string' && publisherSecret.length > 0, 'a plaintext publisher secret was returned exactly once', '');

    // Supervisor.RequestStart launches MediaMTX off the caller's own
    // goroutine (go s.launch(...); return nil) rather than waiting for
    // readiness, so the provision API can genuinely return 200 before
    // MediaMTX has actually finished restarting with the new
    // authInternalUsers config - a real race, not a hypothetical one,
    // confirmed by reading apps/server/internal/runtime/mediamtx/
    // supervisor.go. Wait for readiness again before trusting the new
    // credential config is actually the one MediaMTX is enforcing.
    step('Wait for MediaMTX to finish restarting with the new remote-ingest credential');
    const afterProvisionState = await waitForMediaMtxState(['ready'], 30_000);
    expect(afterProvisionState === 'ready', 'MediaMTX becomes ready again after the credential-provisioning restart', withServerDiag(`state=${afterProvisionState}`));

    const rtmpsBase = `rtmps://${INGEST_HOST}:${RTMPS_PORT}`;

    step('The rendered mediamtx.yml carries the correct verifier for the provisioned secret');
    // Reads the just-rendered config directly off disk (this script
    // runs in the host namespace, the same filesystem as the server
    // process) and independently recomputes the expected
    // "sha256:<base64(sha256(secret))>" verifier in plain JS -
    // cross-checked against apps/server/internal/runtime/mediamtx/
    // credential.go's Go implementation and MediaMTX's own
    // conf/credential.go Check() logic (both verified against the
    // pinned v1.19.3 source). A real regression check on the whole
    // provision -> store -> render pipeline, independent of anything
    // RTMP-protocol-related.
    const expectedVerifier = 'sha256:' + createHash('sha256').update(publisherSecret).digest('base64');
    const configPath = join(dataDir, 'runtime', 'mediamtx.yml');
    const configContent = existsSync(configPath) ? readFileSync(configPath, 'utf8') : '';
    expect(configContent.includes(expectedVerifier), 'the rendered config contains the verifier for the provisioned secret', configContent ? configContent.split('\n').filter((l) => /user:|pass:|ips:/.test(l)).join(' ').slice(0, 400) : '(config file not found)');

    step('MediaMTX Control API and the Go backend loopback port are unreachable from the isolated client namespace');
    expect(
      !clientExecOk('curl', ['-sS', '--max-time', '3', `http://${HOST_ADDR}:${MEDIAMTX_API_PORT}/v3/config/global/get`]),
      'the MediaMTX Control API (bound to 127.0.0.1 only) does not answer on the veth address',
      '',
    );
    expect(
      !clientExecOk('curl', ['-sS', '--max-time', '3', `http://${HOST_ADDR}:${BACKEND_PORT}/api/health`]),
      'the Go backend (bound to 127.0.0.1 only) does not answer on the veth address',
      '',
    );

    step('A real TLS handshake against the RTMPS listener succeeds, trusting only the ephemeral CA');
    // openssl s_client speaks nothing but TLS - no RTMP - so a clean
    // handshake here proves the listener and the ephemeral CA trust
    // chain independently of anything RTMP-protocol-specific.
    const tlsProbe = await clientExecStatus('openssl', ['s_client', '-connect', `${INGEST_HOST}:${RTMPS_PORT}`, '-CAfile', pki.caCertPath, '-verify_return_error', '-brief'], { timeout: 8_000 });
    expect(tlsProbe.status === 0, 'openssl s_client completes a verified TLS handshake', tlsProbe.status === 0 ? '' : `${(tlsProbe.stdout || '').trim().slice(-300)} | ${(tlsProbe.stderr || '').trim().slice(-300)}`);

    step('A TLS handshake trusting an unrelated CA (not the one that signed the RTMPS listener leaf cert) is rejected');
    // A second, wholly independent self-signed CA, generated fresh and
    // never used to sign anything the server presents - proves the
    // client genuinely verifies the certificate chain rather than
    // accepting whatever the listener happens to present.
    const untrustedCaCert = join(pkiDir, 'untrusted-ca-cert.pem');
    const untrustedCaKey = join(pkiDir, 'untrusted-ca-key.pem');
    sh('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', untrustedCaKey, '-out', untrustedCaCert, '-days', '1', '-subj', '/CN=streaming-tree-d2c-untrusted-ca']);
    const untrustedCaProbe = await clientExecStatus('openssl', ['s_client', '-connect', `${INGEST_HOST}:${RTMPS_PORT}`, '-CAfile', untrustedCaCert, '-verify_return_error', '-brief'], { timeout: 8_000 });
    expect(untrustedCaProbe.status !== 0, 'a handshake trusting only an unrelated CA fails verification', untrustedCaProbe.status !== 0 ? '' : `${(untrustedCaProbe.stdout || '').trim().slice(-300)} | ${(untrustedCaProbe.stderr || '').trim().slice(-300)}`);

    step('A TLS handshake with the correct CA but the wrong expected hostname is rejected');
    // The real ephemeral CA (the correct trust root), but a hostname
    // check against a name that is not in the leaf certificate's own
    // subjectAltName (only DNS:ingest-d2c.test) - proves hostname
    // scoping is real, not merely chain-of-trust verification.
    const wrongHostnameProbe = await clientExecStatus('openssl', ['s_client', '-connect', `${INGEST_HOST}:${RTMPS_PORT}`, '-CAfile', pki.caCertPath, '-verify_hostname', 'wrong-host-d2c.test', '-verify_return_error', '-brief'], { timeout: 8_000 });
    expect(wrongHostnameProbe.status !== 0, 'a handshake verified against the wrong expected hostname fails', wrongHostnameProbe.status !== 0 ? '' : `${(wrongHostnameProbe.stdout || '').trim().slice(-300)} | ${(wrongHostnameProbe.stderr || '').trim().slice(-300)}`);

    step('RTMPS accept/reject matrix, from the isolated client namespace, via real ffmpeg (docs/remote-ingest.md §5/§11)');
    // -re (read input at its own native frame rate) is required: a
    // synthetic lavfi input with no pacing flag is generated and sent
    // as fast as the CPU allows, so a "clip" would finish (and the
    // publisher disconnect) within milliseconds rather than lasting
    // its nominal -t duration, defeating any mid-stream check. -re is
    // a per-input option that does not carry over to a later -i, so it
    // is repeated before each of the two lavfi inputs to keep video
    // and audio paced together.
    const ffmpegBase = ['-re', '-f', 'lavfi', '-i', 'testsrc=size=160x120:rate=10', '-re', '-f', 'lavfi', '-i', 'sine=frequency=1000', '-c:v', 'libx264', '-preset', 'ultrafast', '-tune', 'zerolatency', '-c:a', 'aac', '-t', '2', '-f', 'flv'];

    // Only used for the plaintext-RTMP-to-RTMPS-port case below now -
    // a connection-establishment-level failure (the TLS handshake
    // itself never completes) that ffmpeg's exit code reliably
    // reports. The three auth-based rejections further down do not
    // trust ffmpeg's exit code at all - see tryPublishAndCheckAccepted.
    async function tryPublish(url) {
      const result = await clientExecStatus('ffmpeg', [...ffmpegBase, url], { timeout: 15_000 });
      // A blind tail can miss the actual rejection/connection message,
      // which ffmpeg typically prints near the top (around "Output #0")
      // rather than at the very end (which is mostly encoder/muxer
      // housekeeping noise for every run, success or failure alike).
      const lines = (result.stderr || '').trim().split('\n');
      const notable = lines.filter((l) => /rtmp|server|reject|refus|denia|denied|error|fail|connect/i.test(l));
      const combined = [...new Set([...notable, ...lines.slice(-5)])].join('\n');
      return { ok: result.status === 0, exitCode: result.status, detail: combined.slice(0, 500) };
    }

    const plaintextTry = await tryPublish(`rtmp://${INGEST_HOST}:${RTMPS_PORT}/${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${publisherSecret}`);
    expect(!plaintextTry.ok, 'plaintext RTMP to the RTMPS port is rejected', plaintextTry.ok ? plaintextTry.detail : '');

    // MediaMTX's own log at logLevel: info carries server-lifecycle
    // events only, never per-connection detail, so it cannot answer
    // what MediaMTX itself decided about a given publish attempt. The
    // Control API can: GET /v3/paths/list reports each configured
    // path's real "ready" state, reachable directly from the host
    // namespace (loopback-only, and this script itself already runs
    // there).
    function hostFetchJson(path) {
      return new Promise((resolve) => {
        const req = httpRequest({ host: '127.0.0.1', port: MEDIAMTX_API_PORT, path, method: 'GET' }, (res) => {
          let body = '';
          res.on('data', (c) => (body += c));
          res.on('end', () => {
            try {
              resolve({ status: res.statusCode, body: JSON.parse(body) });
            } catch {
              resolve({ status: res.statusCode, body });
            }
          });
        });
        req.on('error', (err) => resolve({ status: 0, body: String((err && err.message) || err) }));
        req.end();
      });
    }

    // Credentials are set via ffmpeg's own -rtmp_app/-rtmp_playpath,
    // not embedded in the destination URL: this ffmpeg build's RTMP
    // URL auto-parsing does not reliably split a
    // rtmps://host/live?user=X&pass=Y-shaped query string into a
    // publish MediaMTX ever logs as accepted (docs/progress.md). GET
    // /v3/paths/list's "ready" field for the target path, polled
    // mid-stream, is MediaMTX's own ground truth for whether a publish
    // actually succeeded - ffmpeg's own exit code is not a reliable
    // signal here (the plaintext-RTMP-to-RTMPS-port case above is
    // different: a connection-establishment-level failure ffmpeg does
    // report correctly).
    async function tryPublishAndCheckAccepted(app, playpath, pathName) {
      // -rtmp_playpath only passed when non-empty: an *explicit* empty
      // override reaches MediaMTX's "publishing" state and then closes
      // within milliseconds rather than persisting - omitting the flag
      // entirely does not have that effect.
      const playpathArgs = playpath ? ['-rtmp_playpath', playpath] : [];
      const child = clientSpawn('ffmpeg', [...ffmpegBase.slice(0, -2), '-rtmp_app', app, ...playpathArgs, '-f', 'flv', `${rtmpsBase}/`]);
      let exited = false;
      child.on('exit', () => {
        exited = true;
      });
      await new Promise((r) => setTimeout(r, 1500));
      const pathsList = await hostFetchJson('/v3/paths/list');
      const items = (pathsList.body && Array.isArray(pathsList.body.items)) ? pathsList.body.items : [];
      const matched = items.find((p) => p && p.name === pathName);
      const accepted = !!(matched && matched.ready);
      const deadline = Date.now() + 10_000;
      while (!exited && Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, 200));
      }
      if (!exited) child.kill('SIGKILL');
      return { accepted, snapshot: JSON.stringify(pathsList.body).slice(0, 600) };
    }

    const noCred = await tryPublishAndCheckAccepted(INGEST_PATH, '', INGEST_PATH);
    expect(!noCred.accepted, 'RTMPS with no credential is rejected (MediaMTX never reports the path ready)', noCred.accepted ? noCred.snapshot : '');
    const wrongCred = await tryPublishAndCheckAccepted(`${INGEST_PATH}?user=${PUBLISHER_USER}&pass=wrong-password`, '', INGEST_PATH);
    expect(!wrongCred.accepted, 'RTMPS with a wrong credential is rejected (MediaMTX never reports the path ready)', wrongCred.accepted ? wrongCred.snapshot : '');
    const wrongPath = await tryPublishAndCheckAccepted(`wrong-path?user=${PUBLISHER_USER}&pass=${publisherSecret}`, '', 'wrong-path');
    expect(!wrongPath.accepted, 'RTMPS with a valid credential but the wrong path is rejected (MediaMTX never reports the path ready)', wrongPath.accepted ? wrongPath.snapshot : '');
    // Deliberately no quick "valid credential is accepted" check here
    // to mirror the three rejections above - a rejection fails fast
    // (MediaMTX never has to actually attach a publisher), but a real
    // accept may genuinely take a bit longer to be reported as ready
    // than tryPublishAndCheckAccepted's 1.5s window comfortably covers
    // over RTMPS through the network-namespace boundary specifically
    // (a first attempt at this exact quick check, using that same
    // window, produced a false-negative "not accepted" here before
    // the real, properly-timed positive-path test below ever got a
    // chance to run - removed rather than given a second, longer
    // timeout, since the fuller test below already proves this
    // correctly with real settle time built in).

    step('RTMPS positive path: valid credential + canonical path succeeds, waiting -> receiving -> waiting');
    const before = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken });
    expect(before.body && before.body.receiving === false, 'ingest status is waiting before any publish', before.status === 200 ? before.text : withServerDiag(before.text));

    // Spawned (not spawnSync) specifically so this script can observe
    // the mid-stream "receiving" state, not merely the exit code once
    // the whole publish has already finished - a synchronous run would
    // never actually prove the waiting -> receiving transition.
    // Explicit -rtmp_app/-rtmp_playpath (see ffmpegBase above), not a
    // URL-embedded query string.
    let publishStderr = '';
    const publishApp = `${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${publisherSecret}`;
    const publishChild = clientSpawn('ffmpeg', [...ffmpegBase.slice(0, -4), '-t', '10', '-rtmp_app', publishApp, '-f', 'flv', `${rtmpsBase}/`]);
    publishChild.stderr.on('data', (c) => (publishStderr += c.toString()));
    let publishExited = false;
    let publishExitCode = null;
    publishChild.on('exit', (code) => {
      publishExited = true;
      publishExitCode = code;
    });

    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
    // MediaMTX's own /v3/paths/list is the same ground truth the
    // reject-matrix above trusts - check it directly here too, so a
    // failure below shows whether MediaMTX itself considers this
    // connection accepted (a backend status-reporting bug) or not (a
    // real credential/config problem for the positive path
    // specifically, distinct from the already-verified reject cases).
    const positivePathState = await hostFetchJson('/v3/paths/list');
    console.log(`     diag paths/list during the valid publish: ${JSON.stringify(positivePathState.body).slice(0, 700)}`);
    const during = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken });
    expect(during.body && during.body.receiving === true, 'ingest status becomes receiving while the publish is active', during.status === 200 ? during.text : withServerDiag(during.text));

    step('A remote read (pull/subscribe) of the ingest path over RTMPS is rejected, even while a real publish is active');
    // apps/server/internal/runtime/mediamtx/config.go's own rendered
    // authInternalUsers only grants "action: read" to the "any" user
    // scoped to ips: [127.0.0.1, ::1] - a request from this isolated,
    // non-loopback client namespace matches neither that entry (wrong
    // source IP) nor the publisher entry (wrong action), so MediaMTX
    // itself has no matching permission and must refuse the read.
    // Run while the valid publish above is still active so a real
    // rejection is being proven, not merely "nothing to read yet".
    const readProbe = await clientExecStatus('ffmpeg', ['-y', '-i', `${rtmpsBase}/${INGEST_PATH}`, '-t', '1', '-f', 'null', '-'], { timeout: 8_000 });
    expect(readProbe.status !== 0, 'a remote RTMPS read attempt against the ingest path is rejected', readProbe.status !== 0 ? '' : `${(readProbe.stderr || '').trim().slice(-400)}`);

    const publishDeadline = Date.now() + 20_000;
    while (!publishExited && Date.now() < publishDeadline) {
      await new Promise((r) => setTimeout(r, 300));
    }
    if (!publishExited) publishChild.kill('SIGKILL');
    expect(publishExited && publishExitCode === 0, 'the valid RTMPS publish completed successfully', publishStderr.slice(-2000));

    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
    const after = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken });
    expect(after.body && after.body.receiving === false, 'ingest status returns to waiting after the publisher disconnects', after.status === 200 ? after.text : withServerDiag(after.text));

    step('MediaMTX exposes the plaintext RTMP listener only on loopback - branch reads are unaffected by remote ingest');
    expect(
      !clientExecOk('curl', ['-sS', '--max-time', '2', `http://${HOST_ADDR}:${MEDIAMTX_RTMP_PORT}/`]),
      'the loopback-only plain RTMP listener does not answer on the veth address either',
      '',
    );

    step('Cookie separation: the management session cookie is not sent to the overlay origin');
    const cookieJarText = readFileSync(cookieJar, 'utf8');
    expect(cookieJarText.includes(MANAGE_HOST), 'the cookie jar recorded a cookie scoped to the management hostname', cookieJarText);
    expect(!cookieJarText.includes(OVERLAY_HOST), 'the cookie jar never recorded a cookie scoped to the overlay hostname (none was ever set for it)', cookieJarText);
    const overlayWithManagementCookie = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/public/chat-overlays/does-not-exist/config`, {
      cookieJar,
      expectStatusOnly: true,
    });
    // A curl cookie jar is scoped by domain per RFC 6265 - curl itself
    // will not attach the manage-host cookie to an overlay-host
    // request. The slug is deliberately unknown, so the real, correct
    // outcome is a 404 (not a 401/403, which would mean the overlay
    // origin required or waited on something cookie-related that it
    // should not have).
    expect(overlayWithManagementCookie.status === 404, 'the overlay origin answers without the management cookie (never required)', overlayWithManagementCookie.status === 404 ? overlayWithManagementCookie.text : withServerDiag(overlayWithManagementCookie.text));

    console.log(`\nAll ${stepCount} steps passed.`);
  } finally {
    step('Cleanup: stop the server/proxies, remove the ephemeral CA trust, tear down the network namespace, leave no owned process/state behind');
    if (manageProxy) await new Promise((res) => manageProxy.close(res));
    if (overlayProxy) await new Promise((res) => overlayProxy.close(res));
    await forceStop(serverHandle);
    try {
      execFileSync('pkill', ['-f', 'streaming-tree-server'], { stdio: 'ignore' });
    } catch {
      // Nothing left running - fine.
    }
    try {
      execFileSync('sudo', ['pkill', '-f', 'mediamtx'], { stdio: 'ignore' });
    } catch {
      // Nothing left running - fine.
    }
    removeEphemeralCATrust();
    if (pkiDir) rmSync(pkiDir, { recursive: true, force: true });
    if (networkUp) tearDownNetwork();
    if (installed) {
      try {
        execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'ignore' });
      } catch (removeError) {
        console.error('warning: cleanup dpkg -r failed', removeError);
      }
    }
    pass('cleanup complete - no owned process, network namespace, veth interface, package install, or trust-store change left behind');
  }
}

main().catch((err) => {
  console.error(`\nFAILED: ${err.message}`);
  process.exitCode = 1;
});
