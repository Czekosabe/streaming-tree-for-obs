#!/usr/bin/env node
/**
 * Windows installer smoke verification (Stage 20A).
 *
 * A separate helper, not integration script #23 (that is
 * scripts/verify-packaged-app.mjs) - see docs/windows-packaging.md §23.
 * Drives the REAL Inno Setup-produced installer through its own documented
 * silent-install flags into a throwaway, non-default location - never the
 * operator's real per-user install path - then a real silent uninstall.
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

async function main() {
  console.log('Stage 20A installer smoke verification');

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
    appProcess = spawn(exePath, [], {
      env: {
        STREAMING_TREE_DATA_DIR: dataDir,
        STREAMING_TREE_PORT: String(PORT),
        STREAMING_TREE_HOST: '127.0.0.1',
        STREAMING_TREE_TEST_NO_UI: '1',
        SystemRoot: process.env.SystemRoot ?? 'C:\\Windows',
      },
      stdio: ['ignore', 'ignore', 'ignore'],
    });
    let exited = false;
    appProcess.on('exit', () => {
      exited = true;
    });

    const deadline = Date.now() + 30_000;
    let healthy = false;
    while (Date.now() < deadline && !exited) {
      try {
        const health = await fetch(`${BASE_URL}/api/health`);
        if (health.ok) {
          healthy = true;
          break;
        }
      } catch {
        // Not ready yet.
      }
      await new Promise((r) => setTimeout(r, 300));
    }
    expect(healthy, 'installed executable becomes healthy');

    const about = await fetch(`${BASE_URL}/api/about`).then((r) => r.json());
    expect(about.creatorName === 'Czekosabe', 'installed executable serves the real About API', about);

    const shutdownResp = await fetch(`${BASE_URL}/api/system/shutdown`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm: true }),
    });
    expect(shutdownResp.ok, 'graceful shutdown accepted');

    const stopDeadline = Date.now() + 15_000;
    while (Date.now() < stopDeadline && !exited) {
      await new Promise((r) => setTimeout(r, 200));
    }
    expect(exited, 'installed executable exits after shutdown');
    appProcess = null;

    step('Silent uninstall');
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

    console.log(`\n${stepCount} steps passed. PASS`);
  } finally {
    if (appProcess !== null) {
      spawn('taskkill', ['/pid', String(appProcess.pid), '/T', '/F'], { stdio: 'ignore' });
    }
    rmSync(dirname(installDir), { recursive: true, force: true });
    rmSync(dataDir, { recursive: true, force: true });
  }
}

main().catch((error) => {
  console.error('\nverify-installer.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
