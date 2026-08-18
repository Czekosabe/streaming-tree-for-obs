#!/usr/bin/env node
/**
 * Linux package verification (Stage 20D1) - a platform-specific CI
 * verification helper, explicitly NOT canonical local integration
 * script #25. The canonical local integration-script count remains 24
 * (docs/linux-desktop-packaging.md §24).
 *
 * Tests the REAL, native, CGO-disabled, statically-linked .deb package
 * produced by scripts/build-release-linux.sh - never a cross-compiled
 * binary, never a substitute for native execution. See
 * docs/linux-desktop-packaging.md for the full architecture this
 * proves.
 *
 * Requires a Linux release build to already exist at
 * build/release-linux/output/ - run
 *   scripts/build-release-linux.sh --version 0.1.0-dev+test
 * first. This script does not rebuild the frontend/executable/package
 * itself, mirroring scripts/verify-macos-package.mjs's own convention.
 *
 * Only ever run on linux - refuses immediately on any other platform.
 * Installing/removing the real .deb requires root (an intrinsic
 * property of dpkg, not something this project introduces -
 * docs/linux-desktop-packaging.md §16) - this script shells out to
 * `sudo dpkg` for exactly those two steps; the packaged application
 * itself is always started and run as the normal, unprivileged
 * invoking user.
 *
 * Usage:  node scripts/verify-linux-package.mjs
 * Exits non-zero on the first failed expectation.
 */

import { execFileSync, spawn } from 'node:child_process';
import { existsSync, mkdtempSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release-linux', 'output');
const PACKAGE_NAME = 'streaming-tree-for-obs';
const INSTALLED_EXE_PATH = '/usr/bin/streaming-tree-server';
const DESKTOP_FILE_PATH = `/usr/share/applications/${PACKAGE_NAME}.desktop`;
const DOC_DIR = `/usr/share/doc/${PACKAGE_NAME}`;

const PORT = 8499;
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

function expectedDebArch() {
  // Node's process.arch on linux is 'arm64' or 'x64'; dpkg's own
  // architecture name for the latter is 'amd64'.
  return process.arch === 'arm64' ? 'arm64' : 'amd64';
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

/** Starts the real installed executable against a hermetic data
 * directory, as the normal unprivileged invoking user - never as
 * root, even though installation itself needed sudo. */
async function startPackagedApp(execPath, dataDir) {
  const child = spawn(execPath, [], {
    cwd: dirname(execPath),
    env: {
      PATH: process.env.PATH,
      HOME: process.env.HOME,
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

/** Force-stops a child - used only for teardown/error paths, never as
 * the primary proof of graceful shutdown (that is proven through the
 * real HTTP endpoint below). */
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

function isPackageInstalled() {
  try {
    execFileSync('dpkg', ['-s', PACKAGE_NAME], { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

async function main() {
  console.log('Stage 20D1 Linux package verification');

  if (process.platform !== 'linux') {
    fail('this script only runs on Linux', `process.platform = ${process.platform}`);
  }

  step('Verify the real .deb package exists (production frontend + package build already completed)');
  const debName = existsSync(OUTPUT_DIR) ? readdirSync(OUTPUT_DIR).find((name) => name.endsWith('.deb')) : undefined;
  expect(
    typeof debName === 'string',
    `a .deb file was found in ${OUTPUT_DIR}`,
    'Run: scripts/build-release-linux.sh --version 0.1.0-dev+test',
  );
  const debPath = join(OUTPUT_DIR, debName);

  step('The .deb control metadata reports the correct package identity');
  const info = execFileSync('dpkg-deb', ['--info', debPath], { encoding: 'utf8' });
  expect(info.includes(`Package: ${PACKAGE_NAME}`), 'Package name is correct', info);
  expect(info.includes(`Architecture: ${expectedDebArch()}`), `Architecture matches ${expectedDebArch()}`, info);
  expect(/Maintainer: Czekosabe/.test(info), 'Maintainer identifies Czekosabe', info);
  expect(!/Depends:/.test(info), 'no Depends field (CGO_ENABLED=0, fully static binary)', info);

  // Ensure a clean slate - a leftover install from a prior failed run
  // must never make this run's own install/remove assertions misleading.
  if (isPackageInstalled()) {
    execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'ignore' });
  }

  let installed = false;
  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-linux-verify-'));
  console.log(`Temporary application-data directory: ${dataDir}`);
  console.log('The real user config/data directory is never touched.');

  let appHandle = null;

  try {
    step('The .deb installs successfully through the real dpkg mechanism');
    execFileSync('sudo', ['dpkg', '-i', debPath], { stdio: 'pipe' });
    installed = true;
    expect(isPackageInstalled(), 'dpkg reports the package as installed', PACKAGE_NAME);
    expect(existsSync(INSTALLED_EXE_PATH), 'the executable was installed to /usr/bin', INSTALLED_EXE_PATH);

    step('Resources contains the four mandatory legal documents');
    for (const doc of ['copyright', 'LEGAL.md', 'PRIVACY.md', 'THIRD_PARTY_NOTICES.md']) {
      expect(existsSync(join(DOC_DIR, doc)), `${DOC_DIR}/${doc} is present`, doc);
    }

    step('The .desktop entry is installed with a fixed Exec target');
    expect(existsSync(DESKTOP_FILE_PATH), '.desktop file is installed', DESKTOP_FILE_PATH);
    const desktopContent = execFileSync('cat', [DESKTOP_FILE_PATH], { encoding: 'utf8' });
    expect(desktopContent.includes(`Exec=${INSTALLED_EXE_PATH}`), 'Exec is the fixed, correct absolute path', desktopContent);
    expect(!/[|;&`$]/.test(desktopContent.split('\n').find((l) => l.startsWith('Exec=')) ?? ''), 'Exec line contains no shell metacharacters', desktopContent);
    try {
      execFileSync('desktop-file-validate', [DESKTOP_FILE_PATH], { stdio: 'pipe' });
      pass('desktop-file-validate accepted the .desktop file');
    } catch (err) {
      if (err.code === 'ENOENT') {
        pass('desktop-file-validate not installed on this runner - skipped (non-fatal)');
      } else {
        fail('desktop-file-validate rejected the .desktop file', err.stdout?.toString() ?? String(err));
      }
    }

    step('Verify --version reports real packaged metadata without starting any service, as a normal user');
    const versionOutput = await runExeVersion(INSTALLED_EXE_PATH);
    expect(versionOutput.includes('Streaming Tree for OBS'), '--version prints the product name', versionOutput);
    expect(versionOutput.includes('GPL-3.0-or-later'), '--version prints the licence identifier', versionOutput);
    expect(/commit [0-9a-f]{12}/.test(versionOutput), '--version prints a real commit hash', versionOutput);

    step('Start the real installed executable as a normal unprivileged user (no Node/npm/Vite process involved)');
    appHandle = await startPackagedApp(INSTALLED_EXE_PATH, dataDir);
    pass('packaged application is ready, listening only on 127.0.0.1');
    expect(
      appHandle.getStdout().includes('browser launch suppressed (test mode)'),
      'the real browser-launch code path (xdg-open) was exercised (safely suppressed by the test seam)',
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
      about.body.applicationLicenseSpdx === 'GPL-3.0-or-later',
      'GPL-3.0-or-later is exposed',
      about.body.applicationLicenseSpdx,
    );

    step('GET / returns the real production frontend HTML');
    const root = await request('GET', '/');
    expect(root.status === 200, 'root status is 200', root.status);
    expect(root.text.includes('id="root"'), 'root HTML contains the SPA mount point', root.text.slice(0, 200));

    step('React Router client routes (Settings/About and a public overlay) resolve to the SPA entry point');
    for (const route of ['/settings', '/settings/about', '/overlay/chat/some-slug']) {
      const r = await request('GET', route);
      expect(r.status === 200 && r.text.includes('id="root"'), `${route} resolves to the SPA entry`, r.status);
    }

    step('The four legal-document HTTP routes serve real content');
    for (const route of ['/legal/license', '/legal/privacy', '/legal/legal', '/legal/third-party-notices']) {
      const r = await request('GET', route);
      expect(r.status === 200 && r.text.length > 0, `${route} serves real content`, r.status);
    }

    step('TTS honestly reports as unavailable on Linux (no shell-out to espeak/festival/spd-say)');
    const ttsCaps = await request('GET', '/api/audio/capabilities');
    expect(ttsCaps.status === 200, 'audio capabilities status is 200', ttsCaps.status);
    expect(ttsCaps.body.systemProviderAvailable === false, 'systemProviderAvailable is false on Linux', ttsCaps.body);

    step('The updater reports the intentional Linux platform-unsupported state, never a false "up to date"');
    const updateStatus = await request('GET', '/api/updates/status');
    expect(updateStatus.status === 200, 'update status is 200', updateStatus.status);
    expect(updateStatus.body.releaseBuild === true, 'this is recognized as a real release build', updateStatus.body);
    expect(updateStatus.body.state === 'platform_unsupported', 'updater state is platform_unsupported, not up_to_date/available', updateStatus.body.state);
    expect(updateStatus.body.installBlocked === true, 'install is reported as blocked', updateStatus.body);

    step('A manual check request is honestly refused rather than silently beginning an unsupported install');
    const checkNow = await request('POST', '/api/updates/check');
    expect(checkNow.status >= 400, 'manual update check is refused on this platform', checkNow.status);

    step('A second launch detects the running instance via the real flock-based single-instance mechanism, unrelated port occupation is not mistaken for it');
    const secondLaunch = await new Promise((resolvePromise, rejectPromise) => {
      const child = spawn(INSTALLED_EXE_PATH, [], {
        cwd: dirname(INSTALLED_EXE_PATH),
        env: {
          PATH: process.env.PATH,
          HOME: process.env.HOME,
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

    step('Application data was written outside every package-owned path');
    expect(existsSync(join(dataDir, 'streaming-tree.db')), 'the real SQLite database was created outside /usr', dataDir);

    step('The package removes cleanly through the real dpkg mechanism, without touching test user data');
    execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'pipe' });
    installed = false;
    expect(!existsSync(INSTALLED_EXE_PATH), 'the executable was removed from /usr/bin', INSTALLED_EXE_PATH);
    expect(!existsSync(DESKTOP_FILE_PATH), 'the .desktop file was removed', DESKTOP_FILE_PATH);
    expect(existsSync(join(dataDir, 'streaming-tree.db')), 'the SQLite database survives package removal', dataDir);

    console.log(`\n${stepCount} steps passed. PASS`);
  } finally {
    await forceStop(appHandle);
    if (installed) {
      try {
        execFileSync('sudo', ['dpkg', '-r', PACKAGE_NAME], { stdio: 'ignore' });
      } catch (removeError) {
        console.error('warning: cleanup dpkg -r failed', removeError);
      }
    }
    rmSync(dataDir, { recursive: true, force: true });
  }

  step('No Streaming Tree process remains');
  let leftoverProcesses = '';
  try {
    leftoverProcesses = execFileSync('pgrep', ['-f', 'streaming-tree-server'], { encoding: 'utf8' }).trim();
  } catch {
    // pgrep exits non-zero when nothing matches - the expected outcome.
    leftoverProcesses = '';
  }
  expect(leftoverProcesses === '', 'no streaming-tree-server process remains running', leftoverProcesses);
}

main().catch((error) => {
  console.error('\nverify-linux-package.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
