#!/usr/bin/env node
/**
 * Windows installer smoke verification (Stage 20A/20E).
 *
 * A separate helper, not integration script #23 (that is
 * scripts/verify-packaged-app.mjs) - see docs/windows-packaging.md §23.
 *
 * Every scenario below that installs/updates/uninstalls anything compiles
 * its OWN throwaway Inno Setup installer, under its OWN dedicated,
 * obviously-fake, stable AppId (never the real product's AppId, never
 * shared between scenarios) - see the per-scenario AppId constants near
 * the top of this file. This is the installer-test-hygiene corrective
 * fix (docs/progress.md) for a real physical incident: an earlier
 * version of this suite installed the real production installer
 * (compiled with no AppId override) for these same scenarios, which
 * worked, but meant every run shared the operator's own real per-user
 * registry identity with a disposable test - exactly the class of risk
 * that, combined with a since-fixed admin-mode installer bug, once left
 * real orphaned residue on the operator's actual machine. The one place
 * the real production AppId is still referenced at all is
 * testProductionIdentityStructuralScenario, which proves its stable
 * value, install scope, and absence of the historical admin-mode
 * directive directly from the .iss SOURCE TEXT - never by compiling or
 * installing under it.
 *
 * Two uninstall behaviors are proven, each with its own dedicated
 * throwaway installer (docs/windows-packaging.md §26):
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
 * Every scenario uses only hermetic, throwaway install/data directories
 * and its own dedicated throwaway AppId - the real per-user install
 * location, the real per-user registry identity, and real AppData are
 * never touched. Neither uninstall scenario stores a real OS-credential-
 * store secret (the purge helper's own credential-deletion correctness
 * is unit-tested hermetically against a fake store in
 * apps/server/internal/userdatapurge - see that package's own tests -
 * so this script never has to touch the real Windows Credential
 * Manager to prove it).
 *
 * Requires an installer to already exist at build/release/output/*.exe -
 * run scripts/build-release.ps1 first (without -SkipInstaller) - purely
 * to prove that real release artifact's own existence/hash are intact;
 * it is never itself installed by this script. Also requires ISCC.exe
 * (findIscc()) - every scenario now compiles its own test installer.
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
// A dedicated, obviously-fake, stable throwaway AppId used ONLY by
// testVersionDetectionScenario's own compiled 0.1.0/0.2.0 test
// installers below - never the real product's AppId above. Fixed
// rather than randomly generated per run because that scenario's own
// fresh/update/downgrade/repair sequence genuinely needs the *same*
// AppId across all three of its own compiled installers to exercise
// Inno's real same-AppId update semantics; "stable within this one
// isolated scenario" is what that requires, not "stable across the
// whole test suite" and never "the operator's own real installed
// identity." A real, physical incident on the operator's own machine
// (docs/progress.md, "fix(installer): give the throwaway
// version-detection test scenario its own dedicated AppId") found
// the production AppId itself had been used for exactly this kind of
// throwaway reproduction in the past, leaving orphaned HKLM/HKCU
// residue that later blocked a real installer test - this constant,
// and streaming-tree.iss's own TestAppId override it drives, is the
// structural fix: this scenario can never again touch the real AppId's
// own registry key, not even transiently while a test is running.
// Every scenario below that installs/updates/uninstalls a real compiled
// installer gets its OWN dedicated, obviously-fake, stable throwaway
// AppId - never the real product's AppId above, and never shared
// between scenarios (so two scenarios can never collide with each
// other, and neither can ever collide with the operator's own real
// installed copy or with historical test residue under the real
// AppId - the exact class of incident this whole mechanism exists to
// prevent, see docs/progress.md's "give the throwaway version-
// detection test scenario its own dedicated AppId" and its own
// corrective-pass follow-up entry). Fixed rather than randomly
// generated per run, because a scenario that installs/updates/repairs
// more than once genuinely needs the *same* AppId across its own
// steps to exercise Inno's real same-AppId semantics - "stable within
// one isolated scenario," never "the operator's own real identity."
function subkeyFor(appId) {
  return `Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${appId}_is1`;
}
const SCENARIO_TEST_APP_ID = '{DEADBEEF-DEAD-BEEF-DEAD-BEEFDEADBEEF}';
const SCENARIO_TEST_UNINSTALL_REG_SUBKEY = subkeyFor(SCENARIO_TEST_APP_ID);
const ORDINARY_UNINSTALL_TEST_APP_ID = '{BAADF00D-BAAD-F00D-BAAD-F00DBAADF00D}';
const PURGE_TEST_APP_ID = '{FEEDFACE-FEED-FACE-FEED-FACEFEEDFACE}';
const UPGRADE_WHILE_RUNNING_TEST_APP_ID = '{C0FFEE00-C0FF-EE00-C0FF-EE00C0FFEE00}';
const SHORTCUT_TASKS_TEST_APP_ID = '{ABADCAFE-ABAD-CAFE-ABAD-CAFEABADCAFE}';
const LOCALIZATION_TEST_APP_ID = '{B105F00D-B105-F00D-B105-F00DB105F00D}';
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
 * `version` and a given throwaway `appId`, reusing the real staged
 * executable/legal documents scripts/build-release.ps1 already
 * produced at STAGING_DIR (built once per CI run, before this script)
 * - never a second `go build`/`npm run build` pass. Every caller in
 * this file passes one of the dedicated per-scenario throwaway AppId
 * constants above - never the real product AppId - so every scenario
 * that installs/updates/uninstalls a real compiled installer is fully
 * isolated from the real product's own registry identity, and from
 * every other scenario. Returns the path to the compiled installer
 * .exe. */
async function compileTestInstaller(isccPath, version, outputDir, appId) {
  const result = await run(isccPath, [
    `/DMyAppVersion=${version}`,
    `/DStagingDir=${STAGING_DIR}`,
    `/DOutputDir=${outputDir}`,
    `/DTestAppId=${appId.replace(/^\{/, '{{')}`,
    INNO_SCRIPT,
  ]);
  expect(result.code === 0, `ISCC compiles a test installer for version ${version} (AppId ${appId})`, result);
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
 * Install mode root key: HKEY_CURRENT_USER"). `subkey` is required,
 * deliberately with no default - every caller must explicitly pass
 * the one dedicated per-scenario throwaway subkey it is checking, so
 * this function can never silently fall back to the real product's
 * own registry subkey (UNINSTALL_REG_SUBKEY exists only for the
 * structural scenario's own comparison, never as a default here). */
async function queryHkcuDisplayVersion(subkey) {
  const hkcu = await run('reg', ['query', `HKCU\\${subkey}`, '/v', 'DisplayVersion']);
  return parseRegDisplayVersion(hkcu.out);
}

/** Reads the real Inno-registered DisplayVersion under HKEY_LOCAL_MACHINE
 * (via the explicit 32-bit/WOW6432Node view, since Setup.exe/Uninstall.exe
 * are 32-bit) for this AppId, or null if not present there. A correctly-
 * behaving per-user build of this installer must NEVER write here - see
 * queryHkcuDisplayVersion's own doc comment. Used only to assert absence,
 * i.e. that the installer under test has not regressed into
 * administrative/all-users install mode. `subkey` is required, with no
 * default - same reasoning as queryHkcuDisplayVersion's own. */
async function queryHklmDisplayVersion(subkey) {
  const hklm = await run('reg', ['query', `HKLM\\${subkey}`, '/reg:32', '/v', 'DisplayVersion']);
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
 * prove UsePreviousLanguage's native preservation for real. `subkey`
 * is required, with no default - testLocalizationScenario always
 * passes its own dedicated LOCALIZATION_TEST_APP_ID subkey. */
async function queryHkcuLanguage(subkey) {
  const hkcu = await run('reg', ['query', `HKCU\\${subkey}`, '/v', 'Inno Setup: Language']);
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

/** Resolves the REAL current desktop directory via .NET's own
 * Environment.GetFolderPath (the same Known Folder API Inno Setup's own
 * {userdesktop} constant resolves through internally) - never a
 * hardcoded `%USERPROFILE%\Desktop`. A real local run on this project's
 * own development machine found that guess wrong: this machine's
 * desktop is OneDrive-redirected to `%USERPROFILE%\OneDrive\Pulpit`
 * (a real, pre-existing, unrelated machine configuration - Known
 * Folder Move, not something this project or this script created), so
 * a hardcoded path silently checked the wrong location and reported a
 * real shortcut as missing when Inno had, correctly, already created
 * it exactly where Windows itself considers the desktop to be. */
async function resolveRealDesktopDir() {
  const result = await run('powershell', [
    '-NoProfile', '-NonInteractive', '-Command',
    '[Environment]::GetFolderPath("Desktop")',
  ]);
  const dir = result.out.trim();
  expect(dir.length > 0, 'the real current-user desktop directory resolves to a real path', result);
  return dir;
}

function desktopShortcutPath(desktopDir) {
  return join(desktopDir, DESKTOP_LNK_NAME);
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
async function testShortcutTasksScenario(isccPath) {
  const compileDir = mkdtempSync(join(tmpdir(), 'streaming-tree-shortcuts-compile-'));
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-shortcuts-verify-')), 'app');
  const desktopDir = await resolveRealDesktopDir();
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Real desktop directory resolved to: ${desktopDir}`);
  console.log(`Desktop shortcut path under test: ${desktopShortcutPath(desktopDir)}`);
  console.log(`Start Menu group under test: ${START_MENU_GROUP}`);

  try {
    step('Compile a throwaway test installer under a dedicated AppId');
    const installerPath = await compileTestInstaller(isccPath, '0.1.0-shortcut-tasks-test', compileDir, SHORTCUT_TASKS_TEST_APP_ID);

    step('Fresh install with default task selection (no /MERGETASKS)');
    const fresh = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', `/DIR=${installDir}`]);
    expect(fresh.code === 0, 'default-task fresh install exits 0', fresh);
    expect(existsSync(startMenuAppShortcutPath()), 'Start Menu app shortcut exists (startmenuicon is checked by default)');
    expect(existsSync(startMenuUninstallShortcutPath()), 'Start Menu uninstall shortcut exists');
    expect(!existsSync(desktopShortcutPath(desktopDir)), 'desktop shortcut does NOT exist (desktopicon is unchecked by default)');
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
    expect(!existsSync(desktopShortcutPath(desktopDir)), 'desktop shortcut still does NOT exist after update - the previous "off" choice was not silently turned on');

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
    expect(existsSync(desktopShortcutPath(desktopDir)), 'desktop shortcut exists when the task is explicitly selected');
    const desktopTarget = await resolveShortcutTarget(desktopShortcutPath(desktopDir));
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
    expect(!existsSync(desktopShortcutPath(desktopDir)), 'desktop shortcut was removed by uninstall');

    step('Fresh install with Start Menu explicitly deselected (/MERGETASKS="!startmenuicon")');
    const noStartMenu = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', `/DIR=${installDir}`, '/MERGETASKS=!startmenuicon']);
    expect(noStartMenu.code === 0, 'install with startmenuicon deselected exits 0', noStartMenu);
    expect(!existsSync(startMenuAppShortcutPath()), 'no Start Menu shortcut was created when the task is deselected');
    expect(existsSync(join(installDir, 'streaming-tree-server.exe')), 'the application itself still installed correctly with no Start Menu shortcut');
  } finally {
    if (existsSync(desktopShortcutPath(desktopDir))) rmSync(desktopShortcutPath(desktopDir), { force: true });
    const groupDir = join(process.env.APPDATA ?? '', 'Microsoft', 'Windows', 'Start Menu', 'Programs', START_MENU_GROUP);
    if (existsSync(groupDir)) rmSync(groupDir, { recursive: true, force: true });
    const uninstallerFile = existsSync(installDir)
      ? readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f))
      : undefined;
    if (uninstallerFile !== undefined) {
      await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    }
    const shortcutTasksTestSubkey = subkeyFor(SHORTCUT_TASKS_TEST_APP_ID);
    await run('reg', ['delete', `HKCU\\${shortcutTasksTestSubkey}`, '/f']);
    await run('reg', ['delete', `HKLM\\${shortcutTasksTestSubkey}`, '/reg:32', '/f']);
    rmSync(compileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
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
 * Inno-registered DisplayVersion each step, never assumed.
 *
 * A second, real-world-driven part below this proves the fix for a real
 * physical finding: installing a differently-labelled prerelease build
 * sharing the SAME numeric core (e.g. "0.2.0-manualtest-batch1" over an
 * already-installed "0.2.0-manualtest") used to compare EQUAL under the
 * old CompareAppVersions (it only checked whether each side's prerelease
 * tag was empty, never what the tag actually said) - see
 * CompareAppVersions's own doc comment in the .iss. Exercises the real
 * Inno-driven behavioral consequence this project's silent installer path
 * can actually prove hermetically: such a pair must never be refused as
 * a downgrade in EITHER direction (neither can be proven older than the
 * other from the version string alone), while a real release still
 * outranks - and blocks - a prerelease of the identical core, unchanged.
 * (ClassifyVersionOperation's richer "Repair vs different build" wording
 * only reaches the interactive Ready-to-Install page, which Inno itself
 * skips entirely under /VERYSILENT - not something this hermetic,
 * always-silent harness can observe; its own doc comment says so.) */
async function testVersionDetectionScenario(isccPath) {
  const compileDir = mkdtempSync(join(tmpdir(), 'streaming-tree-version-compile-'));
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-version-verify-')), 'app');
  const installDir2 = join(mkdtempSync(join(tmpdir(), 'streaming-tree-version-verify2-')), 'app');
  console.log(`Hermetic compile directory: ${compileDir}`);
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Second hermetic install directory: ${installDir2}`);

  try {
    step('Compile throwaway test installers at 0.1.0 and 0.2.0');
    const v1exe = await compileTestInstaller(isccPath, '0.1.0', join(compileDir, 'v1'), SCENARIO_TEST_APP_ID);
    const v2exe = await compileTestInstaller(isccPath, '0.2.0', join(compileDir, 'v2'), SCENARIO_TEST_APP_ID);

    step('Fresh install of 0.1.0');
    const fresh = await run(v1exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(fresh.code === 0, 'fresh install of 0.1.0 exits 0', fresh);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.1.0', 'the registered version is 0.1.0 under HKEY_CURRENT_USER after fresh install');
    expect(await queryHklmDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === null, 'HKEY_LOCAL_MACHINE gained NO registration - the install stayed per-user, not administrative');

    step('Update to 0.2.0 over the same install directory');
    const update = await run(v2exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(update.code === 0, 'update to 0.2.0 exits 0', update);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.2.0', 'the registered version is 0.2.0 under HKEY_CURRENT_USER after update');
    expect(await queryHklmDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === null, 'HKEY_LOCAL_MACHINE still has no registration after update');

    step('Attempt a silent downgrade back to 0.1.0 - must be refused, not silently applied');
    const downgrade = await run(v1exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(downgrade.code !== 0, 'the silent downgrade attempt does NOT exit 0', downgrade);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.2.0', 'the registered version is still 0.2.0 - the downgrade did not apply');

    step('Same-version reinstall of 0.2.0 (repair) - must succeed, not be treated as a downgrade');
    const repair = await run(v2exe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(repair.code === 0, 'same-version reinstall exits 0', repair);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.2.0', 'the registered version is still 0.2.0 after repair');
    expect(await queryHklmDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === null, 'HKEY_LOCAL_MACHINE still has no registration after repair');

    step('Compile two more throwaway installers sharing the 0.2.0 core with different prerelease labels');
    const v2PreAExe = await compileTestInstaller(isccPath, '0.2.0-manualtest', join(compileDir, 'v2-pre-a'), SCENARIO_TEST_APP_ID);
    const v2PreBExe = await compileTestInstaller(isccPath, '0.2.0-manualtest-batch1', join(compileDir, 'v2-pre-b'), SCENARIO_TEST_APP_ID);

    step('A prerelease of the SAME core as an already-installed real release is still a blocked downgrade');
    const preOverRelease = await run(v2PreAExe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(preOverRelease.code !== 0, 'installing "0.2.0-manualtest" over the installed real "0.2.0" release does NOT exit 0', preOverRelease);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.2.0', 'the registered version is still the real release "0.2.0" - the release-outranks-prerelease rule still holds');

    step('Uninstall from the first hermetic directory - the two remaining steps below share this scenario\'s AppId, so the registry must be genuinely empty before treating the second directory\'s install as fresh');
    const firstUninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(firstUninstallerFile !== undefined, 'uninstaller exists in the first hermetic directory', readdirSync(installDir));
    const firstUninstall = await run(join(installDir, firstUninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    expect(firstUninstall.code === 0, 'uninstall of the first hermetic directory exits 0', firstUninstall);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === null, 'no version is registered for this AppId after uninstalling the first hermetic directory');

    step('Fresh install of "0.2.0-manualtest" into a second hermetic directory');
    const freshPreA = await run(v2PreAExe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir2}`]);
    expect(freshPreA.code === 0, 'fresh install of "0.2.0-manualtest" exits 0', freshPreA);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.2.0-manualtest', 'the registered version is the full string "0.2.0-manualtest"');

    step('The real physical finding, reproduced and proven fixed: a differently-labelled prerelease of the same core is never refused as a downgrade');
    const toBatch1 = await run(v2PreBExe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir2}`]);
    expect(toBatch1.code === 0, 'installing "0.2.0-manualtest-batch1" over "0.2.0-manualtest" exits 0 - not blocked as a downgrade', toBatch1);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.2.0-manualtest-batch1', 'the registered version genuinely became the full, distinct string "0.2.0-manualtest-batch1" - never silently treated as a no-op "same version"');

    step('The reverse direction is equally never refused - neither build can be proven older than the other');
    const backToPreA = await run(v2PreAExe, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir2}`]);
    expect(backToPreA.code === 0, 'installing "0.2.0-manualtest" back over "0.2.0-manualtest-batch1" exits 0 - not blocked as a downgrade', backToPreA);
    expect(await queryHkcuDisplayVersion(SCENARIO_TEST_UNINSTALL_REG_SUBKEY) === '0.2.0-manualtest', 'the registered version reverted to the full, distinct string "0.2.0-manualtest"');
  } finally {
    for (const dir of [installDir, installDir2]) {
      const uninstallerFile = existsSync(dir)
        ? readdirSync(dir).find((f) => /^unins\d+\.exe$/.test(f))
        : undefined;
      if (uninstallerFile !== undefined) {
        await run(join(dir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
      }
    }
    // Failure-path backstop: if a scenario failure happened before the
    // uninstaller above could run (or the uninstaller itself failed),
    // this scenario's own dedicated SCENARIO_TEST_UNINSTALL_REG_SUBKEY
    // must never survive into the next run - explicitly targeted at
    // this one known throwaway AppId, never a pattern-based sweep of
    // the real registry. `reg delete` exiting non-zero here just means
    // the key was already gone (the normal, expected case) - never
    // treated as a scenario failure.
    await run('reg', ['delete', `HKCU\\${SCENARIO_TEST_UNINSTALL_REG_SUBKEY}`, '/f']);
    await run('reg', ['delete', `HKLM\\${SCENARIO_TEST_UNINSTALL_REG_SUBKEY}`, '/reg:32', '/f']);
    rmSync(compileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dirname(installDir2), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
  }
}

/** Scenario A (docs/windows-packaging.md §26): an ordinary uninstall,
 * with no purge flag, preserves the data directory - then a fresh
 * reinstall over the same location starts up healthy against that
 * preserved data, proving recovery is real and functional, not just
 * "a file happened to still exist." Compiles its own throwaway
 * installer under ORDINARY_UNINSTALL_TEST_APP_ID (never the real
 * product AppId) - this scenario's own install/uninstall/reinstall
 * behavior does not depend on which AppId is compiled in, only that
 * the same one is used consistently across its own steps. */
async function testOrdinaryUninstallAndReinstallScenario(isccPath) {
  const compileDir = mkdtempSync(join(tmpdir(), 'streaming-tree-install-compile-'));
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-install-verify-')), 'app');
  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-install-data-'));
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Hermetic data directory: ${dataDir}`);
  console.log('The real per-user install location and real AppData are never touched.');

  let appProcess = null;

  try {
    step('Compile a throwaway test installer under a dedicated AppId');
    const installerPath = await compileTestInstaller(isccPath, '0.1.0-ordinary-uninstall-test', compileDir, ORDINARY_UNINSTALL_TEST_APP_ID);

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
    // Failure-path backstop (same pattern as testVersionDetectionScenario):
    // this scenario's own dedicated ORDINARY_UNINSTALL_TEST_APP_ID subkey
    // must never survive a mid-scenario failure into the next run. A
    // non-zero exit here just means the key is already gone - never
    // treated as a scenario failure.
    const ordinaryUninstallTestSubkey = subkeyFor(ORDINARY_UNINSTALL_TEST_APP_ID);
    await run('reg', ['delete', `HKCU\\${ordinaryUninstallTestSubkey}`, '/f']);
    await run('reg', ['delete', `HKLM\\${ordinaryUninstallTestSubkey}`, '/reg:32', '/f']);
    rmSync(compileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dataDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
  }
}

/** Scenario B (docs/windows-packaging.md §26): an explicit purge
 * uninstall - driven via STREAMING_TREE_TEST_PURGE_USER_DATA=1 on the
 * uninstaller's own environment, InitializeUninstall's own documented
 * automated-test hook for the path a GUI checkbox click would
 * otherwise gate under /VERYSILENT - removes the whole data directory,
 * and never touches anything outside it. Compiles its own throwaway
 * installer under PURGE_TEST_APP_ID (never the real product AppId) -
 * purge behavior is driven entirely by STREAMING_TREE_DATA_DIR and the
 * test-only env-var hook, never by which AppId is compiled in. */
async function testExplicitPurgeScenario(isccPath) {
  const compileDir = mkdtempSync(join(tmpdir(), 'streaming-tree-purge-compile-'));
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
    step('Compile a throwaway test installer under a dedicated AppId');
    const installerPath = await compileTestInstaller(isccPath, '0.1.0-purge-test', compileDir, PURGE_TEST_APP_ID);

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
    // Failure-path backstop, same pattern as every other scenario here.
    const purgeTestSubkey = subkeyFor(PURGE_TEST_APP_ID);
    await run('reg', ['delete', `HKCU\\${purgeTestSubkey}`, '/f']);
    await run('reg', ['delete', `HKLM\\${purgeTestSubkey}`, '/reg:32', '/f']);
    rmSync(compileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
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
async function testManualUpgradeWhileRunningScenario(isccPath) {
  const compileDir = mkdtempSync(join(tmpdir(), 'streaming-tree-upgrade-compile-'));
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-upgrade-verify-')), 'app');
  const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-upgrade-data-'));
  console.log(`Hermetic install directory: ${installDir}`);
  console.log(`Hermetic data directory: ${dataDir}`);

  let appProcess = null;
  const upgradePort = PORT + 2;

  try {
    step('Compile a throwaway test installer under a dedicated AppId');
    const installerPath = await compileTestInstaller(isccPath, '0.1.0-upgrade-while-running-test', compileDir, UPGRADE_WHILE_RUNNING_TEST_APP_ID);

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
    // This scenario's own cleanup previously deleted only the install/
    // data directories, never actually running the compiled test
    // installer's own uninstaller - a real gap, found during the
    // installer-test-hygiene corrective pass (docs/progress.md), that
    // left this scenario's own registry registration behind after
    // every run. Uninstall for real first, then the same registry
    // failure-path backstop every other scenario here uses.
    const uninstallerFile = existsSync(installDir)
      ? readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f))
      : undefined;
    if (uninstallerFile !== undefined) {
      await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    }
    const upgradeWhileRunningTestSubkey = subkeyFor(UPGRADE_WHILE_RUNNING_TEST_APP_ID);
    await run('reg', ['delete', `HKCU\\${upgradeWhileRunningTestSubkey}`, '/f']);
    await run('reg', ['delete', `HKLM\\${upgradeWhileRunningTestSubkey}`, '/reg:32', '/f']);
    rmSync(compileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
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
async function testLocalizationScenario(isccPath) {
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

  const compileDir = mkdtempSync(join(tmpdir(), 'streaming-tree-locale-compile-'));
  const installDir = join(mkdtempSync(join(tmpdir(), 'streaming-tree-locale-verify-')), 'app');
  console.log(`Hermetic install directory: ${installDir}`);
  const localizationTestSubkey = subkeyFor(LOCALIZATION_TEST_APP_ID);

  try {
    step('Compile a throwaway test installer under a dedicated AppId');
    const installerPath = await compileTestInstaller(isccPath, '0.1.0-localization-test', compileDir, LOCALIZATION_TEST_APP_ID);

    step('Real fresh install with /LANG=polish registers Polish');
    const polishInstall = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=polish', '/NOICONS', `/DIR=${installDir}`]);
    expect(polishInstall.code === 0, 'the /LANG=polish install exits 0', polishInstall);
    expect(await queryHkcuLanguage(localizationTestSubkey) === 'polish', 'the installed language is registered as "polish"');

    step('Real update with NO /LANG flag preserves Polish (native UsePreviousLanguage)');
    const preservePolish = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(preservePolish.code === 0, 'the language-less update over a Polish install exits 0', preservePolish);
    expect(await queryHkcuLanguage(localizationTestSubkey) === 'polish', 'the language remains "polish" after an update with no /LANG override');

    step('Real proof the exact updater-compatible silent flags never block on language selection, over an existing Polish install');
    const updaterLogPath = join(tmpdir(), `streaming-tree-locale-updater-${Date.now()}.log`);
    const startedAt = Date.now();
    const updaterFlagsRun = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', `/LOG=${updaterLogPath}`, '/NOICONS', `/DIR=${installDir}`]);
    const elapsedMs = Date.now() - startedAt;
    expect(updaterFlagsRun.code === 0, 'the exact real updater command line exits 0 over a Polish install with no /LANG', updaterFlagsRun);
    expect(elapsedMs < 60_000, `the updater-compatible install completed in ${elapsedMs}ms - it did not sit waiting on a language dialog`, elapsedMs);
    expect(await queryHkcuLanguage(localizationTestSubkey) === 'polish', 'the language is still "polish" after the updater-flags run');

    step('Uninstall the Polish install before proving the English direction');
    let uninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(uninstallerFile !== undefined, 'uninstaller exists', readdirSync(installDir));
    let uninstall = await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    expect(uninstall.code === 0, 'uninstall exits 0', uninstall);

    step('Real fresh install with /LANG=english registers English');
    const englishInstall = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/LANG=english', '/NOICONS', `/DIR=${installDir}`]);
    expect(englishInstall.code === 0, 'the /LANG=english install exits 0', englishInstall);
    expect(await queryHkcuLanguage(localizationTestSubkey) === 'english', 'the installed language is registered as "english"');

    step('Real update with NO /LANG flag preserves English (native UsePreviousLanguage)');
    const preserveEnglish = await run(installerPath, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`]);
    expect(preserveEnglish.code === 0, 'the language-less update over an English install exits 0', preserveEnglish);
    expect(await queryHkcuLanguage(localizationTestSubkey) === 'english', 'the language remains "english" after an update with no /LANG override');
  } finally {
    const uninstallerFile = existsSync(installDir)
      ? readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f))
      : undefined;
    if (uninstallerFile !== undefined) {
      await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    }
    await run('reg', ['delete', `HKCU\\${localizationTestSubkey}`, '/f']);
    await run('reg', ['delete', `HKLM\\${localizationTestSubkey}`, '/reg:32', '/f']);
    rmSync(compileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    rmSync(dirname(installDir), { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
  }
}

/** Scenario H (installer-test-hygiene corrective pass, docs/
 * progress.md): proves the production identity's own stable
 * properties directly from the real .iss SOURCE TEXT - never by
 * compiling or installing anything under the real production AppId,
 * per this project's own explicit preference for structural/build-
 * time assertions over installing the real identity merely to prove
 * a literal. Every other scenario in this file now installs only
 * under its own dedicated throwaway AppId (see the per-scenario
 * constants above); this is the one place the real production AppId
 * value itself is verified, and it never leaves the source file. */
function testProductionIdentityStructuralScenario() {
  step('Structural: the real production AppId literal is the documented, stable value');
  const issSource = readFileSync(INNO_SCRIPT, 'utf8');
  const defaultAppIdMatch = issSource.match(/#ifndef TestAppId\s*\n\s*#define TestAppId "([^"]+)"/);
  expect(defaultAppIdMatch !== null, 'TestAppId has a documented #ifndef default in the .iss source', issSource.slice(0, 200));
  const defaultAppIdLiteral = defaultAppIdMatch?.[1] ?? '';
  expect(defaultAppIdLiteral === UNINSTALL_REG_SUBKEY.match(/\{[0-9A-Fa-f-]+\}/)?.[0].replace(/^\{/, '{{'),
    'TestAppId\'s own #ifndef default resolves to exactly the one real product AppId this file itself references',
    { defaultAppIdLiteral, expected: UNINSTALL_REG_SUBKEY });
  expect(/^AppId=\{#TestAppId\}$/m.test(issSource),
    'the [Setup] section\'s own AppId directive resolves through {#TestAppId} - never a second, independently hardcoded literal');

  step('Structural: build-release.ps1\'s test-only AppId override cannot affect an ordinary invocation');
  // scripts/verify-updater.mjs (docs/windows-packaging.md §33) needs
  // build-release.ps1 itself to be able to compile under a throwaway
  // AppId - a real local-execution hazard fix, since that script's own
  // old/new test builds previously compiled under the real production
  // AppId. Rather than the stronger-but-now-obsolete "the string
  // TestAppId never appears at all", this proves the weaker, correct
  // invariant that change actually requires: the override exists, but
  // is structurally inert unless a caller explicitly passes it.
  const buildScript = readFileSync(join(REPO_ROOT, 'scripts', 'build-release.ps1'), 'utf8');
  expect(/\[string\]\$TestAppId/.test(buildScript),
    'build-release.ps1 declares an optional $TestAppId parameter - no [Parameter(Mandatory...)], so its default is an empty string');
  // Matches up to the blank line immediately before "$IsccArgs +=
  // $InnoScript" (not just the first "}", which would stop at the
  // guard's own nested GUID-validation "if" block instead of its real,
  // outer closing brace). `\r?\n` throughout, never a bare `\n` -
  // scripts/build-release.ps1 is checked out with CRLF line endings on
  // a real Windows CI runner, and a bare `\n` here would never match
  // there even though it matches fine against this repository's own
  // on-disk LF working copy - a real gap a bare local run could not
  // have caught, only found by a real CI failure.
  const testAppIdGuard = buildScript.match(/if \(\$TestAppId\) \{([\s\S]*?)\r?\n\}\r?\n\r?\n\$IsccArgs \+= \$InnoScript/);
  expect(testAppIdGuard !== null,
    'the ISCC /DTestAppId= argument is added only inside an explicit "if ($TestAppId)" guard block');
  const dTestAppIdOccurrences = buildScript.match(/\/DTestAppId=/g) ?? [];
  expect(dTestAppIdOccurrences.length === 1,
    'the literal "/DTestAppId=" appears exactly once in the whole script', dTestAppIdOccurrences.length);
  expect(testAppIdGuard !== null && /\/DTestAppId=/.test(testAppIdGuard[1]),
    'that one "/DTestAppId=" occurrence is the one inside the "if ($TestAppId)" guard, never unconditional');

  step('Structural: the Windows package CI workflow never passes -TestAppId - real production/package builds always get the default (production) identity');
  const windowsWorkflow = readFileSync(join(REPO_ROOT, '.github', 'workflows', 'windows-package.yml'), 'utf8');
  expect(!/-TestAppId/.test(windowsWorkflow),
    '.github/workflows/windows-package.yml never passes -TestAppId to build-release.ps1');

  step('Structural: the production install scope is per-user, with no admin-mode override present');
  expect(/^PrivilegesRequired=lowest$/m.test(issSource),
    'PrivilegesRequired=lowest is present - the real product never requires or requests elevation');
  // Matches only an ACTIVE [Setup] directive assignment at the start of
  // a line - never a mention of the directive's own name inside a `;`
  // comment (this file's own §28 corrective-history comments correctly
  // discuss the historical directive by name; that is not a regression).
  expect(!/^PrivilegesRequiredOverridesAllowed\s*=/m.test(issSource),
    'PrivilegesRequiredOverridesAllowed is absent as an active directive - the exact directive that caused the historical admin-mode HKLM incident (docs/windows-packaging.md §28) has not regressed back into the source (comments discussing that history by name are expected and fine)');

  step('Structural: UninstallRegSubkey derives from the same TestAppId, never a second independent copy of the GUID');
  expect(/UninstallRegSubkey = 'Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\\{#TestAppIdBare\}_is1';/.test(issSource),
    'the [Code] section\'s own UninstallRegSubkey constant is built from {#TestAppIdBare}, not a hardcoded GUID literal of its own');
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
  // installerPath/installerFile are deliberately never passed to any
  // scenario below - every scenario that installs/updates/uninstalls
  // something now compiles its own throwaway-AppId installer instead
  // (see the per-scenario AppId constants above). This step's own
  // existence+hash check still verifies the real release artifact
  // build-release.ps1 produced is intact; it is simply never installed
  // by this script.

  testProductionIdentityStructuralScenario();

  step('Locate ISCC.exe - every remaining scenario compiles its own throwaway-AppId installer');
  const isccPath = findIscc();
  expect(isccPath !== undefined, 'ISCC.exe found', 'Install it: winget install --id JRSoftware.InnoSetup --scope user');

  await testOrdinaryUninstallAndReinstallScenario(isccPath);
  await testExplicitPurgeScenario(isccPath);
  await testManualUpgradeWhileRunningScenario(isccPath);
  await testVersionDetectionScenario(isccPath);
  await testShortcutTasksScenario(isccPath);
  await testLocalizationScenario(isccPath);

  console.log(`\n${stepCount} steps passed. PASS`);
}

main().catch((error) => {
  console.error('\nverify-installer.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
