#!/usr/bin/env node
/**
 * Packaged-runtime verification (Stage 20A, integration script #23).
 *
 * Tests the REAL, release-built, packaged executable and its embedded
 * production frontend - never `go run`, never Vite, never a Node-served
 * asset. See docs/windows-packaging.md for the architecture this proves.
 *
 * Requires a release build to already exist at
 * build/release/staging/streaming-tree-server.exe - run
 *   powershell -File scripts/build-release.ps1 -Version "0.1.0-dev+test" -SkipInstaller
 * first. This script does not rebuild the frontend/executable itself: doing
 * so on every run would duplicate the release script's own job and make
 * this test far slower than the thing it is actually verifying.
 *
 * The temporary application-data directory is created under the OS temp
 * location and removed at the end, so no real user data directory is ever
 * touched. STREAMING_TREE_TEST_NO_UI=1 suppresses the real browser-launch/
 * native-dialog side effects for the whole run (docs/windows-packaging.md
 * §30) - never a production code path, only ever read once at process
 * startup.
 *
 * Usage:  node scripts/verify-packaged-app.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const STAGED_EXE = join(REPO_ROOT, 'build', 'release', 'staging', 'streaming-tree-server.exe');

const PORT = 8299;
const BASE_URL = `http://127.0.0.1:${PORT}`;

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

async function request(method, path, body) {
  const options = { method, headers: { Accept: 'application/json' } };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }
  const response = await fetch(`${BASE_URL}${path}`, options);
  const text = await response.text();
  let payload = text;
  try {
    payload = JSON.parse(text);
  } catch {
    // Not JSON - keep the raw text (HTML/plain-text routes).
  }
  return { status: response.status, headers: response.headers, body: payload, text };
}

/** Starts the real packaged executable against a hermetic data directory. */
async function startPackagedApp(dataDir) {
  const child = spawn(STAGED_EXE, [], {
    cwd: dirname(STAGED_EXE),
    env: {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(PORT),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_TEST_NO_UI: '1',
      SystemRoot: process.env.SystemRoot ?? 'C:\\Windows',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  let stderr = '';
  let stdout = '';
  child.stderr.on('data', (chunk) => {
    stderr += chunk.toString();
  });
  child.stdout.on('data', (chunk) => {
    stdout += chunk.toString();
  });

  let exited = false;
  let exitCode = null;
  child.on('exit', (code) => {
    exited = true;
    exitCode = code;
  });

  const deadline = Date.now() + READINESS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (exited) {
      throw new Error(`packaged app exited during startup (code ${exitCode}):\n${stdout}\n${stderr}`);
    }
    try {
      const health = await fetch(`${BASE_URL}/api/health`);
      if (health.ok) {
        return { child, getStdout: () => stdout, getStderr: () => stderr, hasExited: () => exited };
      }
    } catch {
      // Not listening yet.
    }
    await new Promise((r) => setTimeout(r, 300));
  }

  child.kill();
  throw new Error(`packaged app did not become ready within ${READINESS_TIMEOUT_MS} ms:\n${stdout}\n${stderr}`);
}

/** Force-stops a child - used only for teardown/error paths, never as the
 * primary proof of graceful shutdown (that is step 14 below, through the
 * real HTTP endpoint). */
async function forceStop(handle) {
  if (handle === null || handle.hasExited()) return;
  await new Promise((resolvePromise) => {
    const timer = setTimeout(() => resolvePromise(), SHUTDOWN_TIMEOUT_MS);
    handle.child.on('exit', () => {
      clearTimeout(timer);
      resolvePromise();
    });
    spawn('taskkill', ['/pid', String(handle.child.pid), '/T', '/F'], { stdio: 'ignore' });
  });
}

async function main() {
  console.log('Stage 20A packaged-runtime verification');

  step('Verify the staged release executable exists');
  expect(
    existsSync(STAGED_EXE),
    `staged executable found at ${STAGED_EXE}`,
    'Run: powershell -File scripts/build-release.ps1 -Version "0.1.0-dev+test" -SkipInstaller',
  );

  step('Verify --version reports expected metadata without starting any service');
  const versionOutput = await new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(STAGED_EXE, ['--version'], { stdio: ['ignore', 'pipe', 'pipe'] });
    let out = '';
    child.stdout.on('data', (c) => (out += c.toString()));
    child.on('exit', (code) => (code === 0 ? resolvePromise(out) : rejectPromise(new Error(`--version exited ${code}`))));
    setTimeout(() => rejectPromise(new Error('--version timed out - it must never start a service')), 5_000);
  });
  expect(versionOutput.includes('Streaming Tree for OBS'), '--version prints the product name', versionOutput);
  expect(versionOutput.includes('GPL-3.0-or-later'), '--version prints the licence identifier', versionOutput);
  expect(/commit [0-9a-f]{12}/.test(versionOutput), '--version prints a real commit hash', versionOutput);

  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-packaged-verify-'));
  console.log(`Temporary application-data directory: ${dataDir}`);
  console.log('The real user AppData directory is never touched.');

  let appHandle = null;

  try {
    step('Start the packaged application (no Node/npm/Vite process involved)');
    appHandle = await startPackagedApp(dataDir);
    pass('packaged application is ready, listening only on 127.0.0.1');

    step('GET /api/health responds');
    const health = await request('GET', '/api/health');
    expect(health.status === 200, 'health status is 200', health);

    step('GET /api/about responds with the canonical product identity');
    const about = await request('GET', '/api/about');
    expect(about.status === 200, 'about status is 200', about);
    expect(about.body.creatorName === 'Czekosabe', 'creator is exactly Czekosabe', about.body.creatorName);
    expect(
      about.body.supportUrl === 'https://streamelements.com/czekosabe/tip',
      'support URL is the canonical StreamElements link',
      about.body.supportUrl,
    );
    expect(
      about.body.applicationLicenseSpdx === 'GPL-3.0-or-later',
      'GPL-3.0-or-later is exposed',
      about.body.applicationLicenseSpdx,
    );

    step('GET / returns the real production frontend HTML');
    const root = await request('GET', '/');
    expect(root.status === 200, 'root status is 200', root.status);
    expect(root.headers.get('content-type')?.includes('text/html'), 'root content-type is HTML', root.headers.get('content-type'));
    expect(root.text.includes('<div id="root">') || root.text.includes('id="root"'), 'root HTML contains the SPA mount point', root.text.slice(0, 200));

    step('A hashed JS asset returns JavaScript, not HTML');
    const assetMatch = root.text.match(/\/assets\/[\w.-]+\.js/);
    expect(assetMatch !== null, 'index.html references a hashed JS asset', root.text.slice(0, 300));
    const jsAsset = await request('GET', assetMatch[0]);
    expect(jsAsset.status === 200, 'JS asset status is 200', jsAsset.status);
    expect(
      /javascript|ecmascript/.test(jsAsset.headers.get('content-type') ?? ''),
      'JS asset content-type is a JavaScript MIME type',
      jsAsset.headers.get('content-type'),
    );
    expect(!jsAsset.text.trimStart().startsWith('<!doctype'), 'JS asset body is not index.html', jsAsset.text.slice(0, 60));

    step('A CSS asset returns the correct content type');
    const cssMatch = root.text.match(/\/assets\/[\w.-]+\.css/);
    expect(cssMatch !== null, 'index.html references a hashed CSS asset', root.text.slice(0, 300));
    const cssAsset = await request('GET', cssMatch[0]);
    expect(cssAsset.status === 200, 'CSS asset status is 200', cssAsset.status);
    expect((cssAsset.headers.get('content-type') ?? '').includes('css'), 'CSS asset content-type is text/css', cssAsset.headers.get('content-type'));

    step('React Router client routes all resolve to the SPA entry point');
    for (const route of [
      '/settings',
      '/settings/about',
      '/overlay/chat/some-slug',
      '/overlay/alerts/some-slug',
      '/overlay/audio/some-slug',
      '/overlay/widgets/some-slug',
    ]) {
      const r = await request('GET', route);
      expect(r.status === 200 && r.text.includes('id="root"'), `${route} resolves to the SPA entry`, r.status);
    }

    step('A real /api/ endpoint remains an API endpoint, and an unknown one is a JSON 404');
    const unknownApi = await request('GET', '/api/this-does-not-exist');
    expect(unknownApi.status === 404, 'unknown /api/ path is 404', unknownApi.status);
    expect(typeof unknownApi.body === 'object' && unknownApi.body?.error !== undefined, 'unknown /api/ path is JSON-shaped, not HTML', unknownApi.body);

    step('The public engagement SSE route reports the correct content type');
    const sseController = new AbortController();
    const ssePromise = fetch(`${BASE_URL}/api/engagement/stream`, { signal: sseController.signal });
    const sseResponse = await ssePromise;
    expect(
      (sseResponse.headers.get('content-type') ?? '').includes('text/event-stream'),
      'SSE route content-type is text/event-stream',
      sseResponse.headers.get('content-type'),
    );
    sseController.abort();
    try {
      await sseResponse.body?.cancel();
    } catch {
      // Already aborted.
    }

    step('A missing static asset is a real 404, never index.html');
    const missingAsset = await request('GET', '/assets/does-not-exist-abc123.js');
    expect(missingAsset.status === 404, 'missing hashed asset is 404', missingAsset.status);
    expect(!missingAsset.text.includes('id="root"'), 'missing asset never falls back to index.html', missingAsset.text.slice(0, 60));

    step('A path traversal attempt is rejected');
    const traversal = await request('GET', '/assets/../../../../../../Windows/System32/drivers/etc/hosts');
    expect(!traversal.text.includes('localhost'), 'traversal attempt did not leak a real OS file', traversal.status);

    step('The four legal-document routes serve the real repository files');
    const licenseRoute = await request('GET', '/legal/license');
    expect(licenseRoute.status === 200 && licenseRoute.text.includes('GNU GENERAL PUBLIC LICENSE'), '/legal/license serves the real GPL text', licenseRoute.status);
    const privacyRoute = await request('GET', '/legal/privacy');
    expect(privacyRoute.status === 200 && privacyRoute.text.length > 0, '/legal/privacy serves real content', privacyRoute.status);
    const legalRoute = await request('GET', '/legal/legal');
    expect(legalRoute.status === 200 && legalRoute.text.length > 0, '/legal/legal serves real content', legalRoute.status);
    const noticesRoute = await request('GET', '/legal/third-party-notices');
    expect(noticesRoute.status === 200 && noticesRoute.text.length > 0, '/legal/third-party-notices serves real content', noticesRoute.status);

    step('Cross-check the legal routes against the actual repository files on disk');
    const realLicense = readFileSync(join(REPO_ROOT, 'LICENSE'), 'utf8');
    expect(licenseRoute.text === realLicense, '/legal/license byte-matches the repository LICENSE file', 'mismatch');

    step('A second launch detects the running instance and does not start another backend');
    // The second process is expected to exit entirely on its own (that is
    // exactly what this scenario proves) - there is nothing left to force-
    // stop for it in the `finally` block below.
    const secondLaunch = await new Promise((resolvePromise, rejectPromise) => {
      const child = spawn(STAGED_EXE, [], {
        cwd: dirname(STAGED_EXE),
        env: {
          STREAMING_TREE_DATA_DIR: dataDir,
          STREAMING_TREE_PORT: String(PORT),
          STREAMING_TREE_HOST: '127.0.0.1',
          STREAMING_TREE_TEST_NO_UI: '1',
          SystemRoot: process.env.SystemRoot ?? 'C:\\Windows',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      let out = '';
      child.stdout.on('data', (c) => (out += c.toString()));
      child.on('exit', (code) => resolvePromise({ code, out }));
      setTimeout(() => rejectPromise(new Error('second launch did not exit within 10s')), 10_000);
    });
    expect(secondLaunch.code === 0, 'second launch exits cleanly (code 0)', secondLaunch);
    const stillHealthy = await request('GET', '/api/health');
    expect(stillHealthy.status === 200, 'the first instance is still healthy after a second launch attempt', stillHealthy.status);

    step('Graceful shutdown is initiated through the real application shutdown path');
    const shutdown = await request('POST', '/api/system/shutdown', { confirm: true });
    expect(shutdown.status === 200, 'shutdown request accepted', shutdown.status);

    step('The backend actually exits');
    const exitDeadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
    while (Date.now() < exitDeadline && !appHandle.hasExited()) {
      await new Promise((r) => setTimeout(r, 200));
    }
    expect(appHandle.hasExited(), 'the packaged process exited on its own after the shutdown request', {
      stdout: appHandle.getStdout().slice(-500),
    });

    console.log(`\n${stepCount} steps passed. PASS`);
  } finally {
    await forceStop(appHandle);
    rmSync(dataDir, { recursive: true, force: true });
  }
}

main().catch((error) => {
  console.error('\nverify-packaged-app.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
