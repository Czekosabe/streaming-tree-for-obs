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
 * PRE-20E.1: the server itself is now the REAL package-owned systemd
 * service (docs/remote-ingest.md §17/§19), not a directly spawned test
 * binary - real `.deb` install, real LoadCredential= master key/RTMPS
 * cert/key delivery, a real `systemctl edit`-equivalent drop-in layering
 * remote-management/remote-ingest on top of the shipped, disabled-by-
 * default unit, real `daemon-reload`/`enable --now`/`restart`/
 * `disable --now`. Every scenario below - RTMPS, credential lifecycle,
 * the destination-branch-to-sink E2E, the remote-overlay E2E matrix -
 * runs against this one real systemd-managed instance.
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
 *     authenticated API, plus the full lifecycle beyond first
 *     provisioning: rotate (old credential rejected, new accepted);
 *     the rotated credential survives a real service restart against
 *     the same data directory; revoke (nothing can authenticate
 *     afterward); rotate-while-receiving and revoke-while-receiving
 *     both refused with 409, each proven to mutate nothing by letting
 *     the in-flight publish that was using the live credential run to
 *     a clean completion afterward;
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
 *     is never sent to the overlay origin;
 *   - the real package-owned systemd service lifecycle: install alone
 *     enables/starts/exposes nothing; D2A hardening (DynamicUser,
 *     NoNewPrivileges, ProtectSystem=strict, CapabilityBoundingSet=,
 *     ...) survives on both the main process and its MediaMTX child,
 *     neither running as root; a real `systemctl restart` (not a
 *     directly-killed/respawned process) preserves rotated credentials;
 *     package removal leaves no owned process, drop-in, or state
 *     directory behind;
 *   - a real destination-branch E2E: the EXISTING production branch
 *     manager, started explicitly against a seeded platform pointed at
 *     a real local sink MediaMTX instance (never Twitch/YouTube/Kick/
 *     TikTok, never a real credential) - reaches live, the sink proves
 *     real decoded media tracks arrived (not merely an accepted
 *     connection), the branch's own FFmpeg command is structurally
 *     proven never to reference the remote publisher credential (a log
 *     scan), publisher disconnect drives the real waiting_for_ingest
 *     policy without losing desired-running state, and reconnect proves
 *     the existing auto-resume contract - no invented behavior;
 *   - the remote-overlay E2E matrix across every real product family
 *     that exists: chat overlay and audio (hard-404 on an unresolved
 *     slug/token) plus alert profiles, Stage 18A goal widgets, Stage 18B
 *     supporter widgets, and dashboards (all four sharing one safe-
 *     default-on-miss behavior, and the latter three all sharing the
 *     identical remoteoverlay.DomainWidget capability key, proven, not
 *     assumed) - local-slug rejection, valid-capability success,
 *     rotate, disable, and management-host exclusion, each proven
 *     through the real remote HTTPS overlay boundary from the isolated
 *     client; plus a real managed visual asset, reachable through the
 *     overlay origin by its own independent capability token.
 *
 * .github/workflows/linux-headless.yml invokes this script twice per
 * architecture ("pass 1"/"pass 2"), and every piece of per-run state
 * this script itself controls is generated fresh on each invocation -
 * a new random 32-byte master key (node:crypto randomBytes, not a
 * fixed formula), a new ephemeral CA and leaf certificates, RTMPS
 * credential files, and systemd drop-in, a freshly reinstalled .deb,
 * a freshly cleared StateDirectory, and a freshly installed MediaMTX
 * instance under it. The remote-ingest publisher secret itself is
 * generated server-side (POST /api/remote-ingest/provision),
 * independently of this script, on every run. The network-namespace/
 * veth pair use the same fixed names both passes, but the kernel
 * objects themselves are deleted in this script's own cleanup and
 * recreated from scratch on the next invocation - never left up and
 * reused live.
 *
 * It does NOT yet cover: the real TTS-sourced bytesUrl-echoes-
 * presented-token property for the audio overlay family specifically -
 * no supported TTS provider exists on Linux (docs/linux-headless-
 * server.md's own already-established finding), so the real production
 * binary can never emit a real bytesUrl in this environment; that
 * specific property remains proven at the Go unit level only
 * (TestRemoteOverlayAudioBytesURLEchoesThePresentedTokenNotTheLocalSlug).
 * Everything else about the audio family (SSE stream connection,
 * legacy-slug/rotate/disable on the bytes and ack routes) is proven
 * natively above.
 *
 * Usage:  node scripts/verify-linux-remote-server.mjs
 * Exits non-zero on the first failed expectation.
 */

import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createServer as createHttpsServer } from 'node:https';
import { request as httpRequest } from 'node:http';
import { tmpdir, userInfo } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release-linux', 'output');
const PACKAGE_NAME = 'streaming-tree-for-obs';
const INSTALLED_EXE_PATH = '/usr/bin/streaming-tree-server';

// --- real systemd lifecycle (PRE-20E.1 gap C) ---------------------------
// This script drives the REAL packaged systemd service
// (scripts/systemd/streaming-tree.service, installed at UNIT_PATH by the
// .deb) for every scenario below - RTMPS, credential lifecycle, branch-
// to-sink, remote overlay - never a directly-spawned test binary, mirroring
// verify-linux-headless.mjs's own already-proven real-PID-1-systemd
// lifecycle (daemon-reload -> enable --now -> is-active -> disable --now).
// The package-owned unit ships every D2A hardening directive and
// STREAMING_TREE_REMOTE_MANAGEMENT/STREAMING_TREE_REMOTE_INGEST both
// disabled by default (docs/remote-ingest.md §19) - this script layers
// them on via a systemd drop-in, exactly like a real operator would with
// `systemctl edit`, never by editing the package-owned unit file.
const UNIT_NAME = 'streaming-tree.service';
const UNIT_PATH = '/lib/systemd/system/streaming-tree.service';
const PROVISION_MASTER_KEY_HELPER = '/usr/share/streaming-tree/provision-headless-master-key.sh';
const DROPIN_DIR = '/etc/systemd/system/streaming-tree.service.d';
const DROPIN_PATH = `${DROPIN_DIR}/d2c-verify.conf`;
const ETC_DIR = '/etc/streaming-tree';
const MASTER_KEY_PATH = `${ETC_DIR}/master.key`;
const RTMPS_KEY_PATH = `${ETC_DIR}/rtmps.key`;
const RTMPS_CERT_PATH = `${ETC_DIR}/rtmps.crt`;
// The real StateDirectory= path (%S/streaming-tree, unconditionally set
// by the shipped unit itself) - fixed, not a per-run mkdtemp directory,
// exactly like a real deployment's own persistent state.
const STATE_DIR = '/var/lib/streaming-tree';

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
const SINK_RTMP_PORT = 8716; // real destination-branch E2E (PRE-20E.1 gap A) - a local fake sink, never a real provider
const SINK_API_PORT = 8717;

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

// --- real systemd server lifecycle -----------------------------------------
// Every helper below drives the REAL package-owned unit - never a directly
// spawned child process - so ActiveState, journald output, LoadCredential=,
// DynamicUser=, and every other D2A hardening directive are all the real
// ones a real operator's deployment would get, not a test-only stand-in.

function provisionMasterKey() {
  sh('sudo', ['mkdir', '-p', ETC_DIR]);
  // --force: this same pass (and the second of the two independent
  // passes this script always runs) may provision more than once
  // against a runner that already carries a key from an earlier
  // invocation in the same job - expected here, not a real operator's
  // key the helper's own default refusal exists to protect.
  sh('sudo', ['bash', PROVISION_MASTER_KEY_HELPER, '--force', MASTER_KEY_PATH]);
}

/** Delivers the RTMPS leaf cert/key to the real systemd credential
 * mechanism's own operator-chosen path (docs/remote-ingest.md §19),
 * root-owned, 0600 for the key - never inside a package-owned
 * directory, exactly like a real operator's own provisioning. */
function deliverRtmpsCredentialFiles(rtmpsLeaf) {
  const tmpKey = join(tmpdir(), `d2c-rtmps-key-${randomBytes(6).toString('hex')}`);
  const tmpCert = join(tmpdir(), `d2c-rtmps-cert-${randomBytes(6).toString('hex')}`);
  writeFileSync(tmpKey, rtmpsLeaf.keyPem, { mode: 0o600 });
  writeFileSync(tmpCert, rtmpsLeaf.certPem, { mode: 0o644 });
  sh('sudo', ['install', '-o', 'root', '-g', 'root', '-m', '0600', tmpKey, RTMPS_KEY_PATH]);
  sh('sudo', ['install', '-o', 'root', '-g', 'root', '-m', '0644', tmpCert, RTMPS_CERT_PATH]);
  rmSync(tmpKey, { force: true });
  rmSync(tmpCert, { force: true });
}

/** Provisions the administrator password under the exact real service
 * identity, via systemd-run (docs/remote-ingest.md §19,
 * scripts/provision-admin-password.sh's own established mechanism) -
 * never against a second, parallel identity/state path. --pipe (not
 * --pty): stdin is not a terminal, so the real binary's own
 * readProvisioningPassword() reads exactly one line, no confirmation
 * round trip - the same non-interactive contract this script already
 * relied on before this change.
 *
 * A literal STATE_DIR (/var/lib/streaming-tree), not the %S specifier
 * the real unit *file* uses - real CI evidence (a genuine `mkdir /%S:
 * read-only file system` failure) proved %S/%d-style specifiers are
 * only expanded when systemd parses a property from a real unit
 * file's own [Service] section, never when the identical text is set
 * via `systemd-run --property=` on a transient unit. The same real
 * bug and fix landed in scripts/provision-admin-password.sh itself
 * (docs/progress.md, PRE-20E.1) - this mirrors that fix exactly. */
function provisionAdminPasswordViaRealIdentity() {
  const result = spawnSync(
    'sudo',
    [
      'systemd-run', '--pipe', '--collect', '--wait',
      `--property=LoadCredential=streaming-tree-master-key:${MASTER_KEY_PATH}`,
      '--property=DynamicUser=yes',
      '--property=StateDirectory=streaming-tree',
      `--property=Environment=STREAMING_TREE_DATA_DIR=${STATE_DIR}`,
      '--', INSTALLED_EXE_PATH, '--provision-admin-password', '--force',
    ],
    { input: `${ADMIN_PASSWORD}\n`, encoding: 'utf8' },
  );
  if (result.status !== 0) fail('provisioning the administrator password via the real service identity (systemd-run)', (result.stdout || '') + (result.stderr || ''));
}

/** Writes the systemd drop-in layering remote-management/remote-ingest
 * on top of the package-owned unit - `systemctl edit`-equivalent, never
 * an edit of the shipped unit file itself (docs/remote-ingest.md §19). */
function writeRemoteServerDropIn() {
  const lines = [
    '[Service]',
    `LoadCredential=streaming-tree-rtmps-key:${RTMPS_KEY_PATH}`,
    `LoadCredential=streaming-tree-rtmps-cert:${RTMPS_CERT_PATH}`,
    `Environment=STREAMING_TREE_PORT=${BACKEND_PORT}`,
    'Environment=STREAMING_TREE_REMOTE_MANAGEMENT=true',
    `Environment=STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN=${MANAGE_ORIGIN}`,
    'Environment=STREAMING_TREE_REMOTE_INGEST=true',
    `Environment=STREAMING_TREE_REMOTE_INGEST_RTMPS_ADDRESS=${HOST_ADDR}:${RTMPS_PORT}`,
    'Environment=STREAMING_TREE_REMOTE_INGEST_TLS_KEY_PATH=%d/streaming-tree-rtmps-key',
    'Environment=STREAMING_TREE_REMOTE_INGEST_TLS_CERT_PATH=%d/streaming-tree-rtmps-cert',
    `Environment=STREAMING_TREE_REMOTE_INGEST_OVERLAY_ORIGIN=${OVERLAY_ORIGIN}`,
    `Environment=STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS=127.0.0.1:${MEDIAMTX_RTMP_PORT}`,
    `Environment=STREAMING_TREE_MEDIAMTX_API_ADDRESS=127.0.0.1:${MEDIAMTX_API_PORT}`,
    `Environment=STREAMING_TREE_INGEST_PATH=${INGEST_PATH}`,
    '',
  ].join('\n');
  const tmpDropIn = join(tmpdir(), `d2c-dropin-${randomBytes(6).toString('hex')}.conf`);
  writeFileSync(tmpDropIn, lines);
  sh('sudo', ['mkdir', '-p', DROPIN_DIR]);
  sh('sudo', ['install', '-o', 'root', '-g', 'root', '-m', '0644', tmpDropIn, DROPIN_PATH]);
  rmSync(tmpDropIn, { force: true });
}

/** Reads a root-owned file through sudo - StateDirectory content is not
 * readable by this script's own unprivileged invoking user once
 * DynamicUser has taken ownership of it. */
function readRootFile(path) {
  try {
    // A bounded timeout, not an unbounded execFileSync call - two real
    // CI runs (docs/progress.md, PRE-20E.1) died silently with no
    // catchable JS error at all, consistent with a synchronous child
    // process hanging forever with nothing to time it out. A timeout
    // here turns a possible hang into a diagnosable failure instead of
    // silence, whatever the real underlying cause turns out to be.
    return execFileSync('sudo', ['cat', path], { encoding: 'utf8', timeout: 10_000 });
  } catch {
    return '';
  }
}

/** Tails the real unit's own journald output - the only diagnostic
 * source once the service is systemd-managed rather than a direct
 * child process this script itself holds a stdout/stderr pipe to. */
function serverJournal(lines = 400) {
  try {
    return execFileSync('sudo', ['journalctl', '-u', UNIT_NAME, '--no-pager', '-n', String(lines)], { encoding: 'utf8', timeout: 10_000 });
  } catch {
    return '(journalctl unavailable)';
  }
}

async function waitForBackendHealthy(timeoutMs = READINESS_TIMEOUT_MS) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    let active = false;
    try {
      execFileSync('systemctl', ['is-active', '--quiet', UNIT_NAME]);
      active = true;
    } catch {
      // Not active yet.
    }
    if (active) {
      try {
        const health = await new Promise((res, rej) => {
          const r = httpRequest({ host: '127.0.0.1', port: BACKEND_PORT, path: '/api/health', method: 'GET' }, res);
          r.on('error', rej);
          r.end();
        });
        if (health.statusCode === 200) return true;
      } catch {
        // Not listening yet.
      }
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  return false;
}

/** daemon-reload + enable --now + wait for the real service to become
 * active and healthy - the real operator flow (docs/remote-ingest.md
 * §19), never a directly spawned test binary. */
async function startServerViaSystemd() {
  sh('sudo', ['systemctl', 'daemon-reload']);
  sh('sudo', ['systemctl', 'enable', '--now', UNIT_NAME]);
  const ready = await waitForBackendHealthy();
  return { ready };
}

/** A real `systemctl restart` - the credential-lifecycle restart-
 * persistence scenario needs exactly this, and nothing more: the
 * drop-in/credentials already on disk are untouched, only the process
 * itself restarts, identical to an operator-triggered restart. */
async function restartServerViaSystemd() {
  sh('sudo', ['systemctl', 'restart', UNIT_NAME]);
  return { ready: await waitForBackendHealthy() };
}

function stopAndDisableServerViaSystemd() {
  try {
    execFileSync('sudo', ['systemctl', 'disable', '--now', UNIT_NAME], { stdio: 'ignore' });
  } catch {
    // Already stopped/disabled - fine.
  }
}

// --- destination-branch-to-sink E2E (PRE-20E.1 gap A) -----------------
// A real local MediaMTX instance standing in for a destination platform,
// reusing the exact same managed-installer binary the app's own MediaMTX
// already is (scripts/verify-ffmpeg-branches.mjs's own already-proven
// pattern for this) - never a fake/stub sink, never a real provider.
function renderSinkConfig(apiAddress, rtmpAddress) {
  return [
    'logLevel: info', 'logDestinations: [stdout]', 'readTimeout: 10s', 'writeTimeout: 10s',
    'api: true', `apiAddress: ${apiAddress}`,
    'rtmp: true', `rtmpAddress: ${rtmpAddress}`, 'rtmpEncryption: "no"',
    'rtsp: false', 'hls: false', 'webrtc: false', 'srt: false', 'moq: false',
    'metrics: false', 'pprof: false', 'playback: false',
    'pathDefaults:', '  record: false',
    'paths:',
    '  # A real destination platform accepts whatever stream key the',
    '  # publisher presents as the rest of the path, not one fixed name.',
    '  all_others:', '    source: publisher',
    '',
  ].join('\n');
}

/** Copies the already-downloaded managed MediaMTX binary out of the
 * DynamicUser-owned state directory into a path this script's own
 * unprivileged user can execute directly - the sink is intentionally
 * NOT backend-managed (it stands in for an external platform).
 *
 * The real install path (apps/server/internal/runtime/mediamtx/
 * resolver.go's own InstallDir/ManagedExecutablePath) is
 * runtime/mediamtx/<version>/<platformDir>/<executableName> - a real
 * CI failure (docs/progress.md, PRE-20E.1) proved a first version of
 * this function was missing the <platformDir> ("linux-amd64"/
 * "linux-arm64") segment entirely, guessing runtime/mediamtx/<version>/
 * mediamtx directly. */
function findManagedMediaMtxExecutable() {
  const platformDir = process.arch === 'arm64' ? 'linux-arm64' : 'linux-amd64';
  const root = join(STATE_DIR, 'runtime', 'mediamtx');
  const versionsOut = execFileSync('sudo', ['ls', root], { encoding: 'utf8' }).trim();
  const version = versionsOut.split('\n').map((l) => l.trim()).filter(Boolean)[0];
  if (!version) fail('locating the managed MediaMTX executable for the sink', `no version directory under ${root}`);
  const exePath = join(root, version, platformDir, 'mediamtx');
  const tmpExe = join(tmpdir(), `d2c-sink-mediamtx-${randomBytes(6).toString('hex')}`);
  sh('sudo', ['cp', exePath, tmpExe]);
  sh('sudo', ['chown', userInfo().username, tmpExe]);
  sh('chmod', ['0755', tmpExe]);
  return tmpExe;
}

function startSinkSubprocess(exePath, configPath) {
  const child = spawn(exePath, [configPath], { stdio: ['ignore', 'pipe', 'pipe'] });
  let exited = false;
  let exitCode = null;
  let output = '';
  child.stdout.on('data', (c) => (output += c.toString()));
  child.stderr.on('data', (c) => (output += c.toString()));
  child.on('exit', (code) => {
    exited = true;
    exitCode = code;
  });
  return { child, hasExited: () => exited, exitCode: () => exitCode, getOutput: () => output };
}

function hostFetchAddr(hostPort, path) {
  const [host, portStr] = hostPort.split(':');
  return new Promise((resolvePromise) => {
    const req = httpRequest({ host, port: Number(portStr), path, method: 'GET' }, (res) => {
      let body = '';
      res.on('data', (c) => (body += c));
      res.on('end', () => {
        try {
          resolvePromise({ status: res.statusCode, body: JSON.parse(body) });
        } catch {
          resolvePromise({ status: res.statusCode, body });
        }
      });
    });
    req.on('error', () => resolvePromise({ status: 0, body: null }));
    req.end();
  });
}

async function waitForSinkReady(apiAddress, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await hostFetchAddr(apiAddress, '/v3/paths/list');
    if (res.status === 200) return true;
    await new Promise((r) => setTimeout(r, 300));
  }
  return false;
}

async function sinkPathReady(apiAddress, pathName) {
  const res = await hostFetchAddr(apiAddress, '/v3/paths/list');
  if (res.status !== 200) return false;
  const items = (res.body && Array.isArray(res.body.items)) ? res.body.items : [];
  const item = items.find((i) => i && i.name === pathName);
  return item && item.ready ? item : false;
}

async function stopSink(handle) {
  if (!handle || handle.hasExited()) return;
  await new Promise((res) => {
    const timer = setTimeout(() => res(), 5_000);
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

  // Self-contained, fresh-state install (PRE-20E.1 §19/§20: no pass-1
  // dependency) - never assumes an earlier workflow step, or an earlier
  // pass of this same two-pass script, left anything behind. Covers the
  // real systemd service/drop-in/credential paths this script now owns,
  // on top of the package itself.
  stopAndDisableServerViaSystemd();
  try {
    execFileSync('sudo', ['rm', '-rf', DROPIN_DIR], { stdio: 'ignore' });
  } catch {
    // Nothing to remove - fine.
  }
  try {
    execFileSync('sudo', ['rm', '-rf', ETC_DIR], { stdio: 'ignore' });
  } catch {
    // Nothing to remove - fine.
  }
  try {
    execFileSync('sudo', ['rm', '-rf', STATE_DIR], { stdio: 'ignore' });
  } catch {
    // Nothing to remove - fine.
  }
  if (shOk('dpkg', ['-s', PACKAGE_NAME])) {
    execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'ignore' });
  }

  step('The .deb installs successfully');
  execFileSync('sudo', ['dpkg', '-i', debPath], { stdio: 'pipe' });
  let installed = true;
  expect(shOk('dpkg', ['-s', PACKAGE_NAME]), `${PACKAGE_NAME} is installed`, PACKAGE_NAME);
  expect(existsSync(INSTALLED_EXE_PATH), 'the executable was installed', INSTALLED_EXE_PATH);

  step('Package install alone does not enable, start, or expose the service (safe default)');
  let enabledAfterInstall = '';
  try {
    enabledAfterInstall = execFileSync('systemctl', ['is-enabled', UNIT_NAME], { encoding: 'utf8' }).trim();
  } catch (err) {
    enabledAfterInstall = (err.stdout || '').toString().trim() || (err.stderr || '').toString().trim();
  }
  expect(enabledAfterInstall.includes('disabled'), 'the unit is not enabled by package install alone', enabledAfterInstall);
  let activeAfterInstall = 'unknown';
  try {
    activeAfterInstall = execFileSync('systemctl', ['is-active', UNIT_NAME], { encoding: 'utf8' }).trim();
  } catch (err) {
    activeAfterInstall = (err.stdout || '').toString().trim() || (err.stderr || '').toString().trim();
  }
  expect(activeAfterInstall !== 'active', 'the service is not running by package install alone', activeAfterInstall);

  verifyNetnsCapability();

  let networkUp = false;
  let pkiDir;
  let serverStarted = false;
  let manageProxy;
  let overlayProxy;
  let sinkHandle;

  try {
    setUpNetwork();
    networkUp = true;

    step('Generate the ephemeral 3-host test CA and install it into the trust store');
    pkiDir = mkdtempSync(join(tmpdir(), 'streamtree-d2c-pki-'));
    const pki = generateEphemeralPKI(pkiDir);
    installEphemeralCATrust(pki.caCertPath);
    pass('ephemeral CA installed for this CI run only');

    step('Provision the master key, RTMPS credential, and administrator password through the real systemd credential mechanism');
    provisionMasterKey();
    deliverRtmpsCredentialFiles(pki.leaves[INGEST_HOST]);
    provisionAdminPasswordViaRealIdentity();
    writeRemoteServerDropIn();
    pass('master key + RTMPS key/cert delivered via LoadCredential=, administrator password verifier provisioned under the real service identity, drop-in written');

    const dataDir = STATE_DIR;

    step('Real systemd lifecycle: daemon-reload, enable --now, wait for active + healthy');
    const serverHandle = await startServerViaSystemd();
    serverStarted = true;
    expect(serverHandle.ready, 'the real systemd-managed service became active and healthy', serverJournal(200));

    step('The service is genuinely active under systemd (ActiveState=active (running))');
    const statusOut = execFileSync('systemctl', ['status', UNIT_NAME, '--no-pager'], { encoding: 'utf8' });
    expect(statusOut.includes('active (running)'), 'systemctl status reports active (running)', statusOut);

    // A backend-side "internal_error" response body is deliberately
    // sanitized (apps/server/internal/httpapi/remoteingest.go's
    // writeRemoteIngestError) - the real Go error only reaches the
    // server's own log, which now lives in the journal since this
    // script drives the real systemd-managed service, not a directly
    // spawned child process this script itself holds a stdout pipe to.
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
      const lines = serverJournal(300).split('\n');
      const notable = lines.filter((l) => /level=(ERR|WARN)|mediamtx_message/.test(l));
      const recent = lines.slice(-5);
      const combined = [...new Set([...notable, ...recent])].join('\n');
      return `--- server journal: error/warning + mediamtx lines + recent tail ---\n${combined.slice(-1500)}\n--- caller detail ---\n${text.slice(0, 300)}`;
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

    step('D2A hardening survives D2C: the real service and its MediaMTX child both run as a non-root DynamicUser');
    const unitTextForHardening = execFileSync('cat', [UNIT_PATH], { encoding: 'utf8' }) + readRootFile(DROPIN_PATH);
    for (const directive of ['DynamicUser=yes', 'NoNewPrivileges=yes', 'ProtectSystem=strict', 'ProtectHome=yes', 'PrivateTmp=yes', 'CapabilityBoundingSet=', 'RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6']) {
      expect(unitTextForHardening.includes(directive), `the effective unit still carries ${directive}`, directive);
    }
    const mainPid = execFileSync('systemctl', ['show', '-p', 'MainPID', '--value', UNIT_NAME], { encoding: 'utf8' }).trim();
    expect(/^\d+$/.test(mainPid) && mainPid !== '0', 'systemctl reports a real MainPID for the service', mainPid);
    const mainUid = execFileSync('ps', ['-o', 'uid=', '-p', mainPid], { encoding: 'utf8' }).trim();
    expect(mainUid !== '' && mainUid !== '0', 'the main service process does not run as UID 0 (root)', mainUid);
    const mediaMtxPid = execFileSync('pgrep', ['-f', 'mediamtx'], { encoding: 'utf8' }).trim().split('\n')[0];
    expect(/^\d+$/.test(mediaMtxPid), 'a real MediaMTX process is running', mediaMtxPid);
    const mediaMtxUid = execFileSync('ps', ['-o', 'uid=', '-p', mediaMtxPid], { encoding: 'utf8' }).trim();
    expect(mediaMtxUid !== '' && mediaMtxUid !== '0', 'the MediaMTX child process does not run as UID 0 (root) either', mediaMtxUid);
    expect(mediaMtxUid === mainUid, 'MediaMTX runs under the exact same DynamicUser identity as the parent service (inherited, not separately elevated)', `main=${mainUid} mediamtx=${mediaMtxUid}`);

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
    // Read via sudo: DynamicUser has taken ownership of StateDirectory,
    // so this script's own unprivileged invoking user cannot read it
    // directly now that the server runs as a real systemd service.
    const configContent = readRootFile(configPath);
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

    // --- credential lifecycle: rotate, restart persistence, revoke, ------
    // --- and the 409-while-receiving non-mutation guarantee for both -----
    // apps/server/internal/remoteingest/manager.go's own Rotate/Revoke
    // both check IngestReceiving() and return ErrStreamingActive
    // *before* touching the secret store or requesting a MediaMTX
    // restart - so a real 409 here structurally proves no mutation and
    // no restart happened, not merely that the HTTP layer reported one.
    // A full publish+settle (not the fast reject-matrix's 1.5s window)
    // is used to prove *acceptance*, mirroring the already-established
    // reason the positive path above needs real settle time too.
    async function publishAndConfirmAccepted(secret) {
      // -t 8, not 4: PUBLISH_SETTLE_MS (4s) needs a real margin before
      // the clip's own natural end, exactly like the main positive-
      // path test above uses -t 10 against the same 4s settle wait -
      // a first version of this helper used -t 4 (equal to the settle
      // wait itself) and a real CI failure showed the settle check
      // racing the publish's own completion: exitCode 0, but
      // receiving:false, at exactly 40 frames (a clean 4.0s clip
      // finishing right as the check fired), not a real rejection.
      const app = `${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${secret}`;
      const child = clientSpawn('ffmpeg', [...ffmpegBase.slice(0, -4), '-t', '8', '-rtmp_app', app, '-f', 'flv', `${rtmpsBase}/`]);
      let exited = false;
      let exitCode = null;
      let stderr = '';
      child.stderr.on('data', (c) => (stderr += c.toString()));
      child.on('exit', (code) => {
        exited = true;
        exitCode = code;
      });
      await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
      const statusRes = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken });
      const receiving = !!(statusRes.body && statusRes.body.receiving);
      const deadline = Date.now() + 10_000;
      while (!exited && Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, 200));
      }
      if (!exited) child.kill('SIGKILL');
      return { receiving, exited, exitCode, detail: stderr.slice(-800) };
    }

    step('Credential lifecycle: rotate replaces the credential (old rejected, new accepted)');
    const rotateRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-ingest/rotate`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken });
    expect(rotateRes.status === 200, 'POST /api/remote-ingest/rotate succeeds while not streaming', rotateRes.status === 200 ? rotateRes.text : withServerDiag(rotateRes.text));
    const rotatedSecret = rotateRes.body && rotateRes.body.secret;
    expect(typeof rotatedSecret === 'string' && rotatedSecret.length > 0 && rotatedSecret !== publisherSecret, 'rotate returns a new, different plaintext secret (never recoverable/reused from the original)', rotateRes.text);
    const afterRotateState = await waitForMediaMtxState(['ready'], 30_000);
    expect(afterRotateState === 'ready', 'MediaMTX becomes ready again after the rotate-triggered restart', withServerDiag(`state=${afterRotateState}`));

    const oldRejectedAfterRotate = await tryPublishAndCheckAccepted(`${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${publisherSecret}`, '', INGEST_PATH);
    expect(!oldRejectedAfterRotate.accepted, 'the pre-rotate credential is rejected after rotation', oldRejectedAfterRotate.accepted ? oldRejectedAfterRotate.snapshot : '');
    const newAcceptedAfterRotate = await publishAndConfirmAccepted(rotatedSecret);
    expect(newAcceptedAfterRotate.receiving && newAcceptedAfterRotate.exited && newAcceptedAfterRotate.exitCode === 0, 'the newly rotated credential publishes successfully', JSON.stringify(newAcceptedAfterRotate));

    step('Credential lifecycle: a real systemd service restart preserves the rotated credential (new still accepted, old still rejected)');
    // A real `systemctl restart` - unlike the direct-spawn process this
    // script used before PRE-20E.1, systemd's own default KillMode
    // (control-group) tears down the *entire* cgroup on restart,
    // including the MediaMTX child process, not merely the main Go
    // process - the orphaned-MediaMTX-child bug a prior audit found and
    // fixed for the direct-spawn path structurally cannot recur here.
    const restartedHandle = await restartServerViaSystemd();
    expect(restartedHandle.ready, 'the real systemd-managed service becomes active and healthy again after systemctl restart', serverJournal(200));
    // A fresh session is obtained unconditionally rather than assuming
    // a specific session-persistence model survives the restart - the
    // credential-lifecycle claim under test is about the secret store
    // and MediaMTX config, not about session survival.
    const reloginRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/auth/login`, { body: { password: ADMIN_PASSWORD }, headers: { Origin: MANAGE_ORIGIN }, cookieJar });
    expect(reloginRes.status === 200, 'logging in again after the restart succeeds', reloginRes.status === 200 ? reloginRes.text : withServerDiag(reloginRes.text));
    const csrfTokenAfterRestart = reloginRes.body && reloginRes.body.csrfToken;
    const afterRestartMediaMtxState = await waitForMediaMtxState(['ready'], 30_000);
    expect(afterRestartMediaMtxState === 'ready', 'MediaMTX auto-starts and becomes ready again after the service restart', withServerDiag(`state=${afterRestartMediaMtxState}`));

    const newStillAcceptedAfterRestart = await publishAndConfirmAccepted(rotatedSecret);
    expect(newStillAcceptedAfterRestart.receiving && newStillAcceptedAfterRestart.exited && newStillAcceptedAfterRestart.exitCode === 0, 'the rotated credential still publishes successfully after a real service restart', JSON.stringify(newStillAcceptedAfterRestart));
    const oldStillRejectedAfterRestart = await tryPublishAndCheckAccepted(`${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${publisherSecret}`, '', INGEST_PATH);
    expect(!oldStillRejectedAfterRestart.accepted, 'the pre-rotate credential is still rejected after a real service restart', oldStillRejectedAfterRestart.accepted ? oldStillRejectedAfterRestart.snapshot : '');

    step('Credential lifecycle: revoke removes the credential (nothing can authenticate afterward)');
    const revokeRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-ingest/revoke`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(revokeRes.status === 200, 'POST /api/remote-ingest/revoke succeeds while not streaming', revokeRes.status === 200 ? revokeRes.text : withServerDiag(revokeRes.text));
    const afterRevokeState = await waitForMediaMtxState(['ready'], 30_000);
    expect(afterRevokeState === 'ready', 'MediaMTX becomes ready again after the revoke-triggered restart', withServerDiag(`state=${afterRevokeState}`));
    const rejectedAfterRevoke = await tryPublishAndCheckAccepted(`${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${rotatedSecret}`, '', INGEST_PATH);
    expect(!rejectedAfterRevoke.accepted, 'the revoked credential is rejected - nothing can authenticate until provision/rotate is called again', rejectedAfterRevoke.accepted ? rejectedAfterRevoke.snapshot : '');

    step('Credential lifecycle: rotate while receiving is refused with 409 and mutates nothing');
    const reprovisionRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-ingest/provision`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(reprovisionRes.status === 200, 'provisioning again after a revoke succeeds', reprovisionRes.status === 200 ? reprovisionRes.text : withServerDiag(reprovisionRes.text));
    const liveSecret = reprovisionRes.body && reprovisionRes.body.secret;
    const afterReprovisionState = await waitForMediaMtxState(['ready'], 30_000);
    expect(afterReprovisionState === 'ready', 'MediaMTX becomes ready again after the reprovision-triggered restart', withServerDiag(`state=${afterReprovisionState}`));

    const rotateWhileReceivingApp = `${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${liveSecret}`;
    const rotateWhileReceivingChild = clientSpawn('ffmpeg', [...ffmpegBase.slice(0, -4), '-t', '6', '-rtmp_app', rotateWhileReceivingApp, '-f', 'flv', `${rtmpsBase}/`]);
    let rotateWhileReceivingExited = false;
    let rotateWhileReceivingExitCode = null;
    rotateWhileReceivingChild.on('exit', (code) => {
      rotateWhileReceivingExited = true;
      rotateWhileReceivingExitCode = code;
    });
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
    const receivingForRotate409 = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(receivingForRotate409.body && receivingForRotate409.body.receiving === true, 'ingest status is receiving before the rotate-while-receiving attempt', receivingForRotate409.status === 200 ? receivingForRotate409.text : withServerDiag(receivingForRotate409.text));
    const rotateWhileReceivingRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-ingest/rotate`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(rotateWhileReceivingRes.status === 409, 'rotate while receiving is refused with 409, not applied', rotateWhileReceivingRes.status === 409 ? '' : withServerDiag(rotateWhileReceivingRes.text));
    const mediaMtxStateRightAfterRotate409 = await hostFetchJson('/v3/config/global/get');
    expect(mediaMtxStateRightAfterRotate409.status === 200, 'MediaMTX Control API is still answering immediately after the refused rotate (no restart was triggered)', JSON.stringify(mediaMtxStateRightAfterRotate409.body).slice(0, 300));

    const rotate409Deadline = Date.now() + 15_000;
    while (!rotateWhileReceivingExited && Date.now() < rotate409Deadline) {
      await new Promise((r) => setTimeout(r, 300));
    }
    if (!rotateWhileReceivingExited) rotateWhileReceivingChild.kill('SIGKILL');
    expect(rotateWhileReceivingExited && rotateWhileReceivingExitCode === 0, 'the in-flight publish, using the credential the refused rotate never touched, completes successfully - proving no mutation', `exited=${rotateWhileReceivingExited} code=${rotateWhileReceivingExitCode}`);
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));

    step('Credential lifecycle: revoke while receiving is refused with 409 and mutates nothing');
    const revokeWhileReceivingApp = `${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${liveSecret}`;
    const revokeWhileReceivingChild = clientSpawn('ffmpeg', [...ffmpegBase.slice(0, -4), '-t', '6', '-rtmp_app', revokeWhileReceivingApp, '-f', 'flv', `${rtmpsBase}/`]);
    let revokeWhileReceivingExited = false;
    let revokeWhileReceivingExitCode = null;
    revokeWhileReceivingChild.on('exit', (code) => {
      revokeWhileReceivingExited = true;
      revokeWhileReceivingExitCode = code;
    });
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
    const receivingForRevoke409 = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(receivingForRevoke409.body && receivingForRevoke409.body.receiving === true, 'ingest status is receiving before the revoke-while-receiving attempt', receivingForRevoke409.status === 200 ? receivingForRevoke409.text : withServerDiag(receivingForRevoke409.text));
    const revokeWhileReceivingRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-ingest/revoke`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(revokeWhileReceivingRes.status === 409, 'revoke while receiving is refused with 409, not applied', revokeWhileReceivingRes.status === 409 ? '' : withServerDiag(revokeWhileReceivingRes.text));
    const mediaMtxStateRightAfterRevoke409 = await hostFetchJson('/v3/config/global/get');
    expect(mediaMtxStateRightAfterRevoke409.status === 200, 'MediaMTX Control API is still answering immediately after the refused revoke (no restart was triggered)', JSON.stringify(mediaMtxStateRightAfterRevoke409.body).slice(0, 300));

    const revoke409Deadline = Date.now() + 15_000;
    while (!revokeWhileReceivingExited && Date.now() < revoke409Deadline) {
      await new Promise((r) => setTimeout(r, 300));
    }
    if (!revokeWhileReceivingExited) revokeWhileReceivingChild.kill('SIGKILL');
    expect(revokeWhileReceivingExited && revokeWhileReceivingExitCode === 0, 'the in-flight publish, using the credential the refused revoke never touched, completes successfully - proving no mutation', `exited=${revokeWhileReceivingExited} code=${revokeWhileReceivingExitCode}`);
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));

    // --- destination-branch-to-sink E2E (PRE-20E.1 gap A) -----------------
    // The EXISTING production branch manager (apps/server/internal/
    // runtime/branch), pointed at a real local sink MediaMTX instance -
    // never Twitch/YouTube/Kick/TikTok, never real provider credentials.
    // liveSecret is still the valid ingest credential at this point (the
    // rotate/revoke-while-receiving attempts above were both correctly
    // refused).
    step('Branch-to-sink: start a real local destination sink (reusing the same managed MediaMTX binary, standing in for a platform)');
    const sinkApiAddress = `127.0.0.1:${SINK_API_PORT}`;
    const sinkRtmpAddress = `127.0.0.1:${SINK_RTMP_PORT}`;
    const sinkMediaMtxExe = findManagedMediaMtxExecutable();
    const sinkDir = mkdtempSync(join(tmpdir(), 'streamtree-d2c-sink-'));
    const sinkConfigPath = join(sinkDir, 'mediamtx.yml');
    writeFileSync(sinkConfigPath, renderSinkConfig(sinkApiAddress, sinkRtmpAddress));
    sinkHandle = startSinkSubprocess(sinkMediaMtxExe, sinkConfigPath);
    const sinkReady = await waitForSinkReady(sinkApiAddress, 15_000);
    expect(sinkReady, 'the local destination sink MediaMTX becomes ready', sinkHandle.getOutput().slice(-500));

    step('Branch-to-sink: point a seeded platform at the local sink (never a real provider) with a fake stream key');
    const platformsRes = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/platforms`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(platformsRes.status === 200 && Array.isArray(platformsRes.body.platforms) && platformsRes.body.platforms.length === 4, 'GET /api/platforms returns the four seeded platforms', platformsRes.text);
    const branchPlatform = platformsRes.body.platforms.find((p) => p.providerId === 'twitch');
    expect(Boolean(branchPlatform), 'the twitch seeded platform exists', platformsRes.body.platforms);
    const enablePlatformRes = await remoteCurl('PUT', `${MANAGE_ORIGIN}/api/platforms/${branchPlatform.id}`, {
      body: { displayName: branchPlatform.displayName, enabled: true, sortOrder: branchPlatform.sortOrder },
      headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart,
    });
    expect(enablePlatformRes.status === 200 && enablePlatformRes.body.enabled === true, 'the platform is enabled', enablePlatformRes.text);
    const fakeSinkUrl = `rtmp://${sinkRtmpAddress}/out`;
    const outputRes = await remoteCurl('PUT', `${MANAGE_ORIGIN}/api/platforms/${branchPlatform.id}/output`, {
      body: { serverUrl: fakeSinkUrl, autoRestart: true }, headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart,
    });
    expect(outputRes.status === 200 && outputRes.body.serverUrl === fakeSinkUrl, 'the destination output points at the local sink, never a real provider', outputRes.text);
    const FAKE_STREAM_KEY = `d2c-fake-key-${randomBytes(8).toString('hex')}`;
    const sinkPathName = `out/${FAKE_STREAM_KEY}`;
    const streamKeyRes = await remoteCurl('PUT', `${MANAGE_ORIGIN}/api/platforms/${branchPlatform.id}/credentials/stream-key`, {
      body: { streamKey: FAKE_STREAM_KEY }, headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart,
    });
    expect(streamKeyRes.status === 200 && streamKeyRes.body.streamKey && streamKeyRes.body.streamKey.configured === true, 'a fake, non-provider stream key is stored', streamKeyRes.text);

    step('Branch-to-sink: publish from the isolated remote client, then explicitly request the destination to start');
    const branchTestApp = `${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${liveSecret}`;
    // -t 65: comfortably longer than the 40s wait-for-live window plus
    // the 15s sink-readiness check after it - the source publish
    // itself must still be active for both, or the branch/sink never
    // get the chance to observe it.
    const branchPublishChild = clientSpawn('ffmpeg', [...ffmpegBase.slice(0, -4), '-t', '65', '-rtmp_app', branchTestApp, '-f', 'flv', `${rtmpsBase}/`]);
    let branchPublishExited = false;
    let branchPublishExitCode = null;
    branchPublishChild.on('exit', (code) => {
      branchPublishExited = true;
      branchPublishExitCode = code;
    });
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
    const receivingForBranch = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(receivingForBranch.body && receivingForBranch.body.receiving === true, 'ingest is receiving before the destination branch is requested to start', receivingForBranch.text);

    const startBranchRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/runtime/branches/${branchPlatform.id}/start`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(startBranchRes.status === 202 || startBranchRes.status === 200, 'the destination branch start is accepted', startBranchRes.text);

    async function getBranchSnapshot() {
      const res = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/runtime/branches`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
      const list = (res.body && res.body.branches) || [];
      return list.find((b) => b.platformId === branchPlatform.id);
    }
    async function waitForBranchState(wantedStates, timeoutMs) {
      const deadline = Date.now() + timeoutMs;
      let last;
      while (Date.now() < deadline) {
        last = await getBranchSnapshot();
        if (last && (wantedStates.includes(last.state) || last.state === 'error')) return last;
        await new Promise((r) => setTimeout(r, 500));
      }
      return last;
    }

    step('Branch-to-sink: the destination branch reaches live using the EXISTING production branch manager');
    // 40s, not 20s: a real CI failure (docs/progress.md, PRE-20E.1)
    // showed the branch genuinely starting (desiredRunning: true,
    // blockers: [], lastError: null - no defect, just insufficient
    // margin) and still sitting in "starting" 20s in, waiting for
    // FFmpeg's own first -progress pipe:1 line to arrive - the same
    // kind of settle-margin issue the credential-lifecycle publish
    // helper needed more time for earlier this milestone.
    const liveBranch = await waitForBranchState(['live'], 40_000);
    expect(liveBranch && liveBranch.state === 'live', 'the destination branch reaches live', JSON.stringify(liveBranch));

    step('Branch-to-sink: the local sink genuinely receives real, advancing media (not merely a socket connection)');
    const sinkDeadline = Date.now() + 15_000;
    let sinkItem = false;
    while (Date.now() < sinkDeadline) {
      sinkItem = await sinkPathReady(sinkApiAddress, sinkPathName);
      if (sinkItem) break;
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(sinkItem !== false, 'the local sink reports the destination path ready', sinkPathName);
    expect(sinkItem && Array.isArray(sinkItem.tracks) && sinkItem.tracks.length > 0, 'the sink detected real decoded tracks - actual media bytes arrived, not merely an accepted connection', sinkItem);

    step('Branch-to-sink: the branch never uses the remote publisher credential (structural isolation + a real log scan)');
    // apps/server/internal/runtime/branch/command.go's own buildArgs
    // always takes its -i input from Supervisor.Snapshot().Connection.
    // PublishURL, which is unconditionally the loopback rtmpAddress/
    // IngestPath (config.go's own RenderConfig keeps rtmpAddress
    // loopback-only even with remote ingest enabled) - never the RTMPS
    // remote-publisher URL or credential, structurally, not merely by
    // convention. Confirmed here by scanning the real service journal.
    const branchIsolationLog = serverJournal(500);
    expect(!branchIsolationLog.includes(liveSecret), 'the remote publisher secret never appears in the service journal', '');

    step('Branch-to-sink: publisher disconnect returns ingest to waiting; the branch follows its real ingest-loss policy (waiting_for_ingest, still desired-running)');
    if (!branchPublishExited) branchPublishChild.kill('SIGKILL');
    const branchDisconnectDeadline = Date.now() + 15_000;
    while (!branchPublishExited && Date.now() < branchDisconnectDeadline) {
      await new Promise((r) => setTimeout(r, 300));
    }
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
    const afterBranchDisconnect = await remoteCurl('GET', `${MANAGE_ORIGIN}/api/remote-ingest/status`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(afterBranchDisconnect.body && afterBranchDisconnect.body.receiving === false, 'ingest status returns to waiting after the publisher disconnects', afterBranchDisconnect.text);
    const branchAfterLoss = await waitForBranchState(['waiting_for_ingest'], 15_000);
    expect(branchAfterLoss && branchAfterLoss.state === 'waiting_for_ingest', 'the branch transitions to waiting_for_ingest (not stopped, not an error)', JSON.stringify(branchAfterLoss));
    expect(branchAfterLoss && branchAfterLoss.desiredRunning === true, 'the branch remains desired-running through ingest loss - eligible for the existing auto-resume contract', JSON.stringify(branchAfterLoss));

    step('Branch-to-sink: publisher reconnect - the branch auto-resumes to live per the EXISTING desired-running contract (no new behavior invented)');
    const reconnectApp = `${INGEST_PATH}?user=${PUBLISHER_USER}&pass=${liveSecret}`;
    // -t 65 for the same reason the main branch-to-sink publish above needs it.
    const reconnectChild = clientSpawn('ffmpeg', [...ffmpegBase.slice(0, -4), '-t', '65', '-rtmp_app', reconnectApp, '-f', 'flv', `${rtmpsBase}/`]);
    let reconnectExited = false;
    reconnectChild.on('exit', () => {
      reconnectExited = true;
    });
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));
    // 40s for the same real reason the first live-wait above needed it.
    const branchAfterResume = await waitForBranchState(['live'], 40_000);
    expect(branchAfterResume && branchAfterResume.state === 'live', 'the branch auto-resumes to live once the source reconnects (branch.Manager.reconcileOnce -> attemptResume)', JSON.stringify(branchAfterResume));

    step('Branch-to-sink: an explicit stop is respected');
    const stopBranchRes = await remoteCurl('POST', `${MANAGE_ORIGIN}/api/runtime/branches/${branchPlatform.id}/stop`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    expect(stopBranchRes.status === 200, 'the branch stop is accepted', stopBranchRes.text);
    const branchAfterStop = await waitForBranchState(['idle', 'stopping'], 15_000);
    expect(branchAfterStop && (branchAfterStop.state === 'idle' || branchAfterStop.state === 'stopping'), 'the branch stops on explicit request', JSON.stringify(branchAfterStop));

    if (!reconnectExited) reconnectChild.kill('SIGKILL');
    const reconnectExitDeadline = Date.now() + 15_000;
    while (!reconnectExited && Date.now() < reconnectExitDeadline) {
      await new Promise((r) => setTimeout(r, 300));
    }
    await new Promise((r) => setTimeout(r, PUBLISH_SETTLE_MS));

    await stopSink(sinkHandle);
    sinkHandle = undefined;
    rmSync(sinkDir, { recursive: true, force: true });

    step('MediaMTX exposes the plaintext RTMP listener only on loopback - branch reads are unaffected by remote ingest');
    expect(
      !clientExecOk('curl', ['-sS', '--max-time', '2', `http://${HOST_ADDR}:${MEDIAMTX_RTMP_PORT}/`]),
      'the loopback-only plain RTMP listener does not answer on the veth address either',
      '',
    );

    // --- remote overlay E2E matrix (PRE-20E.1 gap B) -----------------------
    // Five real product overlay families (chat, alerts, audio, Stage 18A
    // goal widget, Stage 18B supporter widget - plus a dashboard, since it
    // has materially different response composition) through the actual
    // remote HTTPS overlay boundary from the isolated client, all four
    // sharing exactly the domain-key architecture the codebase really has
    // (chat-overlay, alert-profile, audio, widget - not five keys).
    async function remoteCurlLocal(method, url, body) {
      return remoteCurl(method, url, { body, headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    }
    async function enableOverlayCapability(domain, localSlug) {
      return remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-overlay/${domain}/${localSlug}/enable`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    }
    async function rotateOverlayCapability(domain, localSlug) {
      return remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-overlay/${domain}/${localSlug}/rotate`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    }
    async function disableOverlayCapability(domain, localSlug) {
      return remoteCurl('POST', `${MANAGE_ORIGIN}/api/remote-overlay/${domain}/${localSlug}/disable`, { headers: { Origin: MANAGE_ORIGIN }, cookieJar, csrfToken: csrfTokenAfterRestart });
    }
    function tokenFromCapabilityUrl(url) {
      return url.split('/').filter(Boolean).pop();
    }

    // domain: chat-overlay/alert-profile (never a hard error for the
    // wrong reasons, but chat-overlay itself DOES hard-404 on an
    // unresolved slug/token) - each family's real per-domain behavior is
    // asserted, never a single uniform assumption (apps/server/internal/
    // httpapi/*_test.go already establishes exactly which domains 404
    // and which return a safe default).
    async function testHardFailDomain(label, domain, publicPath, createLocalSlug, directLoopbackPath) {
      step(`Remote overlay: ${label} - direct loopback still uses the local slug`);
      const localSlug = await createLocalSlug();
      const direct = await hostFetchAddr(`127.0.0.1:${BACKEND_PORT}`, directLoopbackPath(localSlug));
      expect(direct.status === 200, `${label} direct loopback local publicSlug still works`, direct);

      step(`Remote overlay: ${label} - legacy local slug does not resolve remotely`);
      const legacy = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(localSlug)}`, { expectStatusOnly: true });
      expect(legacy.status === 404, `${label} legacy local slug fails remotely (404)`, legacy.status);

      step(`Remote overlay: ${label} - an invalid random capability fails`);
      const randomToken = randomBytes(16).toString('hex');
      const invalid = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(randomToken)}`, { expectStatusOnly: true });
      expect(invalid.status === 404, `${label} a random invalid capability fails (404)`, invalid.status);

      step(`Remote overlay: ${label} - enable issues a working remote capability`);
      const enableRes = await enableOverlayCapability(domain, localSlug);
      expect(enableRes.status === 200 && typeof enableRes.body.url === 'string', `${label} enable returns a remote capability URL`, enableRes.text);
      const token1 = tokenFromCapabilityUrl(enableRes.body.url);
      const valid1 = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token1)}`, { expectStatusOnly: true });
      expect(valid1.status === 200, `${label} the valid remote capability works through the overlay origin`, valid1.status);

      step(`Remote overlay: ${label} - rotate invalidates the old capability, the new one works`);
      const rotateRes = await rotateOverlayCapability(domain, localSlug);
      expect(rotateRes.status === 200, `${label} rotate succeeds`, rotateRes.text);
      const token2 = tokenFromCapabilityUrl(rotateRes.body.url);
      expect(token2 !== token1, `${label} rotate returns a different token`, `${token1} vs ${token2}`);
      const oldAfterRotate = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token1)}`, { expectStatusOnly: true });
      expect(oldAfterRotate.status === 404, `${label} the old capability fails immediately after rotate`, oldAfterRotate.status);
      const newAfterRotate = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token2)}`, { expectStatusOnly: true });
      expect(newAfterRotate.status === 200, `${label} the new capability works after rotate`, newAfterRotate.status);

      step(`Remote overlay: ${label} - disable revokes remote access`);
      const disableRes = await disableOverlayCapability(domain, localSlug);
      expect(disableRes.status === 200, `${label} disable succeeds`, disableRes.text);
      const afterDisable = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token2)}`, { expectStatusOnly: true });
      expect(afterDisable.status === 404, `${label} the capability fails after disable`, afterDisable.status);

      step(`Remote overlay: ${label} - the management origin never exposes the same public route`);
      const onManageHost = await remoteCurl('GET', `${MANAGE_ORIGIN}${publicPath(token2)}`, { expectStatusOnly: true });
      expect(onManageHost.status === 404, `${label} the public overlay route is excluded on the management origin`, onManageHost.status);
      return localSlug;
    }

    // domain: widget (goal/supporter/dashboard) and alert-profile both
    // NEVER hard-error - a safe, default snapshot is returned instead
    // (Part 40 / apps/server/internal/httpapi/goals.go, alerts.go's own
    // documented "never distinguishes not-found from disabled" contract).
    // Provable remotely by fingerprinting a real, non-default value only
    // the true profile carries.
    async function testSafeDefaultDomain(label, domain, publicPath, createLocalSlug, directLoopbackPath, realFingerprint, isRealResponse) {
      step(`Remote overlay: ${label} - direct loopback still uses the local slug and returns the real profile`);
      const localSlug = await createLocalSlug();
      const direct = await hostFetchAddr(`127.0.0.1:${BACKEND_PORT}`, directLoopbackPath(localSlug));
      expect(direct.status === 200 && isRealResponse(direct.body), `${label} direct loopback returns the real profile (${realFingerprint})`, direct.body);

      step(`Remote overlay: ${label} - legacy local slug remotely returns the safe default, never the real profile`);
      const legacy = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(localSlug)}`);
      expect(legacy.status === 200 && !isRealResponse(legacy.body), `${label} legacy local slug remotely falls back to the safe default`, legacy.body);

      step(`Remote overlay: ${label} - enable issues a working remote capability that returns the real profile`);
      const enableRes = await enableOverlayCapability(domain, localSlug);
      expect(enableRes.status === 200 && typeof enableRes.body.url === 'string', `${label} enable returns a remote capability URL`, enableRes.text);
      const token1 = tokenFromCapabilityUrl(enableRes.body.url);
      const valid1 = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token1)}`);
      expect(valid1.status === 200 && isRealResponse(valid1.body), `${label} the valid remote capability returns the real profile`, valid1.body);

      step(`Remote overlay: ${label} - rotate invalidates the old capability (falls back to the safe default), the new one works`);
      const rotateRes = await rotateOverlayCapability(domain, localSlug);
      expect(rotateRes.status === 200, `${label} rotate succeeds`, rotateRes.text);
      const token2 = tokenFromCapabilityUrl(rotateRes.body.url);
      const oldAfterRotate = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token1)}`);
      expect(oldAfterRotate.status === 200 && !isRealResponse(oldAfterRotate.body), `${label} the old capability falls back to the safe default immediately after rotate`, oldAfterRotate.body);
      const newAfterRotate = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token2)}`);
      expect(newAfterRotate.status === 200 && isRealResponse(newAfterRotate.body), `${label} the new capability returns the real profile after rotate`, newAfterRotate.body);

      step(`Remote overlay: ${label} - disable falls back to the safe default`);
      const disableRes = await disableOverlayCapability(domain, localSlug);
      expect(disableRes.status === 200, `${label} disable succeeds`, disableRes.text);
      const afterDisable = await remoteCurl('GET', `${OVERLAY_ORIGIN}${publicPath(token2)}`);
      expect(afterDisable.status === 200 && !isRealResponse(afterDisable.body), `${label} the safe default is returned after disable`, afterDisable.body);
      return localSlug;
    }

    // --- 1. chat overlay (hard-fail domain) ---------------------------------
    await testHardFailDomain(
      'chat overlay', 'chat-overlay',
      (slugOrToken) => `/api/public/chat-overlays/${slugOrToken}/config`,
      async () => {
        const res = await remoteCurlLocal('POST', `${MANAGE_ORIGIN}/api/chat-overlays`, { name: 'D2C Chat Overlay' });
        expect(res.status === 200, 'chat overlay profile created', res.text);
        return res.body.publicSlug;
      },
      (slug) => `/api/public/chat-overlays/${slug}/config`,
    );

    // --- 2. alert profile (safe-default domain) -----------------------------
    await testSafeDefaultDomain(
      'alert overlay', 'alert-profile',
      (slugOrToken) => `/api/public/alert-profiles/${slugOrToken}/config`,
      async () => {
        const created = await remoteCurlLocal('POST', `${MANAGE_ORIGIN}/api/alert-profiles`, { name: 'D2C Alert Profile' });
        expect(created.status === 200, 'alert profile created', created.text);
        const configured = await remoteCurlLocal('PUT', `${MANAGE_ORIGIN}/api/alert-profiles/${created.body.id}`, {
          name: 'D2C Alert Profile', enabled: true, language: 'en', theme: 'large', position: 'top', textAlign: 'center',
          maxQueueItems: 100, maximumQueueAgeSeconds: 120,
        });
        expect(configured.status === 200 && configured.body.theme === 'large', 'alert profile configured with a non-default theme (fingerprint)', configured.text);
        return created.body.publicSlug;
      },
      (slug) => `/api/public/alert-profiles/${slug}/config`,
      'theme=large',
      (body) => body && body.theme === 'large',
    );

    // --- 3. Stage 18A goal widget (safe-default domain, sharing DomainWidget) ---
    const goalCreateRes = await remoteCurlLocal('POST', `${MANAGE_ORIGIN}/api/goals`, {
      name: 'D2C Followers Goal', kind: 'followers', enabled: true, target: 4242, baseline: 0, providers: [], accounts: [], configRevision: 0,
    });
    expect(goalCreateRes.status === 201, 'a real Stage 18A goal is created', goalCreateRes.text);
    const widgetPresentation = {
      enabled: true, orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
      backgroundColor: '#00000080', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
      borderRadiusPx: 12, opacity: 1.0, showCurrent: true, showTarget: true, showPercent: true,
    };
    await testSafeDefaultDomain(
      'Stage 18A goal widget', 'widget',
      (slugOrToken) => `/api/public/widgets/${slugOrToken}/config`,
      async () => {
        const created = await remoteCurlLocal('POST', `${MANAGE_ORIGIN}/api/widget-profiles`, {
          ...widgetPresentation, name: 'D2C Goal Widget', kind: 'goal', goalId: goalCreateRes.body.id,
        });
        expect(created.status === 201 && created.body.kind === 'goal', 'a real Stage 18A goal widget profile is created, referencing the real goal', created.text);
        return created.body.publicSlug;
      },
      (slug) => `/api/public/widgets/${slug}/config`,
      'kind=goal with the real target',
      (body) => body && body.kind === 'goal' && body.target === 4242,
    );

    // --- 4. Stage 18B supporter/activity widget (safe-default domain) ----------
    await testSafeDefaultDomain(
      'Stage 18B supporter widget', 'widget',
      (slugOrToken) => `/api/public/widgets/${slugOrToken}/config`,
      async () => {
        const created = await remoteCurlLocal('POST', `${MANAGE_ORIGIN}/api/widget-profiles`, {
          ...widgetPresentation, name: 'D2C Latest Follower', kind: 'latest_follower',
        });
        expect(created.status === 201 && created.body.kind === 'latest_follower', 'a real Stage 18B supporter widget profile is created', created.text);
        return created.body.publicSlug;
      },
      (slug) => `/api/public/widgets/${slug}/config`,
      'kind=latest_follower',
      (body) => body && body.kind === 'latest_follower',
    );

    // --- 5. dashboard (same DomainWidget key, materially different composition) -
    const dashChildRes = await remoteCurlLocal('POST', `${MANAGE_ORIGIN}/api/widget-profiles`, {
      ...widgetPresentation, name: 'D2C Dashboard Child', kind: 'latest_subscriber',
    });
    expect(dashChildRes.status === 201, 'a real widget profile exists to compose into the dashboard', dashChildRes.text);
    await testSafeDefaultDomain(
      'Stage 18B dashboard', 'widget',
      (slugOrToken) => `/api/public/widgets/${slugOrToken}/config`,
      async () => {
        const created = await remoteCurlLocal('POST', `${MANAGE_ORIGIN}/api/widget-profiles`, {
          ...widgetPresentation, name: 'D2C Dashboard', kind: 'dashboard', columns: 2,
          children: [{ widgetProfileId: dashChildRes.body.id, column: 1, columnSpan: 1, row: 1, rowSpan: 1 }],
        });
        expect(created.status === 201 && created.body.kind === 'dashboard', 'a real dashboard profile is created, composing the real child widget', created.text);
        return created.body.publicSlug;
      },
      (slug) => `/api/public/widgets/${slug}/config`,
      'kind=dashboard',
      (body) => body && body.kind === 'dashboard',
    );
    pass('goal widget, supporter widget, and dashboard all resolve through the identical remoteoverlay.DomainWidget capability key');

    // --- 6. audio (hard-fail on bytes/ack; SSE stream; TTS honestly ------------
    // unavailable on this platform, docs/linux-headless-server.md's own
    // already-established finding - the presented-token bytesUrl property
    // is proven at the Go unit level (TestRemoteOverlayAudioBytesURLEchoes
    // ThePresentedTokenNotTheLocalSlug), never fabricated here without a
    // real synthesized item to test against.
    step('Remote overlay: audio - legacy local slug does not grant remote access to bytes');
    const audioLocalSlug = await (async () => {
      const status = await remoteCurlLocal('GET', `${MANAGE_ORIGIN}/api/audio/status`, undefined);
      expect(status.status === 200, 'audio status is reachable', status.text);
      const settings = await remoteCurlLocal('GET', `${MANAGE_ORIGIN}/api/audio/settings`, undefined);
      expect(settings.status === 200 && typeof settings.body.publicSlug === 'string', 'the real audio publicSlug is available', settings.text);
      return settings.body.publicSlug;
    })();
    const audioBytesLegacy = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/public/audio/${audioLocalSlug}/bytes/anytoken`, { expectStatusOnly: true });
    expect(audioBytesLegacy.status === 404, 'the legacy local slug is rejected on the audio bytes route', audioBytesLegacy.status);

    step('Remote overlay: audio - a valid remote capability establishes the SSE stream through the overlay origin');
    const audioEnableRes = await enableOverlayCapability('audio', audioLocalSlug);
    expect(audioEnableRes.status === 200 && typeof audioEnableRes.body.url === 'string', 'audio enable returns a remote capability URL', audioEnableRes.text);
    const audioToken1 = tokenFromCapabilityUrl(audioEnableRes.body.url);
    const audioStream = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/public/audio/${audioToken1}/stream`, { maxTimeSeconds: 3 });
    expect(typeof audioStream.text === 'string' && audioStream.text.includes('audio.reset'), 'the SSE stream connects and sends the initial audio.reset event through the overlay origin', audioStream.text.slice(0, 300));

    step('Remote overlay: audio - the ack route accepts the valid remote capability (never rejected by the proxy boundary)');
    const audioAckRes = await remoteCurl('POST', `${OVERLAY_ORIGIN}/api/public/audio/${audioToken1}/ack`, { body: { token: 'x', itemId: 'y', kind: 'start' }, expectStatusOnly: true });
    expect(audioAckRes.status !== 403, 'the ack route does not reject a valid capability at the proxy boundary', audioAckRes.status);

    step('Remote overlay: audio - rotate invalidates the old capability on the bytes route');
    const audioRotateRes = await rotateOverlayCapability('audio', audioLocalSlug);
    expect(audioRotateRes.status === 200, 'audio rotate succeeds', audioRotateRes.text);
    const audioToken2 = tokenFromCapabilityUrl(audioRotateRes.body.url);
    expect(audioToken2 !== audioToken1, 'audio rotate returns a different token', `${audioToken1} vs ${audioToken2}`);
    const audioOldBytesAfterRotate = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/public/audio/${audioToken1}/bytes/anytoken`, { expectStatusOnly: true });
    expect(audioOldBytesAfterRotate.status === 404, 'the pre-rotate audio capability is rejected on the bytes route', audioOldBytesAfterRotate.status);

    step('Remote overlay: audio - disable revokes remote access');
    const audioDisableRes = await disableOverlayCapability('audio', audioLocalSlug);
    expect(audioDisableRes.status === 200, 'audio disable succeeds', audioDisableRes.text);
    const audioAfterDisable = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/public/audio/${audioToken2}/bytes/anytoken`, { expectStatusOnly: true });
    expect(audioAfterDisable.status === 404, 'the audio capability is rejected on the bytes route after disable', audioAfterDisable.status);
    pass('audio SSE/ack/bytes proven remotely; the real TTS-sourced bytesUrl-echoes-presented-token property is honestly left to its existing Go unit-test proof - no supported TTS provider exists on Linux (docs/linux-headless-server.md), so no real bytesUrl can ever be emitted by the actual production binary in this environment');

    // --- visual asset referenced by a remote overlay (PRE-20E.1 §13) -------
    step('Remote overlay: a managed visual asset is reachable through the overlay origin via its own independent token');
    const tinyPngPath = join(pkiDir, 'd2c-visual-asset.png');
    // The smallest possible valid PNG (1x1, transparent) - real bytes,
    // real decodable image, not a placeholder string.
    const tinyPng = Buffer.from('89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000a4944415478da6360000002000155a2415a0000000049454e44ae426082', 'hex');
    writeFileSync(tinyPngPath, tinyPng);
    const uploadRes = await clientExecStatus('curl', [
      '-sS', '-o', '-', '-w', '\n__STATUS__:%{http_code}', '--max-time', '10',
      '-X', 'POST', '-H', `X-CSRF-Token: ${csrfTokenAfterRestart}`, '-b', cookieJar, '-c', cookieJar,
      '-F', `file=@${tinyPngPath};type=image/png`, '-F', 'displayName=D2C Visual Asset',
      `${MANAGE_ORIGIN}/api/visual-assets`,
    ]);
    const uploadMarker = (uploadRes.stdout || '').lastIndexOf('__STATUS__:');
    const uploadStatus = Number.parseInt(uploadMarker >= 0 ? uploadRes.stdout.slice(uploadMarker + '__STATUS__:'.length).trim() : '0', 10);
    const uploadBodyText = uploadMarker >= 0 ? uploadRes.stdout.slice(0, uploadMarker) : uploadRes.stdout;
    let uploadBody = {};
    try {
      uploadBody = JSON.parse(uploadBodyText);
    } catch {
      // Left as {} - the expect() below reports the raw text.
    }
    expect(uploadStatus === 201 && typeof uploadBody.url === 'string', 'a real visual asset uploads successfully through the authenticated management API', uploadBodyText.slice(0, 300));
    const visualAssetUrl = uploadBody.url;
    expect(visualAssetUrl.startsWith('/api/public/visual-assets/'), 'the visual-asset URL is well-formed', visualAssetUrl);

    const visualAssetViaOverlay = await remoteCurl('GET', `${OVERLAY_ORIGIN}${visualAssetUrl}`, { expectStatusOnly: true });
    expect(visualAssetViaOverlay.status === 200, 'the real visual-asset bytes are reachable through the overlay origin', visualAssetViaOverlay.status);

    const invalidAssetToken = randomBytes(32).toString('hex');
    const invalidAssetViaOverlay = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/public/visual-assets/${invalidAssetToken}`, { expectStatusOnly: true });
    expect(invalidAssetViaOverlay.status === 404, 'an invalid visual-asset token is not found', invalidAssetViaOverlay.status);

    const assetListViaOverlay = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/visual-assets`, { expectStatusOnly: true });
    expect(assetListViaOverlay.status === 404, 'no visual-asset list/enumeration endpoint is exposed through the overlay origin', assetListViaOverlay.status);

    const managementRouteViaOverlay = await remoteCurl('GET', `${OVERLAY_ORIGIN}/api/about`, { expectStatusOnly: true });
    expect(managementRouteViaOverlay.status === 404, 'a management-only route remains inaccessible through the overlay origin', managementRouteViaOverlay.status);

    // The visual-asset token is genuinely independent of the overlay
    // capability token that made this test possible to set up - it was
    // never derived from or bound to it (apps/server/internal/domain/
    // visualasset/asset.go's own NewPublicToken: an unrelated,
    // independently generated 32-byte crypto/rand value).
    expect(!visualAssetUrl.includes(audioToken2), 'the visual-asset token is independent of any overlay capability token, never bound to one', visualAssetUrl);

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
    step('Cleanup: stop the real service/sink/proxies, remove the ephemeral CA trust and systemd drop-in/credentials/state, tear down the network namespace, leave no owned process/state behind');
    if (manageProxy) await new Promise((res) => manageProxy.close(res));
    if (overlayProxy) await new Promise((res) => overlayProxy.close(res));
    if (sinkHandle) await stopSink(sinkHandle);
    if (serverStarted) stopAndDisableServerViaSystemd();
    // Defense in depth only - systemd's own default KillMode
    // (control-group) already tears down the whole cgroup, including
    // MediaMTX and any branch FFmpeg child, on disable --now.
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
    try {
      execFileSync('sudo', ['rm', '-rf', DROPIN_DIR], { stdio: 'ignore' });
    } catch {
      // Nothing to remove - fine.
    }
    try {
      execFileSync('sudo', ['rm', '-rf', ETC_DIR], { stdio: 'ignore' });
    } catch {
      // Nothing to remove - fine.
    }
    try {
      execFileSync('sudo', ['rm', '-rf', STATE_DIR], { stdio: 'ignore' });
    } catch {
      // Nothing to remove - fine.
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
    pass('cleanup complete - no owned process, network namespace, veth interface, systemd drop-in/state, package install, or trust-store change left behind');
  }
}

// A prior CI run (docs/progress.md, PRE-20E.1) ended abruptly with no
// "FAILED:" text at all - the log simply stopped mid-write, consistent
// with an unhandled promise rejection or uncaught exception bypassing
// main().catch() entirely (Node's own default behavior for those is an
// abrupt process exit with minimal diagnostics). These two handlers
// close that diagnostic gap for good, whatever the real cause turns
// out to be - and main().catch() below now prints the full error
// (stack included), not merely its message, for the same reason.
process.on('unhandledRejection', (reason) => {
  console.error('\nFAILED: unhandled promise rejection');
  console.error(reason);
  // Node's own docs: it is not safe to resume normal operation after
  // an uncaught exception/unhandled rejection - an explicit exit is
  // required, or the process can hang indefinitely in a corrupted
  // state instead of terminating (a first version of this handler
  // omitted this and, if anything, made an already-unexplained CI
  // failure's symptom worse, not better - docs/progress.md).
  process.exit(1);
});
process.on('uncaughtException', (err) => {
  console.error('\nFAILED: uncaught exception');
  console.error(err);
  process.exit(1);
});

main().catch((err) => {
  console.error('\nFAILED:');
  console.error(err);
  process.exitCode = 1;
});
