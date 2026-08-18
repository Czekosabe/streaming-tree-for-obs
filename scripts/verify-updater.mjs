#!/usr/bin/env node
/**
 * Stage 20B application-updater end-to-end verification - integration
 * script #24 (docs/updater.md §41).
 *
 * Exercises the REAL cycle against real, locally-built artifacts: an
 * old version is really installed via the real Inno Setup installer
 * into a hermetic directory, its real updater manager checks a local
 * fake GitHub API server (never the real GitHub API), rejects a
 * mismatched manifest and a tampered installer, then downloads and
 * verifies a real newer installer, installs it through the real
 * external-helper handoff (a real OpenProcess/WaitForSingleObject
 * parent-wait, a real silent Inno Setup upgrade, a real restart), and
 * the restarted process is confirmed to report the new version and
 * surface the one-shot post-update result.
 *
 * The fake-GitHub redirection is possible ONLY because both builds
 * below are compiled with `-tags integration` via
 * `build-release.ps1 -IntegrationTest` - see
 * cmd/server/updater_testhook_integration.go's own doc comment for why
 * this structurally cannot happen in a real release build (the exact,
 * plain `go build ./cmd/server` scripts/build-release.ps1 normally
 * runs, and the only way Inno Setup's own installed content is ever
 * produced).
 *
 * Requires: Go, npm, and Inno Setup's ISCC.exe on PATH (the same
 * requirements scripts/build-release.ps1 itself already documents).
 * Builds two full releases (~1-2 minutes each) - this script takes
 * several minutes to run.
 *
 * Usage:  node scripts/verify-updater.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import http from 'node:http';
import {
  copyFileSync,
  existsSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUTPUT_DIR = join(REPO_ROOT, 'build', 'release', 'output');
const APP_PORT = 18199;
const APP_BASE_URL = `http://127.0.0.1:${APP_PORT}`;

const OLD_VERSION = '0.9.0';
const NEW_VERSION = '0.9.1';

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
  if (detail !== undefined) console.error(`          ${JSON.stringify(detail)}`);
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

async function buildRelease(version) {
  const result = await run('powershell', [
    '-ExecutionPolicy', 'Bypass', '-File', 'scripts/build-release.ps1',
    '-Version', version, '-IntegrationTest',
  ], { cwd: REPO_ROOT });
  if (result.code !== 0) {
    throw new Error(`build-release.ps1 -Version ${version} -IntegrationTest failed:\n${result.out}\n${result.err}`);
  }
}

async function waitFor(predicate, timeoutMs, intervalMs = 300) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await predicate()) return true;
    await new Promise((r) => setTimeout(r, intervalMs));
  }
  return false;
}

async function fetchStatus() {
  const res = await fetch(`${APP_BASE_URL}/api/updates/status`);
  return res.json();
}

async function main() {
  console.log('Stage 20B application-updater end-to-end verification');
  console.log(`Old version: ${OLD_VERSION}  New version: ${NEW_VERSION}`);

  const stagingDir = mkdtempSync(join(tmpdir(), 'streaming-tree-updater-verify-'));
  const installDir = join(stagingDir, 'app');
  const dataDir = join(stagingDir, 'data');
  console.log(`Hermetic staging directory: ${stagingDir}`);
  console.log('The real per-user install location and real AppData are never touched.');

  let appProcess = null;
  let fakeGithub = null;

  try {
    step(`Build the OLD release (${OLD_VERSION}, -IntegrationTest)`);
    await buildRelease(OLD_VERSION);
    const oldInstaller = readdirSync(OUTPUT_DIR).find((f) => f.endsWith('.exe'));
    expect(oldInstaller !== undefined, 'old installer produced');
    const oldInstallerPath = join(stagingDir, oldInstaller);
    copyFileSync(join(OUTPUT_DIR, oldInstaller), oldInstallerPath);
    pass(`old installer staged at ${oldInstallerPath}`);

    step(`Build the NEW release (${NEW_VERSION}, -IntegrationTest)`);
    await buildRelease(NEW_VERSION);
    const newInstaller = readdirSync(OUTPUT_DIR).find((f) => f.endsWith('.exe'));
    const newManifestPath = join(OUTPUT_DIR, 'streaming-tree-release.json');
    expect(newInstaller !== undefined, 'new installer produced');
    expect(existsSync(newManifestPath), 'new release manifest produced', newManifestPath);
    const newInstallerBytes = readFileSync(join(OUTPUT_DIR, newInstaller));
    const newManifest = JSON.parse(readFileSync(newManifestPath, 'utf8'));
    expect(newManifest.version === NEW_VERSION, 'manifest version matches the built version', newManifest);
    pass(`new installer + manifest staged (${newInstaller}, ${newInstallerBytes.length} bytes)`);

    step('Start the local fake GitHub API server');
    fakeGithub = createFakeGithubServer({ newManifest, newInstallerName: newInstaller, newInstallerBytes });
    const fakePort = await fakeGithub.listen();
    pass(`fake GitHub API server listening on 127.0.0.1:${fakePort}`);

    step('Silent install of the OLD version into the hermetic directory');
    const install = await run(oldInstallerPath, [
      '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NOICONS', `/DIR=${installDir}`,
    ]);
    expect(install.code === 0, 'silent install exits 0', install);
    const exePath = join(installDir, 'streaming-tree-server.exe');
    expect(existsSync(join(installDir, 'unins000.exe')), 'Inno Setup created the unins000.exe installed-context marker');
    expect(existsSync(join(installDir, 'unins000.dat')), 'Inno Setup created the unins000.dat installed-context marker');

    step('Launch the installed OLD version, redirected to the fake GitHub server');
    appProcess = spawn(exePath, [], {
      env: {
        STREAMING_TREE_DATA_DIR: dataDir,
        STREAMING_TREE_PORT: String(APP_PORT),
        STREAMING_TREE_HOST: '127.0.0.1',
        STREAMING_TREE_TEST_NO_UI: '1',
        STREAMING_TREE_TEST_UPDATE_API_BASE_URL: `http://127.0.0.1:${fakePort}`,
        SystemRoot: process.env.SystemRoot ?? 'C:\\Windows',
      },
      stdio: ['ignore', 'ignore', 'ignore'],
    });
    let oldExited = false;
    appProcess.on('exit', () => {
      oldExited = true;
    });

    const healthy = await waitFor(async () => {
      try {
        const r = await fetch(`${APP_BASE_URL}/api/health`);
        return r.ok;
      } catch {
        return false;
      }
    }, 30_000);
    expect(healthy, 'installed OLD executable becomes healthy');

    const initialStatus = await fetchStatus();
    expect(initialStatus.releaseBuild === true, 'updater reports releaseBuild=true', initialStatus);
    expect(initialStatus.currentVersion === OLD_VERSION, 'updater reports the real OLD current version', initialStatus);

    step('CheckNow rejects a release whose manifest version does not match its own tag');
    fakeGithub.setMode('bad-manifest-version-mismatch');
    await fetch(`${APP_BASE_URL}/api/updates/check`, { method: 'POST' });
    const afterBadManifest = await fetchStatus();
    expect(afterBadManifest.state === 'error', 'state is error after a version-mismatched manifest', afterBadManifest);
    expect(afterBadManifest.lastErrorCode === 'invalid_manifest', 'error code is invalid_manifest', afterBadManifest);
    expect(fakeGithub.hitCount('/repos/Czekosabe/streaming-tree-for-obs/releases/latest') >= 1, 'the fake server, not the real GitHub API, was actually contacted');

    step('Download rejects a tampered installer that disagrees with its own manifest SHA-256');
    fakeGithub.setMode('good-manifest-tampered-installer');
    await fetch(`${APP_BASE_URL}/api/updates/check`, { method: 'POST' });
    const afterGoodManifest = await fetchStatus();
    expect(afterGoodManifest.state === 'available', 'state is available once the manifest itself is valid', afterGoodManifest);
    await fetch(`${APP_BASE_URL}/api/updates/download`, { method: 'POST' });
    const afterTamperedDownload = await waitFor(async () => (await fetchStatus()).state !== 'downloading', 30_000)
      .then(fetchStatus);
    expect(afterTamperedDownload.state === 'error', 'state is error after a tampered download', afterTamperedDownload);
    expect(afterTamperedDownload.lastErrorCode === 'hash_mismatch', 'error code is hash_mismatch', afterTamperedDownload);

    step('The real update cycle: check, download, verify');
    fakeGithub.setMode('good');
    await fetch(`${APP_BASE_URL}/api/updates/check`, { method: 'POST' });
    const available = await fetchStatus();
    expect(available.state === 'available', 'state is available for the real new release', available);
    expect(available.latestVersion === NEW_VERSION, 'latestVersion is the real new version', available);

    await fetch(`${APP_BASE_URL}/api/updates/download`, { method: 'POST' });
    const readyOk = await waitFor(async () => (await fetchStatus()).state === 'ready_to_install', 60_000);
    expect(readyOk, 'state becomes ready_to_install after a real, verified download');
    const ready = await fetchStatus();
    expect(ready.installBlocked === false, 'install is not blocked (genuinely installed, not streaming)', ready);

    step('Install and restart: the real external-helper handoff');
    const installResp = await fetch(`${APP_BASE_URL}/api/updates/install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm: true }),
    });
    expect(installResp.ok, 'install request accepted');

    const oldStopped = await waitFor(() => oldExited, 30_000);
    expect(oldStopped, 'the old process exited on its own after the install request');
    appProcess = null;

    const restarted = await waitFor(async () => {
      try {
        const r = await fetch(`${APP_BASE_URL}/api/health`);
        return r.ok;
      } catch {
        return false;
      }
    }, 120_000);
    expect(restarted, 'the application became healthy again after the real Inno Setup silent upgrade and restart');

    step('The restarted application reports the real new version');
    const about = await fetch(`${APP_BASE_URL}/api/about`).then((r) => r.json());
    expect(about.version === NEW_VERSION, 'restarted app reports the new version', about);

    step('The one-shot post-update result is surfaced exactly once');
    const postUpdateStatus = await fetchStatus();
    expect(postUpdateStatus.postUpdateOutcome === 'ok', 'post-update outcome is ok', postUpdateStatus);
    expect(postUpdateStatus.postUpdateFromVersion === OLD_VERSION, 'post-update fromVersion is the real old version', postUpdateStatus);
    expect(postUpdateStatus.postUpdateToVersion === NEW_VERSION, 'post-update toVersion is the real new version', postUpdateStatus);
    const secondRead = await fetchStatus();
    expect(secondRead.postUpdateOutcome === undefined, 'the post-update result is cleared after being read once', secondRead);

    step('Verify the actually-installed executable on disk now reports the new version');
    const versionResult = await run(exePath, ['--version']);
    expect(versionResult.out.includes(NEW_VERSION), '--version on the real installed file reports the new version', versionResult.out);

    step('Graceful shutdown of the restarted (new) process');
    const shutdownResp = await fetch(`${APP_BASE_URL}/api/system/shutdown`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm: true }),
    });
    expect(shutdownResp.ok, 'graceful shutdown accepted');
    const stopped = await waitFor(async () => {
      try {
        await fetch(`${APP_BASE_URL}/api/health`);
        return false;
      } catch {
        return true;
      }
    }, 15_000);
    expect(stopped, 'restarted application exits after shutdown');

    step('Silent uninstall');
    const uninstallerFile = readdirSync(installDir).find((f) => /^unins\d+\.exe$/.test(f));
    expect(uninstallerFile !== undefined, 'uninstaller is present');
    const uninstall = await run(join(installDir, uninstallerFile), ['/VERYSILENT', '/SUPPRESSMSGBOXES']);
    expect(uninstall.code === 0, 'silent uninstall exits 0', uninstall);

    console.log(`\n${stepCount} steps passed. PASS`);
  } finally {
    if (appProcess !== null) {
      spawn('taskkill', ['/pid', String(appProcess.pid), '/T', '/F'], { stdio: 'ignore' });
    }
    if (fakeGithub !== null) {
      await fakeGithub.close();
    }
    // The install directory may still have a lingering helper/installer
    // process releasing file handles - retry the removal briefly rather
    // than failing the whole run over cleanup.
    for (let attempt = 0; attempt < 5; attempt += 1) {
      try {
        rmSync(stagingDir, { recursive: true, force: true });
        break;
      } catch {
        await new Promise((r) => setTimeout(r, 500));
      }
    }
  }
}

/**
 * A minimal fake GitHub Releases API - serves exactly the two endpoints
 * the real Client actually calls (docs/updater.md §2), never anything
 * resembling the real GitHub host. `setMode` switches what the next
 * `/releases/latest` response describes, so this one server can prove
 * both the rejection paths and the real successful cycle without a
 * second, separately-orchestrated fake server.
 */
function createFakeGithubServer({ newManifest, newInstallerName, newInstallerBytes }) {
  let mode = 'good';
  const hits = new Map();

  const goodManifestBytes = Buffer.from(JSON.stringify(newManifest));
  const mismatchedManifest = { ...newManifest, version: '0.9.9' }; // tag says NEW_VERSION, manifest disagrees
  const mismatchedManifestBytes = Buffer.from(JSON.stringify(mismatchedManifest));
  const tamperedInstallerBytes = Buffer.from('x'.repeat(newInstallerBytes.length)); // same size, wrong content

  const server = http.createServer((req, res) => {
    const path = req.url.split('?')[0];
    hits.set(path, (hits.get(path) ?? 0) + 1);

    if (path === '/repos/Czekosabe/streaming-tree-for-obs/releases/latest') {
      const manifestBytes = mode === 'bad-manifest-version-mismatch' ? mismatchedManifestBytes : goodManifestBytes;
      const body = {
        id: 1,
        tag_name: `v${newManifest.version}`,
        name: newManifest.version,
        draft: false,
        prerelease: false,
        body: 'Integration test release notes.',
        published_at: new Date().toISOString(),
        assets: [
          {
            id: 1,
            name: 'streaming-tree-release.json',
            size: manifestBytes.length,
            url: `http://127.0.0.1:${server.address().port}/asset/manifest`,
          },
          {
            id: 2,
            name: newInstallerName,
            size: newInstallerBytes.length,
            url: `http://127.0.0.1:${server.address().port}/asset/installer`,
          },
        ],
      };
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(body));
      return;
    }

    if (path === '/asset/manifest') {
      const manifestBytes = mode === 'bad-manifest-version-mismatch' ? mismatchedManifestBytes : goodManifestBytes;
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(manifestBytes);
      return;
    }

    if (path === '/asset/installer') {
      const bytes = mode === 'good-manifest-tampered-installer' ? tamperedInstallerBytes : newInstallerBytes;
      res.writeHead(200, { 'Content-Type': 'application/octet-stream' });
      res.end(bytes);
      return;
    }

    res.writeHead(404);
    res.end();
  });

  return {
    listen: () =>
      new Promise((resolvePromise) => {
        server.listen(0, '127.0.0.1', () => resolvePromise(server.address().port));
      }),
    close: () => new Promise((resolvePromise) => server.close(() => resolvePromise())),
    setMode: (next) => {
      mode = next;
    },
    hitCount: (path) => hits.get(path) ?? 0,
  };
}

main().catch((error) => {
  console.error('\nverify-updater.mjs FAILED');
  console.error(error);
  process.exitCode = 1;
});
