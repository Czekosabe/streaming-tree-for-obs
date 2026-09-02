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
import { createReadStream, existsSync, mkdtempSync, readdirSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release', 'output');
const STAGING_DIR = join(REPO_ROOT, 'build', 'release', 'staging');
const INNO_SCRIPT = join(REPO_ROOT, 'scripts', 'installer', 'streaming-tree.iss');
// Mirrors the exact AppId GUID in scripts/installer/streaming-tree.iss's
// own [Setup] section - never a second, independently-typed copy of it.
const UNINSTALL_REG_SUBKEY = 'Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\{C067013C-D143-49F8-9510-D078482D6DA4}_is1';
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

/** Locates ISCC.exe using the same candidate paths as
 * scripts/build-release.ps1's own Find-RequiredCommand search, so this
 * script can compile its own throwaway test installers at fixed
 * versions without needing a second PowerShell build pass. */
function findIscc() {
  const candidates = [
    join(process.env.LOCALAPPDATA ?? '', 'Programs', 'Inno Setup 6', 'ISCC.exe'),
    'C:\\Program Files (x86)\\Inno Setup 6\\ISCC.exe',
    'C:\\Program Files\\Inno Setup 6\\ISCC.exe',
  ];
  return candidates.find((p) => existsSync(p));
}

/** Compiles scripts/installer/streaming-tree.iss at a fixed test
 * `version`, reusing the real staged executable/legal documents
 * scripts/build-release.ps1 already produced at STAGING_DIR (built once
 * per CI run, before this script) - never a second `go build`/`npm run
 * build` pass. Returns the path to the compiled installer .exe. */
async function compileTestInstaller(isccPath, version, outputDir) {
  const result = await run(isccPath, [
    `/DMyAppVersion=${version}`,
    `/DStagingDir=${STAGING_DIR}`,
    `/DOutputDir=${outputDir}`,
    INNO_SCRIPT,
  ]);
  expect(result.code === 0, `ISCC compiles a test installer for version ${version}`, result);
  const exe = readdirSync(outputDir).find((f) => f.endsWith('.exe'));
  expect(exe !== undefined, `ISCC produced an installer for version ${version}`, readdirSync(outputDir));
  return join(outputDir, exe);
}

/** Reads the real Inno-registered DisplayVersion under HKEY_CURRENT_USER
 * for this AppId, or null if not present there. This is the ONE
 * canonical root a correctly-behaving per-user build of this installer
 * ever writes to (docs/windows-packaging.md §28) - PrivilegesRequired=
 * lowest with no PrivilegesRequiredOverridesAllowed override, confirmed
 * by a real Inno /LOG capture ("Administrative install mode: No /
 * Install mode root key: HKEY_CURRENT_USER"). */
async function queryHkcuDisplayVersion() {
  const hkcu = await run('reg', ['query', `HKCU\\${UNINSTALL_REG_SUBKEY}`, '/v', 'DisplayVersion']);
  return parseRegDisplayVersion(hkcu.out);
}

/** Reads the real Inno-registered DisplayVersion under HKEY_LOCAL_MACHINE
 * (via the explicit 32-bit/WOW6432Node view, since Setup.exe/Uninstall.exe
 * are 32-bit) for this AppId, or null if not present there. A correctly-
 * behaving per-user build of this installer must NEVER write here - see
 * queryHkcuDisplayVersion's own doc comment. Used only to assert absence,
 * i.e. that the installer under test has not regressed into
 * administrative/all-users install mode. */
async function queryHklmDisplayVersion() {
  const hklm = await run('reg', ['query', `HKLM\\${UNINSTALL_REG_SUBKEY}`, '/reg:32', '/v', 'DisplayVersion']);
  return parseRegDisplayVersion(hklm.out);
}

function parseRegDisplayVersion(regQueryOutput) {
  const match = regQueryOutput.match(/DisplayVersion\s+REG_SZ\s+(\S+)/);
  return match ? match[1] : null;
}

/** Reads the real Inno-registered "Inno Setup: Language" value under
 * HKEY_CURRENT_USER for this AppId, or null if not present - the exact
 * [Languages] Name: value (e.g. "english"/"polish"), never a display
 * name. Confirmed via a real local install+registry read during
 * development (docs/windows-packaging.md §29) - this is the one place
 * Inno records which language an install actually used, read back here
 * by both a fresh /LANG=-driven install and a language-less update to
 * prove UsePreviousLanguage's native preservation for real. */
async function queryHkcuLanguage() {
  const hkcu = await run('reg', ['query', `HKCU\\${UNINSTALL_REG_SUBKEY}`, '/v', 'Inno Setup: Language']);
  const match = hkcu.out.match(/Inno Setup: Language\s+REG_SZ\s+(\S+)/);
  return match ? match[1] : null;
}

/** Reads a .lnk shortcut's real target path via the standard WScript.Shell
 * COM object (the normal Windows mechanism for this - Node has no native
 * .lnk parser), or null if the file does not exist / cannot be read. */
async function resolveShortcutTarget(lnkPath) {
  if (!existsSync(lnkPath)) return null;
  const escaped = lnkPath.replace(/'/g, "''");
  const result = await run('powershell', [
    '-NoProfile', '-NonInteractive', '-Command',
    `(New-Object -ComObject WScript.Shell).CreateShortcut('${escaped}').TargetPath`,
  ]);
  const target = result.out.trim();
  return target.length > 0 ? target : null;
}

/** Compares two filesystem paths for real identity, not string equality:
 * a real CI finding (docs/windows-packaging.md §28) showed a shortcut's
 * WScript.Shell-resolved TargetPath can come back in a different string
 * form than the path this script itself built (the runner's own %TEMP%
 * resolves to a short 8.3 form, e.g. "RUNNER~1", visible in this same
 * script's own "Hermetic install directory" log line) for what is the
 * same real file. Returns full diagnostics, not just a boolean, so a
 * mismatch is fully explained in the CI log the first time rather than
 * requiring another push-and-wait round-trip to see what each side
 * actually resolved to. */
function comparePaths(a, b) {
  let resolvedA = a;
  let resolvedB = b;
  let resolveError = null;
  try {
    resolvedA = realpathSync.native(a);
    resolvedB = realpathSync.native(b);
  } catch (err) {
    resolveError = err.message;
  }
  const same = resolvedA.toLowerCase() === resolvedB.toLowerCase();
  return { same, raw: { a, b }, resolved: { a: resolvedA, b: resolvedB }, resolveError };
}

const DESKTOP_LNK_NAME = 'Streaming Tree for OBS.lnk';
const START_MENU_GROUP = 'Streaming Tree for OBS';

function desktopShortcutPath() {
  return join(process.env.USERPROFILE ?? '', 'Desktop', DESKTOP_LNK_NAME);
}
function startMenuAppShortcutPath() {
  return join(process.env.APPDATA ?? '', 'Microsoft', 'Windows', 'Start Menu', 'Programs', START_MENU_GROUP, DESKTOP_LNK_NAME);
}
function startMenuUninstallShortcutPath() {
  return join(process.env.APPDATA ?? '', 'Microsoft', 'Windows', 'Start Menu', 'Programs', START_MENU_GROUP, `Uninstall ${START_MENU_GROUP}.lnk`);
}

/** Scenario E (docs/windows-packaging.md §28): real Start Menu/desktop
 * shortcut task behavior, on the GitHub-hosted runner's own disposable
 * per-user Desktop/Start Menu - never the operator's real machine (this
 * script never runs there for this scenario; see its own doc comment
 * and docs/manual-verification.md for why this was previously left to
 * manual verification only). Only ever touches paths built from the
 * exact literal app/group name above - never a wildcard or prefix
 * match - and every created shortcut is removed again before this
 * function returns, on every path including failure. */
async function testShortcutTasksScenario(installerPath) {
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-shortcuts-verify-')), 'app');
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Desktop shortcut path under test: ${desktopShortcutPath()}`);
  console.log(`Start Menu group under test: ${START_MENU_GROUP}`);

  try {
    step('Fresh install with default task selection (no /MERGETASKS)');
    const fresh = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', `/DIR=${installDir}`]);
    expect(fresh.code === 0, 'default-task fresh install exits 0', fresh);
    expect(existsSync(startMenuAppShortcutPath()), 'Start Menu app shortcut exists (startmenuicon is checked by default)');
    expect(existsSync(startMenuUninstallShortcutPath()), 'Start Menu uninstall shortcut exists');
    expect(!existsSync(desktopShortcutPath()), 'desktop shortcut does NOT exist (desktopicon is unchecked by default)');
    const startMenuTarget = await resolveShortcutTarget(startMenuAppShortcutPath());
    const startMenuComparison = startMenuTarget !== null
      ? comparePaths(startMenuTarget, join(installDir, 'streaming-tree-server.exe'))
      : { same: false };
    expect(startMenuTarget !== null && startMenuComparison.same,
      'the Start Menu shortcut target resolves to the actual installed executable', startMenuComparison);

    step('Update over the same install with no /MERGETASKS - previous (default) choices must remain stable');
    const update = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', `/DIR=${installDir}`]);
    expect(update.code === 0, 'update exits 0', update);
    expect(existsSync(startMenuAppShortcutPath()), 'Start Menu shortcut still exists after update (native UsePreviousTasks kept it selected)');
    expect(!existsSync(desktopShortcutPath()), 'desktop shortcut still does NOT exist after update - the previous "off" choice was not silently turned on');

    step('Uninstall - both installer-owned Start Menu shortcuts must be removed');
    let uninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(uninstallerFile !== undefined, 'uninstaller exists', readdirSync(installDir));
    let uninstall = await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    expect(uninstall.code === 0, 'uninstall exits 0', uninstall);
    expect(!existsSync(startMenuAppShortcutPath()), 'Start Menu app shortcut was removed by uninstall');
    expect(!existsSync(startMenuUninstallShortcutPath()), 'Start Menu uninstall shortcut was removed by uninstall');

    step('Fresh install with the desktop task explicitly selected (/MERGETASKS="desktopicon")');
    const withDesktop = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', `/DIR=${installDir}`, '/MERGETASKS=desktopicon']);
    expect(withDesktop.code === 0, 'install with desktopicon merged exits 0', withDesktop);
    expect(existsSync(desktopShortcutPath()), 'desktop shortcut exists when the task is explicitly selected');
    const desktopTarget = await resolveShortcutTarget(desktopShortcutPath());
    const desktopComparison = desktopTarget !== null
      ? comparePaths(desktopTarget, join(installDir, 'streaming-tree-server.exe'))
      : { same: false };
    expect(desktopTarget !== null && desktopComparison.same,
      'the desktop shortcut target resolves to the actual installed executable', desktopComparison);
    expect(existsSync(startMenuAppShortcutPath()), 'Start Menu shortcut still exists too (startmenuicon default was merged, not replaced)');

    step('Uninstall - the desktop shortcut must be removed too');
    uninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(uninstallerFile !== undefined, 'uninstaller exists', readdirSync(installDir));
    uninstall = await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    expect(uninstall.code === 0, 'uninstall exits 0', uninstall);
    expect(!existsSync(desktopShortcutPath()), 'desktop shortcut was removed by uninstall');

    step('Fresh install with Start Menu explicitly deselected (/MERGETASKS="!startmenuicon")');
    const noStartMenu = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', `/DIR=${installDir}`, '/MERGETASKS=!startmenuicon']);
    expect(noStartMenu.code === 0, 'install with startmenuicon deselected exits 0', noStartMenu);
    expect(!existsSync(startMenuAppShortcutPath()), 'no Start Menu shortcut was created when the task is deselected');
    expect(existsSync(join(installDir, 'streaming-tree-server.exe')), 'the application itself still installed correctly with no Start Menu shortcut');
  } finally {
    if (existsSync(desktopShortcutPath())) rmSync(desktopShortcutPath(), { force: true });
    const groupDir = join(process.env.APPDATA ?? '', 'Microsoft', 'Windows', 'Start Menu', 'Programs', START_MENU_GROUP);
    if (existsSync(groupDir)) rmSync(groupDir, { recursive: true, force: true });
    const uninstallerFile = existsSync(installDir)
      ? readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f))
      : undefined;
    if (uninstallerFile !== undefined) {
      await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    }
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
  }
}

/** Scenario D (docs/windows-packaging.md §26/§2/§3/§13): the real
 * fresh/update/downgrade-block/repair version-detection logic added to
 * InitializeSetup/UninstallRegRoot in scripts/installer/streaming-
 * tree.iss - compiles three throwaway test installers (0.1.0, 0.2.0, and
 * 0.1.0 again) from the SAME already-built staged executable and drives
 * them through a real fresh install, a real update, a real BLOCKED
 * silent downgrade attempt, and a real same-version repair/reinstall -
 * all against one hermetic install directory, verified against the real
 * Inno-registered DisplayVersion each step, never assumed. */
async function testVersionDetectionScenario(isccPath) {
  const compileDir = mkdtempSync(join(tmpdir(), 'streaming-tree-version-compile-'));
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-version-verify-')), 'app');
  console.log(`Hermetic compile directory: ${compileDir}`);
  console.log(`Hermetic install directory: ${installDir}`);

  try {
    step('Compile throwaway test installers at 0.1.0 and 0.2.0');
    const v1exe = await compileTestInstaller(isccPath, '0.1.0', join(compileDir, 'v1'));
    const v2exe = await compileTestInstaller(isccPath, '0.2.0', join(compileDir, 'v2'));

    step('Fresh install of 0.1.0');
    const fresh = await run(v1exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(fresh.code === 0, 'fresh install of 0.1.0 exits 0', fresh);
    expect(await queryHkcuDisplayVersion() === '0.1.0', 'the registered version is 0.1.0 under HKEY_CURRENT_USER after fresh install');
    expect(await queryHklmDisplayVersion() === null, 'HKEY_LOCAL_MACHINE gained NO registration - the install stayed per-user, not administrative');

    step('Update to 0.2.0 over the same install directory');
    const update = await run(v2exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(update.code === 0, 'update to 0.2.0 exits 0', update);
    expect(await queryHkcuDisplayVersion() === '0.2.0', 'the registered version is 0.2.0 under HKEY_CURRENT_USER after update');
    expect(await queryHklmDisplayVersion() === null, 'HKEY_LOCAL_MACHINE still has no registration after update');

    step('Attempt a silent downgrade back to 0.1.0 - must be refused, not silently applied');
    const downgrade = await run(v1exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(downgrade.code !== 0, 'the silent downgrade attempt does NOT exit 0', downgrade);
    expect(await queryHkcuDisplayVersion() === '0.2.0', 'the registered version is still 0.2.0 - the downgrade did not apply');

    step('Same-version reinstall of 0.2.0 (repair) - must succeed, not be treated as a downgrade');
    const repair = await run(v2exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(repair.code === 0, 'same-version reinstall exits 0', repair);
    expect(await queryHkcuDisplayVersion() === '0.2.0', 'the registered version is still 0.2.0 after repair');
    expect(await queryHklmDisplayVersion() === null, 'HKEY_LOCAL_MACHINE still has no registration after repair');
  } finally {
    const uninstallerFile = existsSync(installDir)
      ? readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f))
      : undefined;
    if (uninstallerFile !== undefined) {
      await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    }
    rmSync(compileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
  }
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
    const install = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
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
    const reinstall = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
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
      await new Promise((r) => setTimeout(r, 500));
    }
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dataDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
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
    const install = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
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
      await new Promise((r) => setTimeout(r, 500));
    }
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dataDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(outsideDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
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
    const install = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(install.code === 0, 'silent install exits 0', install);

    const exePath = join(installDir, 'streaming-tree-server.exe');

    step('Start the application and leave it running - deliberately not shut down before the upgrade');
    const started = await startAppAndWaitHealthy(exePath, dataDir, upgradePort, { STREAMING_TREE_TEST_KEEP_TRAY: '1' });
    appProcess = started.appProcess;

    step('Launch a newer installer over the same location WHILE the application is still running');
    // /LOG=: this scenario has never yet had a clean run - captured and
    // printed on failure below so a real cause is diagnosed from real
    // evidence rather than guessed, the same discipline the purge
    // scenario's own diagnostic already used successfully.
    const upgradeLogPath = join(tmpdir(), `streaming-tree-upgrade-install-${Date.now()}.log`);
    const upgrade = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`, `/LOG=${upgradeLogPath}`]);
    if (upgrade.code !== 0 && existsSync(upgradeLogPath)) {
      console.log('--- Inno install log (diagnostic, upgrade-while-running did not complete) ---');
      console.log(readFileSync(upgradeLogPath, 'utf8'));
      console.log('--- end Inno install log ---');
    }
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
      // A still-running process here means the scenario itself already
      // failed (a healthy cooperative shutdown clears appProcess above
      // before reaching here) - force it down first so cleanup below
      // does not also fail with EPERM on its still-open .exe, masking
      // the real failure that was already recorded by expect() above.
      spawn('taskkill', ['/pid', String(appProcess.pid), '/T', '/F'], { stdio: 'ignore' });
      await new Promise((r) => setTimeout(r, 500));
    }
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dataDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
  }
}

/** Scenario G (docs/windows-packaging.md §29): the installer's English/
 * Polish localization contract. Two parts:
 *   1. Structural - parses the real .iss SOURCE TEXT (not a compiled
 *      artifact) to prove [Languages] offers exactly English (Inno's
 *      own built-in compiler:Default.isl) + Polish (Inno's own shipped
 *      compiler:Languages\Polish.isl), and that every english.<key> has
 *      a matching polish.<key> [CustomMessages] pair in both languages
 *      with no orphan on either side.
 *   2. Real language selection and persistence, driven the same way a
 *      real user or the real built-in updater would: a real
 *      /LANG=english and a real /LANG=polish silent install each
 *      register the matching "Inno Setup: Language" value (confirmed
 *      empirically during development - see queryHkcuLanguage's own
 *      doc comment), a real update with NO /LANG flag at all preserves
 *      whichever language the existing install already had (native
 *      UsePreviousLanguage) in both directions, and the exact real
 *      updater-compatible silent flags from
 *      apps/server/internal/updater/helper_windows.go
 *      (/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG=...) complete
 *      quickly over an existing Polish-language install with no /LANG
 *      override - i.e. the update never sits waiting on a language
 *      dialog - measured directly, not assumed from the absence of a
 *      hang. */
async function testLocalizationScenario(installerPath) {
  step('Structural: [Languages] offers exactly English + Polish, via Inno\'s own shipped .isl files');
  const issSource = readFileSync(INNO_SCRIPT, 'utf8');
  const languagesSection = issSource.match(/\[Languages\]\r?\n([\s\S]*?)\r?\n\[/);
  expect(languagesSection !== null, '[Languages] section found in the .iss source');
  const languagesBody = languagesSection[1];
  expect(/Name:\s*"english";\s*MessagesFile:\s*"compiler:Default\.isl"/.test(languagesBody),
    "english maps to Inno's own built-in compiler:Default.isl");
  expect(/Name:\s*"polish";\s*MessagesFile:\s*"compiler:Languages\\Polish\.isl"/.test(languagesBody),
    "polish maps to Inno's own shipped compiler:Languages\\Polish.isl");
  const languageEntries = [...languagesBody.matchAll(/^Name:\s*"(\w+)"/gm)].map((m) => m[1]);
  expect(languageEntries.length === 2 && languageEntries.includes('english') && languageEntries.includes('polish'),
    'exactly two languages are offered - no accidental third language', languageEntries);

  step('Structural: every english.* [CustomMessages] key has a polish.* counterpart and vice versa - no orphans');
  const customMessageMatches = [...issSource.matchAll(/^(english|polish)\.([A-Za-z0-9_]+)\s*=/gm)];
  const englishKeys = new Set(customMessageMatches.filter((m) => m[1] === 'english').map((m) => m[2]));
  const polishKeys = new Set(customMessageMatches.filter((m) => m[1] === 'polish').map((m) => m[2]));
  const englishOnly = [...englishKeys].filter((k) => !polishKeys.has(k));
  const polishOnly = [...polishKeys].filter((k) => !englishKeys.has(k));
  expect(englishOnly.length === 0 && polishOnly.length === 0,
    'no orphan [CustomMessages] key exists in only one language', { englishOnly, polishOnly });
  expect(englishKeys.size >= 20, `a realistic number of custom messages were localized (${englishKeys.size} keys)`, [...englishKeys].sort());

  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-locale-verify-')), 'app');
  console.log(`Hermetic install directory: ${installDir}`);

  try {
    step('Real fresh install with /LANG=polish registers Polish');
    const polishInstall = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=polish', '/NOICONS', `/DIR=${installDir}`]);
    expect(polishInstall.code === 0, 'the /LANG=polish install exits 0', polishInstall);
    expect(await queryHkcuLanguage() === 'polish', 'the installed language is registered as "polish"');

    step('Real update with NO /LANG flag preserves Polish (native UsePreviousLanguage)');
    const preservePolish = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(preservePolish.code === 0, 'the language-less update over a Polish install exits 0', preservePolish);
    expect(await queryHkcuLanguage() === 'polish', 'the language remains "polish" after an update with no /LANG override');

    step('Real proof the exact updater-compatible silent flags never block on language selection, over an existing Polish install');
    const updaterLogPath = join(tmpdir(), `streaming-tree-locale-updater-${Date.now()}.log`);
    const startedAt = Date.now();
    const updaterFlagsRun = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', `/LOG=${updaterLogPath}`, '/NOICONS', `/DIR=${installDir}`]);
    const elapsedMs = Date.now() - startedAt;
    expect(updaterFlagsRun.code === 0, 'the exact real updater command line exits 0 over a Polish install with no /LANG', updaterFlagsRun);
    expect(elapsedMs < 60_000, `the updater-compatible install completed in ${elapsedMs}ms - it did not sit waiting on a language dialog`, elapsedMs);
    expect(await queryHkcuLanguage() === 'polish', 'the language is still "polish" after the updater-flags run');

    step('Uninstall the Polish install before proving the English direction');
    let uninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(uninstallerFile !== undefined, 'uninstaller exists', readdirSync(installDir));
    let uninstall = await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    expect(uninstall.code === 0, 'uninstall exits 0', uninstall);

    step('Real fresh install with /LANG=english registers English');
    const englishInstall = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(englishInstall.code === 0, 'the /LANG=english install exits 0', englishInstall);
    expect(await queryHkcuLanguage() === 'english', 'the installed language is registered as "english"');

    step('Real update with NO /LANG flag preserves English (native UsePreviousLanguage)');
    const preserveEnglish = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(preserveEnglish.code === 0, 'the language-less update over an English install exits 0', preserveEnglish);
    expect(await queryHkcuLanguage() === 'english', 'the language remains "english" after an update with no /LANG override');
  } finally {
    const uninstallerFile = existsSync(installDir)
      ? readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f))
      : undefined;
    if (uninstallerFile !== undefined) {
      await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    }
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
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

  step('Locate ISCC.exe for the version-detection scenario');
  const isccPath = findIscc();
  expect(isccPath !== undefined, 'ISCC.exe found', 'Install it: winget install --id JRSoftware.InnoSetup --scope user');
  await testVersionDetectionScenario(isccPath);
  await testShortcutTasksScenario(installerPath);
  await testLocalizationScenario(installerPath);

  console.log(`\n${stepCount} steps passed. PASS`);
}

main().catch((error) => {
  console.error('\nverify-installer.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
