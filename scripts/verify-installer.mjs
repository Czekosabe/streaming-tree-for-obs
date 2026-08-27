#!/usr/bin/env node
/**
 * Windows installer smoke verification (Stage 20A/20E).
 *
 * A separate helper, not integration script #23 (that is
 * scripts/verify-packaged-app.mjs) - see docs/windows-packaging.md §23.
 * Drives the REAL Inno Setup-produced installer through its own documented
 * silent-install flags into a throwaway, non-default location - never the
 * operator's real per-user install path - then a real silent uninstall.
 *
 * Two scenarios (docs/windows-packaging.md §26):
 *   A. Ordinary uninstall (no purge flag) - install, run, uninstall,
 *      verify program files are gone but the data directory survives,
 *      then reinstall over the same location and verify the
 *      application starts up healthy again against that preserved
 *      data directory.
 *   B. Explicit purge uninstall (STREAMING_TREE_TEST_PURGE_USER_DATA=1
 *      on the uninstaller's own environment - the hook
 *      InitializeUninstall exposes for exactly this kind of automated
 *      test, since there is no GUI checkbox to click under
 *      /VERYSILENT, and a real Windows CI run found a custom command-
 *      line switch does not survive Inno's own uninstaller-relaunch-
 *      from-TEMP mechanism the way an environment variable does - see
 *      ShouldPurgeUserDataForTest's own doc comment in the .iss) -
 *      verify the data directory is fully removed, and that a
 *      sentinel file outside it is never touched.
 *
 * Both scenarios use only hermetic, throwaway install/data directories
 * - the real per-user install location and real AppData are never
 * touched. Neither scenario stores a real OS-credential-store secret
 * (the purge helper's own credential-deletion correctness is unit-
 * tested hermetically against a fake store in
 * apps/server/internal/userdatapurge - see that package's own tests -
 * so this script never has to touch the real Windows Credential
 * Manager to prove it).
 *
 * Requires an installer to already exist at build/release/output/*.exe -
 * run scripts/build-release.ps1 first (without -SkipInstaller).
 *
 * Usage:  node scripts/verify-installer.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createReadStream, existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release', 'output');
const PORT = 8298;
const BASE_URL = `http://127.0.0.1:${PORT}`;

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
  if (condition) return pass(message);
  fail(message, detail);
}

function run(command, args, options = {}) {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'], ...options });
    let out = '';
    let err = '';
    child.stdout.on('data', (c) => (out += c.toString()));
    child.stderr.on('data', (c) => (err += c.toString()));
    child.on('exit', (code) => resolvePromise({ code, out, err }));
    child.on('error', rejectPromise);
  });
}

/** Lowercase-hex SHA-256 of path, computed natively in Node - no
 * external `powershell.exe`/`Get-FileHash` subprocess. Removes an
 * entire class of failure this script had no way to diagnose (a
 * subprocess silently producing empty stdout on a real CI run, with
 * its real stderr never surfaced) - hashing is exactly the kind of
 * operation Node's own standard library already does natively and
 * deterministically, with no platform-specific quoting/PATH/shell
 * concerns at all. */
function sha256File(path) {
  return new Promise((resolvePromise, rejectPromise) => {
    const hash = createHash('sha256');
    const stream = createReadStream(path);
    stream.on('data', (chunk) => hash.update(chunk));
    stream.on('end', () => resolvePromise(hash.digest('hex')));
    stream.on('error', rejectPromise);
  });
}

/** Spawns the installed executable against dataDir/port, polls
 * /api/health until it answers (or the process exits first), and
 * returns { appProcess, exitedRef } - exitedRef.exited flips true the
 * moment the process actually exits, read by callers that need to
 * poll for it afterward. Shared by all scenarios below so the same
 * real health/shutdown behavior is verified identically in each.
 * extraEnv lets a caller opt back into the real tray window
 * (STREAMING_TREE_TEST_KEEP_TRAY=1) while STREAMING_TREE_TEST_NO_UI
 * still suppresses the browser launch - see config.TestKeepTray's own
 * doc comment. */
async function startAppAndWaitHealthy(exePath, dataDir, port, extraEnv = {}) {
  const appProcess = spawn(exePath, [], {
    env: {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(port),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_TEST_NO_UI: '1',
      SystemRoot: process.env.SystemRoot ?? 'C:\\Windows',
      ...extraEnv,
    },
    stdio: ['ignore', 'ignore', 'ignore'],
  });
  const exitedRef = { exited: false };
  appProcess.on('exit', () => {
    exitedRef.exited = true;
  });

  const baseUrl = `http://127.0.0.1:${port}`;
  const deadline = Date.now() + 30_000;
  let healthy = false;
  while (Date.now() < deadline && !exitedRef.exited) {
    try {
      const health = await fetch(`${baseUrl}/api/health`);
      if (health.ok) {
        healthy = true;
        break;
      }
    } catch {
      // Not ready yet.
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  expect(healthy, 'the executable becomes healthy');
  return { appProcess, exitedRef, baseUrl };
}

/** Requests the real graceful-shutdown endpoint and waits for the
 * process to actually exit - the same endpoint the Windows
 * PrepareToInstall/InitializeUninstall cooperative-shutdown request
 * ultimately triggers via the tray's OnQuit callback
 * (docs/windows-packaging.md §26), exercised here directly over HTTP
 * since that is the deterministic, already-existing way to drive the
 * exact same shutdown path from a script. */
async function gracefulShutdownAndWait(baseUrl, exitedRef) {
  const shutdownResp = await fetch(`${baseUrl}/api/system/shutdown`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm: true }),
  });
  expect(shutdownResp.ok, 'graceful shutdown accepted');

  const stopDeadline = Date.now() + 15_000;
  while (Date.now() < stopDeadline && !exitedRef.exited) {
    await new Promise((r) => setTimeout(r, 200));
  }
  expect(exitedRef.exited, 'the executable exits after shutdown');
}

/** Scenario A (docs/windows-packaging.md §26): an ordinary uninstall,
 * with no purge flag, preserves the data directory - then a fresh
 * reinstall over the same location starts up healthy against that
 * preserved data, proving recovery is real and functional, not just
 * "a file happened to still exist." */
async function testOrdinaryUninstallAndReinstallScenario(installerPath) {
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-install-verify-')), 'app');
  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-install-data-'));
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Hermetic data directory: ${dataDir}`);
  console.log('The real per-user install location and real AppData are never touched.');

  let appProcess = null;

  try {
    step('Silent install into the hermetic directory');
    const install = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(install.code === 0, 'silent install exits 0', install);

    step('Verify the installed files exist');
    const exePath = join(installDir, 'streaming-tree-server.exe');
    for (const file of ['streaming-tree-server.exe', 'LICENSE', 'THIRD_PARTY_NOTICES.md', 'LEGAL.md', 'PRIVACY.md']) {
      expect(existsSync(join(installDir, file)), `${file} exists in the install directory`);
    }

    step('The installed executable reports --version correctly');
    const versionResult = await run(exePath, ['--version']);
    expect(versionResult.code === 0, '--version exits 0', versionResult);
    expect(versionResult.out.includes('Streaming Tree for OBS'), '--version prints the product name', versionResult.out);

    step('Create a test application-data marker before uninstalling');
    const markerPath = join(dataDir, 'uninstall-preservation-marker.txt');
    writeFileSync(markerPath, 'created by verify-installer.mjs');
    expect(existsSync(markerPath), 'marker file created');

    step('The installed executable starts and answers production health/API checks');
    const started = await startAppAndWaitHealthy(exePath, dataDir, PORT);
    appProcess = started.appProcess;

    const about = await fetch(`${started.baseUrl}/api/about`).then((r) => r.json());
    expect(about.creatorName === 'Czekosabe', 'installed executable serves the real About API', about);

    await gracefulShutdownAndWait(started.baseUrl, started.exitedRef);
    appProcess = null;

    step('Silent uninstall (no purge flag - the ordinary, default path)');
    const uninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(uninstallerFile !== undefined, 'uninstaller was created by the installer', readdirSync(installDir));
    const uninstall = await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    expect(uninstall.code === 0, 'silent uninstall exits 0', uninstall);

    step('Verify the installed application executable is gone');
    // The uninstaller's own process can still be exiting/self-deleting for
    // a brief moment after it reports success - poll briefly rather than
    // asserting immediately.
    const removalDeadline = Date.now() + 10_000;
    let removed = false;
    while (Date.now() < removalDeadline) {
      if (!existsSync(exePath)) {
        removed = true;
        break;
      }
      await new Promise((r) => setTimeout(r, 300));
    }
    expect(removed, 'streaming-tree-server.exe was removed by uninstall');

    step('Verify the hermetic application-data marker survived uninstall untouched');
    expect(existsSync(markerPath), 'the test data-directory marker file was NOT deleted by uninstall');

    step('Reinstall over the same location');
    const reinstall = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(reinstall.code === 0, 'reinstall exits 0', reinstall);
    expect(existsSync(exePath), 'streaming-tree-server.exe exists again after reinstall');
    expect(existsSync(markerPath), 'the marker file is still present after reinstall');

    step('The reinstalled executable starts up healthy against the preserved data directory');
    const restarted = await startAppAndWaitHealthy(exePath, dataDir, PORT);
    appProcess = restarted.appProcess;
    await gracefulShutdownAndWait(restarted.baseUrl, restarted.exitedRef);
    appProcess = null;
  } finally {
    if (appProcess !== null) {
      spawn('taskkill', ['/pid', String(appProcess.pid), '/T', '/F'], { stdio: 'ignore' });
    }
    rmSync(dirname(installDir), { recursive: true, force: true });
    rmSync(dataDir, { recursive: true, force: true });
  }
}

/** Scenario B (docs/windows-packaging.md §26): an explicit purge
 * uninstall - driven via STREAMING_TREE_TEST_PURGE_USER_DATA=1 on the
 * uninstaller's own environment, InitializeUninstall's own documented
 * automated-test hook for the path a GUI checkbox click would
 * otherwise gate under /VERYSILENT - removes the whole data directory,
 * and never touches anything outside it. */
async function testExplicitPurgeScenario(installerPath) {
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-purge-verify-')), 'app');
  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-purge-data-'));
  // A sentinel file OUTSIDE dataDir, in its own throwaway directory -
  // proves the purge never reaches beyond the one directory it owns,
  // the same structural proof apps/web's body-scroll-lock regression
  // test and the userdatapurge Go unit tests already use.
  const outsideDir = mkdtempSync(join(tmpdir(), 'streaming-tree-purge-outside-'));
  const sentinelPath = join(outsideDir, 'unrelated-file.txt');
  writeFileSync(sentinelPath, 'must survive the purge untouched');
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Hermetic data directory (to be purged): ${dataDir}`);
  console.log(`Sentinel directory (must survive): ${outsideDir}`);

  let appProcess = null;
  const purgePort = PORT + 1;

  try {
    step('Silent install into a second hermetic directory');
    const install = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(install.code === 0, 'silent install exits 0', install);

    const exePath = join(installDir, 'streaming-tree-server.exe');

    step('Create a real destination via the API, so the database is not empty when purged');
    const started = await startAppAndWaitHealthy(exePath, dataDir, purgePort);
    appProcess = started.appProcess;
    const created = await fetch(`${started.baseUrl}/api/platforms`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ providerId: 'twitch', displayName: 'Purge test destination', enabled: false }),
    });
    expect(created.status === 201, 'a real destination was created', created.status);

    const dbPath = join(dataDir, 'streaming-tree.db');
    expect(existsSync(dbPath), 'the database file exists before purge');

    await gracefulShutdownAndWait(started.baseUrl, started.exitedRef);
    appProcess = null;

    step('Silent uninstall with the explicit purge flag');
    const uninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(uninstallerFile !== undefined, 'uninstaller was created by the installer', readdirSync(installDir));
    // Both env vars must reach the uninstaller's own relaunched-temp-
    // copy child process (Inno Setup copies unins*.exe to TEMP and
    // relaunches it to do the actual removal, since a running process
    // cannot delete its own .exe - a real Windows CI run found that
    // relaunch reconstructs the command line using only switches Inno
    // itself recognizes, silently dropping a custom /PURGEUSERDATA
    // flag; environment variables survive it, since that relaunch is
    // still ordinary child-process creation - see
    // ShouldPurgeUserDataForTest's own doc comment in the .iss).
    // STREAMING_TREE_DATA_DIR: without it the purge helper's own
    // config.Load() falls back to the real default
    // %AppData%\StreamingTree instead of the hermetic test directory.
    // /LOG=: Inno's own detailed uninstall log - read on failure below
    // so a real "did [UninstallRun]'s Check: evaluate true, did the
    // purge command actually run, what did it exit with" answer is
    // available instead of guessing at another theory blind.
    const uninstallLogPath = join(tmpdir(), `streaming-tree-purge-uninstall-${Date.now()}.log`);
    const uninstall = await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES', `/LOG=${uninstallLogPath}`], {
      env: { ...process.env, STREAMING_TREE_DATA_DIR: dataDir, STREAMING_TREE_TEST_PURGE_USER_DATA: '1' },
    });
    expect(uninstall.code === 0, 'silent purge uninstall exits 0', uninstall);

    step('Verify the whole data directory was removed by the purge');
    const removalDeadline = Date.now() + 15_000;
    let removed = false;
    while (Date.now() < removalDeadline) {
      if (!existsSync(dataDir)) {
        removed = true;
        break;
      }
      await new Promise((r) => setTimeout(r, 300));
    }
    if (!removed && existsSync(uninstallLogPath)) {
      console.log('--- Inno uninstall log (diagnostic, purge did not remove the data directory) ---');
      console.log(readFileSync(uninstallLogPath, 'utf8'));
      console.log('--- end Inno uninstall log ---');
    }
    expect(removed, 'the data directory (database, assets, managed runtime) no longer exists', dataDir);

    step('Verify the sentinel file outside the data directory was never touched');
    expect(existsSync(sentinelPath), 'the unrelated sentinel file outside dataDir still exists');
  } finally {
    if (appProcess !== null) {
      spawn('taskkill', ['/pid', String(appProcess.pid), '/T', '/F'], { stdio: 'ignore' });
    }
    rmSync(dirname(installDir), { recursive: true, force: true });
    rmSync(dataDir, { recursive: true, force: true });
    rmSync(outsideDir, { recursive: true, force: true });
  }
}

/** Scenario C (docs/windows-packaging.md §26): the real physical
 * Windows failure this whole mechanism exists to fix - a newer
 * installer launched while the application is still running
 * previously required the operator to close it manually via Task
 * Manager. Leaves the application running (STREAMING_TREE_TEST_KEEP_
 * TRAY=1 keeps the real tray window alive so the installer's
 * PrepareToInstall has something real to find and message - see
 * config.TestKeepTray), then launches the installer over the same
 * location without shutting the running instance down first. A
 * successful, automatic completion here is the proof: the installer's
 * PrepareToInstall detected the running instance via the real
 * AppSingleInstanceMutex, requested cooperative shutdown through the
 * tray's hidden window, and waited for it to actually exit - all
 * without any manual intervention. */
async function testManualUpgradeWhileRunningScenario(installerPath) {
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-upgrade-verify-')), 'app');
  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-upgrade-data-'));
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Hermetic data directory: ${dataDir}`);

  let appProcess = null;
  const upgradePort = PORT + 2;

  try {
    step('Silent install into a third hermetic directory (manual-upgrade-while-running scenario)');
    const install = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(install.code === 0, 'silent install exits 0', install);

    const exePath = join(installDir, 'streaming-tree-server.exe');

    step('Start the application and leave it running - deliberately not shut down before the upgrade');
    const started = await startAppAndWaitHealthy(exePath, dataDir, upgradePort, { STREAMING_TREE_TEST_KEEP_TRAY: '1' });
    appProcess = started.appProcess;

    step('Launch a newer installer over the same location WHILE the application is still running');
    const upgrade = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(upgrade.code === 0, 'the installer completes automatically over a running instance - no manual close needed', upgrade);

    step('Verify the previously-running process actually exited cooperatively, not left orphaned');
    const exitDeadline = Date.now() + 5_000;
    while (Date.now() < exitDeadline && !started.exitedRef.exited) {
      await new Promise((r) => setTimeout(r, 200));
    }
    expect(started.exitedRef.exited, 'the running application process exited via cooperative shutdown');
    appProcess = null;

    step('Verify the installed files are intact after the upgrade');
    expect(existsSync(exePath), 'streaming-tree-server.exe exists after the upgrade');

    step('The upgraded executable still starts up healthy against the same data directory');
    const restarted = await startAppAndWaitHealthy(exePath, dataDir, upgradePort);
    appProcess = restarted.appProcess;
    await gracefulShutdownAndWait(restarted.baseUrl, restarted.exitedRef);
    appProcess = null;
  } finally {
    if (appProcess !== null) {
      spawn('taskkill', ['/pid', String(appProcess.pid), '/T', '/F'], { stdio: 'ignore' });
    }
    rmSync(dirname(installDir), { recursive: true, force: true });
    rmSync(dataDir, { recursive: true, force: true });
  }
}

async function main() {
  console.log('Stage 20A/20E installer smoke verification');

  step('Locate the built installer');
  const installerFile = existsSync(OUTPUT_DIR)
    ? readdirSync(OUTPUT_DIR).find((f) => f.endsWith('.exe'))
    : undefined;
  expect(installerFile !== undefined, 'installer found in build/release/output', 'Run: powershell -File scripts/build-release.ps1 -Version "0.1.0-dev+test"');
  const installerPath = join(OUTPUT_DIR, installerFile);

  step('Verify the SHA-256 digest file matches the installer');
  const hashFile = `${installerPath}.sha256`;
  expect(existsSync(hashFile), 'digest file exists', hashFile);
  const recomputedHash = await sha256File(installerPath);
  const recordedHash = readFileSync(hashFile, 'utf8').split(/\s+/)[0];
  expect(recomputedHash === recordedHash, 'recomputed SHA-256 matches the recorded digest', {
    recomputed: recomputedHash,
    recorded: recordedHash,
  });

  await testOrdinaryUninstallAndReinstallScenario(installerPath);
  await testExplicitPurgeScenario(installerPath);
  await testManualUpgradeWhileRunningScenario(installerPath);

  console.log(`\n${stepCount} steps passed. PASS`);
}

main().catch((error) => {
  console.error('\nverify-installer.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
