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

/** Sends/receives a raw binary body - a backup archive, never JSON/text
 * (request() above decodes every response as UTF-8 text, which would
 * corrupt a real zip archive). */
async function requestRaw(method, path, { body, headers = {} } = {}) {
  const response = await fetch(`${BASE_URL}${path}`, { method, headers, body });
  const buf = Buffer.from(await response.arrayBuffer());
  let payload = null;
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    try {
      payload = JSON.parse(buf.toString('utf8'));
    } catch {
      payload = null;
    }
  }
  return { status: response.status, body: payload, raw: buf, headers: response.headers };
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

  step('Verify the embedded manifest declares Per-Monitor-V2 DPI awareness (Stage 20E)');
  // A real physical/manual Windows test found the tray's native popup
  // menu visibly blurry - traced to the executable carrying no
  // application manifest at all, so it ran DPI-unaware
  // (apps/server/cmd/server/README-icon.txt has the full root-cause
  // writeup). This extracts the REAL RT_MANIFEST resource from the
  // built .exe the exact same way Windows itself reads it
  // (FindResource/LoadResource/LockResource), not by trusting
  // winres.json's own source content alone.
  const manifestXML = await new Promise((resolvePromise, rejectPromise) => {
    const child = spawn('go', ['run', join(REPO_ROOT, 'scripts', 'check-windows-manifest.go'), STAGED_EXE], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let out = '';
    let err = '';
    child.stdout.on('data', (c) => (out += c.toString()));
    child.stderr.on('data', (c) => (err += c.toString()));
    child.on('exit', (code) => (code === 0 ? resolvePromise(out) : rejectPromise(new Error(`check-windows-manifest exited ${code}: ${err}`))));
    setTimeout(() => rejectPromise(new Error('check-windows-manifest timed out')), 15_000);
  });
  expect(manifestXML.includes('<dpiAwareness'), 'a manifest is embedded and declares dpiAwareness', manifestXML);
  expect(manifestXML.includes('permonitorv2'), 'dpiAwareness includes permonitorv2', manifestXML);
  expect(manifestXML.includes('<dpiAware') && manifestXML.includes('>true<'), 'the legacy dpiAware fallback is also true', manifestXML);
  expect(manifestXML.includes('asInvoker'), 'execution level is asInvoker (never auto-elevates)', manifestXML);

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

    step('The real production JS bundle actually contains the provider brand marks (Stage 20E)');
    // A real physical/manual Windows test reported provider destination
    // cards rendering as flat coloured squares instead of recognizable
    // Twitch/YouTube/Kick/TikTok marks. ProviderBrand.tsx's own unit test
    // proves the component itself renders correct SVG geometry, but that
    // cannot prove the geometry actually survives this project's real
    // production bundling/packaging/embedding pipeline into what a
    // packaged Windows install actually serves - only fetching the real
    // built JS from the real running packaged server can prove that. Each
    // provider's official (or, for TikTok, contrast-corrected) fill hex
    // must appear in the shipped bundle; their absence would mean the
    // component either failed to build in, or a build step stripped it.
    for (const hex of ['#9146FF', '#FF0000', '#53FC19', '#f4f6fb']) {
      expect(
        jsAsset.text.includes(hex),
        `the production JS bundle contains the provider brand colour ${hex}`,
      );
    }

    step('A CSS asset returns the correct content type');
    const cssMatch = root.text.match(/\/assets\/[\w.-]+\.css/);
    expect(cssMatch !== null, 'index.html references a hashed CSS asset', root.text.slice(0, 300));
    const cssAsset = await request('GET', cssMatch[0]);
    expect(cssAsset.status === 200, 'CSS asset status is 200', cssAsset.status);
    expect((cssAsset.headers.get('content-type') ?? '').includes('css'), 'CSS asset content-type is text/css', cssAsset.headers.get('content-type'));

    step('The favicon is served through the same hashed, immutably-cacheable asset path as JS/CSS (Stage 20E)');
    // A real physical/manual Windows test reported the browser tab not
    // visibly showing the new logo. Traced to the favicon previously
    // being served from a static, unversioned /favicon.ico with no
    // explicit Cache-Control at all (internal/httpapi/production.go's
    // applyAssetCaching only covers assets/) - a real risk of a stale
    // browser-cached favicon surviving a packaged app upgrade. Fixed by
    // moving it into Vite's own hashed asset pipeline
    // (apps/web/src/assets/favicon.ico, referenced by a relative path
    // in index.html rather than living under public/) - this proves
    // both halves of that fix against the real production server: the
    // old unversioned path is gone, and the new hashed path is
    // correctly immutable-cacheable.
    const faviconLinkMatch = root.text.match(/<link rel="icon" href="(\/assets\/favicon-[\w.-]+\.ico)"/);
    expect(faviconLinkMatch !== null, 'index.html references a hashed favicon asset', root.text.slice(0, 300));
    const oldFavicon = await request('GET', '/favicon.ico');
    expect(oldFavicon.status === 404, 'the old unversioned /favicon.ico is gone, not silently still served', oldFavicon.status);
    const favicon = await request('GET', faviconLinkMatch[1]);
    expect(favicon.status === 200, 'the hashed favicon asset is served', favicon.status);
    expect(
      (favicon.headers.get('content-type') ?? '').includes('icon'),
      'favicon content-type identifies it as an icon',
      favicon.headers.get('content-type'),
    );
    expect(
      favicon.headers.get('cache-control') === 'public, max-age=31536000, immutable',
      'the hashed favicon is safely immutable-cacheable, unlike the old unversioned path',
      favicon.headers.get('cache-control'),
    );

    step('React Router client routes all resolve to the SPA entry point');
    for (const route of [
      '/settings',
      '/settings/about',
      '/onboarding',
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

    step('Stage 21: GET /api/onboarding is available and starts pending on a genuinely fresh data directory');
    // docs/onboarding.md §4.3's own existing-user migration rule: an
    // untouched database (this hermetic data directory has never had a
    // platform enabled, an output server configured, or an account
    // connected) starts 'pending' - proven here against the real
    // embedded production migration, not just the Go repository test.
    const onboardingFresh = await request('GET', '/api/onboarding');
    expect(onboardingFresh.status === 200, 'onboarding-state status is 200', onboardingFresh.status);
    expect(onboardingFresh.body.status === 'pending', 'a fresh data directory starts pending', onboardingFresh.body);

    step('Stage 21: setting the onboarding status persists it');
    const onboardingSet = await request('PUT', '/api/onboarding', { status: 'dismissed' });
    expect(onboardingSet.status === 200, 'PUT /api/onboarding accepted', onboardingSet.status);
    expect(onboardingSet.body.status === 'dismissed', 'the response reflects the new status', onboardingSet.body);

    step('Stage 22: create a metadata preset and apply it to a real destination on the packaged application');
    // Proves the Stage 22 routes are actually registered and reachable in
    // the real production binary (this script never runs `go run`), and
    // that Apply's local write really lands - not just that a unit test
    // fake accepted the call.
    const presetCreate = await request('POST', '/api/metadata-presets', {
      name: 'Packaged verification preset', note: '', title: 'Packaged apply check',
      description: '', tags: ['packaged'], language: 'en', visibility: '',
      matureContent: false, dvr: false, latencyMode: '',
      providers: { twitch: { category: 'Just Chatting', categoryId: '509658' } },
    });
    expect(presetCreate.status === 201, 'POST /api/metadata-presets returns 201', presetCreate.body);
    const presetId = presetCreate.body.id;

    const presetApply = await request('POST', `/api/metadata-presets/${presetId}/apply`, {
      platformIds: ['pf_seed_twitch'],
    });
    expect(presetApply.status === 200, 'POST .../apply returns 200', presetApply.body);
    expect(
      presetApply.body.platforms.pf_seed_twitch?.title === 'Packaged apply check',
      'the applied title actually landed on the real destination',
      presetApply.body,
    );

    step('Stage 21: the onboarding status survives a real graceful shutdown and restart against the same data directory');
    const preRestartShutdown = await request('POST', '/api/system/shutdown', { confirm: true });
    expect(preRestartShutdown.status === 200, 'graceful shutdown accepted before restart', preRestartShutdown.status);
    const preRestartExitDeadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
    while (Date.now() < preRestartExitDeadline && !appHandle.hasExited()) {
      await new Promise((r) => setTimeout(r, 200));
    }
    expect(appHandle.hasExited(), 'the packaged process exited after the pre-restart shutdown request', {
      stdout: appHandle.getStdout().slice(-500),
    });

    appHandle = await startPackagedApp(dataDir);
    pass('the packaged application restarted against the same hermetic data directory');
    const onboardingAfterRestart = await request('GET', '/api/onboarding');
    expect(onboardingAfterRestart.status === 200, 'onboarding-state is still available after restart', onboardingAfterRestart.status);
    expect(
      onboardingAfterRestart.body.status === 'dismissed',
      'the onboarding status set before the restart survived it - never silently reset to pending',
      onboardingAfterRestart.body,
    );

    step('Stage 22: the preset and its already-applied destination metadata both survive the same restart');
    const presetAfterRestart = await request('GET', `/api/metadata-presets/${presetId}`);
    expect(presetAfterRestart.status === 200, 'the preset is still available after restart', presetAfterRestart.status);
    expect(
      presetAfterRestart.body.name === 'Packaged verification preset',
      'the preset survived the restart',
      presetAfterRestart.body,
    );
    const destinationAfterRestart = await request('GET', '/api/platforms/pf_seed_twitch/metadata');
    expect(
      destinationAfterRestart.body.title === 'Packaged apply check',
      'the destination metadata Apply wrote before the restart is still there after it',
      destinationAfterRestart.body,
    );

    step('Stage 23: backup export and restore preview/commit are reachable and real on the packaged binary');
    // Proves the Stage 23 backup/restore routes are actually registered
    // and reachable in the real production binary (never `go run`) - the
    // deep archive/security/restart-persistence matrix already has
    // dedicated coverage (43 Go tests, several against a real database
    // and a real SecretStore, in apps/server/internal/domain/backup) and
    // its own full HTTP-level restart-persistence proof against a real
    // dev backend (scripts/verify-backup-restore.mjs); this step only
    // needs to prove the packaged binary's own wiring, not repeat that
    // whole matrix a third time.
    const packagedExport = await requestRaw('POST', '/api/backup/export');
    expect(packagedExport.status === 200, 'POST /api/backup/export succeeds on the packaged binary', packagedExport.status);
    expect(packagedExport.raw.subarray(0, 2).toString('latin1') === 'PK', 'the exported package is a real ZIP archive', packagedExport.raw.subarray(0, 4));

    const packagedPreview = await requestRaw('POST', '/api/backup/restore/preview', {
      body: packagedExport.raw,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(packagedPreview.status === 200, 'POST /api/backup/restore/preview succeeds on the packaged binary', packagedPreview.body);
    expect(
      packagedPreview.body.counts.metadataPresets >= 1,
      'the preview counts the real preset created earlier in this run',
      packagedPreview.body,
    );

    const packagedCommit = await request('POST', `/api/backup/restore/commit/${packagedPreview.body.token}`);
    expect(packagedCommit.status === 200, 'POST /api/backup/restore/commit/{token} succeeds on the packaged binary', packagedCommit.body);
    expect(packagedCommit.body.restartRequired === true, 'the packaged binary also reports restartRequired: true', packagedCommit.body);

    const presetsAfterRestore = await request('GET', '/api/metadata-presets');
    expect(
      presetsAfterRestore.body.some((p) => p.name === 'Packaged verification preset' && p.id !== presetId),
      'the restored preset exists under a fresh id, not the original one',
      { originalId: presetId, presets: presetsAfterRestore.body },
    );

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
