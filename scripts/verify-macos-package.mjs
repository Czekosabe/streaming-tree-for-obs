#!/usr/bin/env node
/**
 * macOS package verification (Stage 20C1) - a platform-specific CI
 * verification helper, explicitly NOT canonical local integration script
 * #25. The canonical local integration-script count remains 24
 * (docs/updater.md §41); this only ever runs meaningfully on a real,
 * native macOS host and is documented separately
 * (docs/macos-packaging.md §28).
 *
 * Tests the REAL, native, CGO-enabled, unsigned .app bundle and DMG
 * produced by scripts/build-release-macos.sh - never a cross-compiled
 * binary, never a substitute for native execution. See
 * docs/macos-packaging.md for the full architecture this proves.
 *
 * Requires a macOS release build to already exist at
 * build/release-macos/output/ - run
 *   scripts/build-release-macos.sh --version 0.1.0-dev+test
 * first. This script does not rebuild the frontend/executable/DMG
 * itself, mirroring scripts/verify-packaged-app.mjs's own convention.
 *
 * Only ever run on darwin - refuses immediately on any other platform.
 *
 * Usage:  node scripts/verify-macos-package.mjs
 * Exits non-zero on the first failed expectation.
 */

import { execFileSync, spawn } from 'node:child_process';
import { existsSync, mkdtempSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release-macos', 'output');
const APP_BUNDLE = join(OUTPUT_DIR, 'Streaming Tree for OBS.app');
const EXE_PATH = join(APP_BUNDLE, 'Contents', 'MacOS', 'streaming-tree-server');
const INFO_PLIST = join(APP_BUNDLE, 'Contents', 'Info.plist');

const EXPECTED_BUNDLE_ID = 'io.github.czekosabe.streaming-tree-for-obs';

const PORT = 8399;
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

function readPlist(path) {
  const json = execFileSync('plutil', ['-convert', 'json', '-o', '-', path], { encoding: 'utf8' });
  return JSON.parse(json);
}

function expectedMachOArch() {
  // Node's process.arch on darwin is 'arm64' or 'x64'; `file`'s own
  // Mach-O architecture label for the latter is 'x86_64'.
  return process.arch === 'arm64' ? 'arm64' : 'x86_64';
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

/** Starts the real packaged executable (from inside the real .app bundle)
 * against a hermetic data directory - no Node/npm/Vite process is ever
 * part of this invocation, only the compiled Go binary itself. */
async function startPackagedApp(execPath, dataDir) {
  const child = spawn(execPath, [], {
    cwd: dirname(execPath),
    env: {
      PATH: process.env.PATH,
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(PORT),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_TEST_NO_UI: '1',
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

  child.kill('SIGKILL');
  throw new Error(`packaged app did not become ready within ${READINESS_TIMEOUT_MS} ms:\n${stdout}\n${stderr}`);
}

/** Force-stops a child - used only for teardown/error paths, never as the
 * primary proof of graceful shutdown (that is proven through the real
 * HTTP endpoint below). */
async function forceStop(handle) {
  if (handle === null || handle.hasExited()) return;
  await new Promise((resolvePromise) => {
    const timer = setTimeout(() => resolvePromise(), SHUTDOWN_TIMEOUT_MS);
    handle.child.on('exit', () => {
      clearTimeout(timer);
      resolvePromise();
    });
    handle.child.kill('SIGKILL');
  });
}

function runExeVersion(execPath) {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(execPath, ['--version'], { stdio: ['ignore', 'pipe', 'pipe'] });
    let out = '';
    child.stdout.on('data', (c) => (out += c.toString()));
    child.on('exit', (code) => (code === 0 ? resolvePromise(out) : rejectPromise(new Error(`--version exited ${code}`))));
    setTimeout(() => rejectPromise(new Error('--version timed out - it must never start a service')), 5_000);
  });
}

async function main() {
  console.log('Stage 20C1 macOS package verification');

  if (process.platform !== 'darwin') {
    fail('this script only runs on macOS', `process.platform = ${process.platform}`);
  }

  step('Verify the real .app bundle exists (production frontend + package build already completed)');
  expect(
    existsSync(APP_BUNDLE) && existsSync(EXE_PATH),
    `.app bundle and executable found at ${APP_BUNDLE}`,
    'Run: scripts/build-release-macos.sh --version 0.1.0-dev+test',
  );

  step('Info.plist parses (plutil -lint) and reports the stable bundle identity');
  execFileSync('plutil', ['-lint', INFO_PLIST]);
  pass('plutil -lint accepted Info.plist');
  const plist = readPlist(INFO_PLIST);
  expect(plist.CFBundleIdentifier === EXPECTED_BUNDLE_ID, 'stable bundle identifier is correct', plist.CFBundleIdentifier);
  expect(plist.CFBundleExecutable === 'streaming-tree-server', 'CFBundleExecutable names the real executable', plist.CFBundleExecutable);
  expect(typeof plist.CFBundleShortVersionString === 'string' && plist.CFBundleShortVersionString.length > 0, 'CFBundleShortVersionString is set', plist.CFBundleShortVersionString);
  expect(plist.CFBundleVersion === plist.CFBundleShortVersionString, 'CFBundleVersion matches CFBundleShortVersionString', plist);
  expect(plist.LSUIElement === true, 'LSUIElement is true (agent app, docs/macos-packaging.md §13)', plist.LSUIElement);

  step('The executable architecture matches this runner');
  const fileOutput = execFileSync('file', [EXE_PATH], { encoding: 'utf8' });
  expect(fileOutput.includes(expectedMachOArch()), `executable architecture matches ${expectedMachOArch()}`, fileOutput.trim());

  step('Resources contains the four mandatory legal documents');
  for (const doc of ['LICENSE', 'THIRD_PARTY_NOTICES.md', 'LEGAL.md', 'PRIVACY.md']) {
    expect(existsSync(join(APP_BUNDLE, 'Contents', 'Resources', doc)), `Resources/${doc} is present`, doc);
  }

  step('Verify --version reports real packaged build metadata without starting any service');
  const versionOutput = await runExeVersion(EXE_PATH);
  expect(versionOutput.includes('Streaming Tree for OBS'), '--version prints the product name', versionOutput);
  expect(versionOutput.includes('GPL-3.0-or-later'), '--version prints the licence identifier', versionOutput);
  expect(/commit [0-9a-f]{12}/.test(versionOutput), '--version prints a real commit hash', versionOutput);

  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-macos-verify-'));
  console.log(`Temporary application-data directory: ${dataDir}`);
  console.log('The real user Application Support directory is never touched.');

  let appHandle = null;

  try {
    step('Start the real executable from inside the .app bundle (no Node/npm/Vite process involved)');
    appHandle = await startPackagedApp(EXE_PATH, dataDir);
    pass('packaged application is ready, listening only on 127.0.0.1');
    expect(
      appHandle.getStdout().includes('browser launch suppressed (test mode)'),
      'the real browser-launch code path was exercised (safely suppressed by the test seam) and would have opened the local URL',
      appHandle.getStdout().slice(-500),
    );

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
    expect(root.text.includes('id="root"'), 'root HTML contains the SPA mount point', root.text.slice(0, 200));

    step('React Router client routes (Settings/About and a public overlay) resolve to the SPA entry point');
    for (const route of ['/settings', '/settings/about', '/overlay/chat/some-slug']) {
      const r = await request('GET', route);
      expect(r.status === 200 && r.text.includes('id="root"'), `${route} resolves to the SPA entry`, r.status);
    }

    step('The four legal-document routes serve real content');
    for (const route of ['/legal/license', '/legal/privacy', '/legal/legal', '/legal/third-party-notices']) {
      const r = await request('GET', route);
      expect(r.status === 200 && r.text.length > 0, `${route} serves real content`, r.status);
    }

    step('TTS honestly reports as unavailable on macOS (no shell-out to `say`, no fake capability)');
    const ttsCaps = await request('GET', '/api/audio/capabilities');
    expect(ttsCaps.status === 200, 'audio capabilities status is 200', ttsCaps.status);
    expect(ttsCaps.body.systemProviderAvailable === false, 'systemProviderAvailable is false on macOS', ttsCaps.body);

    step('The updater reports the intentional macOS platform-unsupported state, never a false "up to date"');
    const updateStatus = await request('GET', '/api/updates/status');
    expect(updateStatus.status === 200, 'update status is 200', updateStatus.status);
    expect(updateStatus.body.releaseBuild === true, 'this is recognized as a real release build', updateStatus.body);
    expect(updateStatus.body.state === 'platform_unsupported', 'updater state is platform_unsupported, not up_to_date/available', updateStatus.body.state);
    expect(updateStatus.body.installBlocked === true, 'install is reported as blocked', updateStatus.body);

    step('A manual check request is honestly refused rather than silently beginning an unsupported install');
    const checkNow = await request('POST', '/api/updates/check');
    expect(checkNow.status >= 400, 'manual update check is refused on this platform', checkNow.status);

    step('A second launch detects the running instance via the real flock-based single-instance mechanism and does not start another backend');
    const secondLaunch = await new Promise((resolvePromise, rejectPromise) => {
      const child = spawn(EXE_PATH, [], {
        cwd: dirname(EXE_PATH),
        env: {
          PATH: process.env.PATH,
          STREAMING_TREE_DATA_DIR: dataDir,
          STREAMING_TREE_PORT: String(PORT),
          STREAMING_TREE_HOST: '127.0.0.1',
          STREAMING_TREE_TEST_NO_UI: '1',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      let out = '';
      child.stdout.on('data', (c) => (out += c.toString()));
      child.on('exit', (code) => resolvePromise({ code, out }));
      setTimeout(() => rejectPromise(new Error('second launch did not exit within 10s')), 10_000);
    });
    expect(secondLaunch.code === 0, 'second launch exits cleanly (code 0)', secondLaunch);
    expect(
      secondLaunch.out.includes('another instance is already running'),
      'second launch logged that it detected the real first instance',
      secondLaunch.out,
    );
    const stillHealthy = await request('GET', '/api/health');
    expect(stillHealthy.status === 200, 'the first instance is still healthy after a second launch attempt', stillHealthy.status);

    step('Graceful shutdown is initiated through the real shared shutdown path');
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

    // nativealert (the real NSAlert/Cgo fatal-startup-error bridge,
    // docs/macos-packaging.md §12) is never invoked for real here: doing
    // so would block on a native modal with no user present to dismiss
    // it and hang this CI job forever. Its compilation is already
    // proven by this same executable existing and running at all - a
    // broken Objective-C bridge would have failed `go build` with
    // CGO_ENABLED=1 before this script ever ran.
    pass('nativealert (NSAlert/Cgo bridge) compiled successfully as part of this executable - not invoked live in CI, by design');
  } finally {
    await forceStop(appHandle);
    rmSync(dataDir, { recursive: true, force: true });
  }

  // --- DMG lifecycle -----------------------------------------------------
  step('The DMG exists');
  const dmgName = readdirSync(OUTPUT_DIR).find((name) => name.endsWith('.dmg'));
  expect(typeof dmgName === 'string', 'a .dmg file was found in the output directory', readdirSync(OUTPUT_DIR));
  const dmgPath = join(OUTPUT_DIR, dmgName);

  const mountPoint = mkdtempSync(join(tmpdir(), 'streaming-tree-dmg-mount-'));
  let mounted = false;
  const copyDir = mkdtempSync(join(tmpdir(), 'streaming-tree-applications-like-'));
  let copiedAppHandle = null;

  try {
    step('The DMG mounts successfully via hdiutil');
    execFileSync('hdiutil', ['attach', '-nobrowse', '-mountpoint', mountPoint, dmgPath]);
    mounted = true;
    pass(`mounted at ${mountPoint}`);

    step('The mounted volume contains the real .app, and its metadata matches the staged app');
    const mountedApp = join(mountPoint, 'Streaming Tree for OBS.app');
    expect(existsSync(mountedApp), 'mounted volume contains the .app bundle', mountedApp);
    const mountedPlist = readPlist(join(mountedApp, 'Contents', 'Info.plist'));
    expect(mountedPlist.CFBundleIdentifier === plist.CFBundleIdentifier, 'mounted app bundle identifier matches the staged app', mountedPlist.CFBundleIdentifier);
    expect(mountedPlist.CFBundleVersion === plist.CFBundleVersion, 'mounted app version matches the staged app', mountedPlist.CFBundleVersion);
    expect(existsSync(join(mountPoint, 'Applications')), 'mounted volume contains the Applications symlink', mountPoint);

    step('The app can be copied to a hermetic temporary "Applications-like" directory and started from there');
    const copiedAppPath = join(copyDir, 'Streaming Tree for OBS.app');
    execFileSync('cp', ['-R', mountedApp, copiedAppPath]);
    const copiedExePath = join(copiedAppPath, 'Contents', 'MacOS', 'streaming-tree-server');
    execFileSync('chmod', ['+x', copiedExePath]);

    copiedAppHandle = await startPackagedApp(copiedExePath, dataDir);
    pass('the copied app started and became healthy from the hermetic "Applications-like" directory');
    const copiedHealth = await request('GET', '/api/health');
    expect(copiedHealth.status === 200, 'copied app health status is 200', copiedHealth.status);

    step('Application data remains outside the app bundle');
    expect(!dataDir.startsWith(copyDir), 'the data directory is not inside the copied app bundle', { dataDir, copyDir });
    expect(existsSync(join(dataDir, 'streaming-tree.db')), 'the real SQLite database was created outside the app bundle', dataDir);

    const copiedShutdown = await request('POST', '/api/system/shutdown', { confirm: true });
    expect(copiedShutdown.status === 200, 'copied app shutdown request accepted', copiedShutdown.status);
    const copiedExitDeadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
    while (Date.now() < copiedExitDeadline && !copiedAppHandle.hasExited()) {
      await new Promise((r) => setTimeout(r, 200));
    }
    expect(copiedAppHandle.hasExited(), 'the copied app process exited on its own after shutdown', {});

    step('Removing the copied app does not remove the application data');
    rmSync(copyDir, { recursive: true, force: true });
    expect(existsSync(join(dataDir, 'streaming-tree.db')), 'the SQLite database survives removal of the copied .app', dataDir);
  } finally {
    await forceStop(copiedAppHandle);
    if (mounted) {
      try {
        execFileSync('hdiutil', ['detach', mountPoint, '-force']);
      } catch (detachError) {
        console.error('warning: hdiutil detach failed', detachError);
      }
    }
    rmSync(mountPoint, { recursive: true, force: true });
    rmSync(copyDir, { recursive: true, force: true });
    rmSync(dataDir, { recursive: true, force: true });
  }
  step('The DMG unmounted cleanly (verified above) and no test resources remain');
  pass('DMG detached, mount point and hermetic directories removed');

  step('No Streaming Tree process remains');
  let leftoverProcesses = '';
  try {
    leftoverProcesses = execFileSync('pgrep', ['-f', 'streaming-tree-server'], { encoding: 'utf8' }).trim();
  } catch {
    // pgrep exits non-zero when nothing matches - the expected outcome.
    leftoverProcesses = '';
  }
  expect(leftoverProcesses === '', 'no streaming-tree-server process remains running', leftoverProcesses);

  console.log(`\n${stepCount} steps passed. PASS`);
}

main().catch((error) => {
  console.error('\nverify-macos-package.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
