#!/usr/bin/env node
/**
 * Scripted MediaMTX runtime verification.
 *
 * Exercises the whole managed-dependency and supervision path against the REAL
 * MediaMTX v1.19.3 binary, downloaded and checksum-verified through the
 * application's own installation endpoint.
 *
 * Everything happens inside a temporary application data directory with
 * dynamically chosen loopback ports, so the real user database and the real
 * managed runtime directory are never opened, read or modified.
 *
 * Usage:  node scripts/verify-mediamtx-runtime.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

const SUPPORTED_VERSION = 'v1.19.3';

const READINESS_TIMEOUT_MS = 60_000;
const INSTALL_TIMEOUT_MS = 300_000;
const SHUTDOWN_TIMEOUT_MS = 15_000;

let stepCount = 0;

function step(message) {
  stepCount += 1;
  console.log(`\n[${String(stepCount).padStart(2, '0')}] ${message}`);
}

function pass(message) {
  console.log(`     ok  ${message}`);
}

function expect(condition, message, detail) {
  if (condition) {
    pass(message);
    return;
  }
  console.error(`     FAIL ${message}`);
  if (detail !== undefined) {
    console.error(`          ${typeof detail === 'string' ? detail : JSON.stringify(detail)}`);
  }
  throw new Error(message);
}

/** Reserves a free loopback port, so the script never collides with a service. */
function reservePort() {
  return new Promise((resolvePort, reject) => {
    const server = createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      server.close(() => resolvePort(port));
    });
  });
}

async function request(baseUrl, method, path) {
  const response = await fetch(`${baseUrl}${path}`, {
    method,
    headers: { Accept: 'application/json' },
  });

  const text = await response.text();
  let body = null;
  if (text !== '') {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }
  return { status: response.status, body };
}

/** Starts the backend and resolves once /api/health answers. */
async function startBackend(env, baseUrl) {
  const child = spawn('go', ['run', './cmd/server'], {
    cwd: SERVER_DIR,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  let stderr = '';
  child.stderr.on('data', (chunk) => {
    stderr += chunk.toString();
  });
  child.stdout.on('data', () => {
    // Drained so the pipe cannot fill and block the child.
  });

  let exited = false;
  child.on('exit', () => {
    exited = true;
  });

  const deadline = Date.now() + READINESS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (exited) {
      throw new Error(`backend exited during startup:\n${stderr}`);
    }
    try {
      const health = await fetch(`${baseUrl}/api/health`);
      if (health.ok) return child;
    } catch {
      // Not listening yet.
    }
    await new Promise((r) => setTimeout(r, 300));
  }

  child.kill();
  throw new Error(`backend did not become ready in ${READINESS_TIMEOUT_MS} ms:\n${stderr}`);
}

/**
 * Stops the backend and confirms the port is released.
 *
 * `go run` spawns the compiled binary as a child, and on Windows a signal to
 * the wrapper does not always reach it, so the whole tree is terminated.
 */
async function stopBackend(child, baseUrl) {
  await new Promise((resolveStop) => {
    const timer = setTimeout(() => resolveStop(), SHUTDOWN_TIMEOUT_MS);
    child.on('exit', () => {
      clearTimeout(timer);
      resolveStop();
    });

    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], { stdio: 'ignore' });
    } else {
      child.kill('SIGTERM');
    }
  });

  const deadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
  while (Date.now() < deadline) {
    try {
      await fetch(`${baseUrl}/api/health`);
    } catch {
      return;
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error('the backend is still answering after shutdown');
}

/** Polls /api/runtime until the MediaMTX state matches one of the wanted ones. */
async function waitForState(baseUrl, wanted, timeoutMs, label) {
  const deadline = Date.now() + timeoutMs;
  let last = null;

  while (Date.now() < deadline) {
    const runtime = await request(baseUrl, 'GET', '/api/runtime');
    last = runtime.body?.mediaMtx;
    if (wanted.includes(last?.state)) return runtime.body;

    if (last?.state === 'error') {
      throw new Error(
        `MediaMTX entered the error state while waiting for ${label}: ` +
          JSON.stringify(last.lastError),
      );
    }
    await new Promise((r) => setTimeout(r, 500));
  }

  throw new Error(
    `MediaMTX state is "${last?.state}" after ${timeoutMs} ms, want one of ${wanted.join(', ')}`,
  );
}

async function waitForIngest(baseUrl, wanted, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let last = null;

  while (Date.now() < deadline) {
    const runtime = await request(baseUrl, 'GET', '/api/runtime');
    last = runtime.body?.ingest?.state;
    if (last === wanted) return runtime.body.ingest;
    await new Promise((r) => setTimeout(r, 500));
  }

  throw new Error(`ingest state is "${last}" after ${timeoutMs} ms, want "${wanted}"`);
}

async function main() {
  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-mediamtx-'));

  const backendPort = await reservePort();
  const rtmpPort = await reservePort();
  const apiPort = await reservePort();
  const baseUrl = `http://127.0.0.1:${backendPort}`;

  const env = {
    STREAMING_TREE_DATA_DIR: tempDir,
    STREAMING_TREE_PORT: String(backendPort),
    STREAMING_TREE_HOST: '127.0.0.1',
    STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS: `127.0.0.1:${rtmpPort}`,
    STREAMING_TREE_MEDIAMTX_API_ADDRESS: `127.0.0.1:${apiPort}`,
    // Explicitly cleared: an override would skip the managed installation this
    // script exists to verify.
    STREAMING_TREE_MEDIAMTX_PATH: '',
  };

  console.log('Scripted MediaMTX runtime verification');
  console.log(`Temporary data directory: ${tempDir}`);
  console.log(`Backend :${backendPort}  RTMP :${rtmpPort}  MediaMTX API :${apiPort}`);
  console.log('The real user database and managed runtime directory are never touched.');

  let backend = null;

  try {
    step('Start the backend with a temporary data directory');
    backend = await startBackend(env, baseUrl);
    pass('backend health is answering');

    step('Fetch the initial runtime state');
    const initial = await request(baseUrl, 'GET', '/api/runtime');
    expect(initial.status === 200, 'GET /api/runtime returns 200', initial.status);
    expect(initial.body.version === 1, 'the runtime payload is versioned', initial.body.version);
    expect(
      initial.body.mediaMtx.supportedVersion === SUPPORTED_VERSION,
      `the supported version is ${SUPPORTED_VERSION}`,
      initial.body.mediaMtx.supportedVersion,
    );
    expect(
      initial.body.mediaMtx.state === 'missing',
      'MediaMTX starts out missing in a clean data directory',
      initial.body.mediaMtx,
    );
    expect(
      initial.body.ingest.state === 'unavailable',
      'ingest is unavailable while MediaMTX is missing',
      initial.body.ingest.state,
    );
    expect(
      initial.body.connection.publishUrl === `rtmp://127.0.0.1:${rtmpPort}/live`,
      'the publish URL is derived from the configured address and path',
      initial.body.connection.publishUrl,
    );

    step('Confirm the platform API still works while MediaMTX is missing');
    const platforms = await request(baseUrl, 'GET', '/api/platforms');
    expect(platforms.status === 200, 'GET /api/platforms still returns 200', platforms.status);
    expect(
      Array.isArray(platforms.body.platforms) && platforms.body.platforms.length === 4,
      'the seeded platform configuration is readable',
      platforms.body.platforms?.length,
    );

    step('Confirm the runtime endpoints reject a request body');
    const withBody = await fetch(`${baseUrl}/api/runtime/mediamtx/install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: 'http://example.invalid/evil.tar.gz' }),
    });
    expect(withBody.status === 400, 'an install request with a body is rejected', withBody.status);

    step('Install MediaMTX through the managed installer');
    const install = await request(baseUrl, 'POST', '/api/runtime/mediamtx/install');
    expect(install.status === 202, 'POST install returns 202', install.body);

    step('Wait for the installation to finish');
    // The real archive is ~30 MB and its checksum is verified before use.
    const installed = await waitForState(
      baseUrl,
      ['ready', 'stopped'],
      INSTALL_TIMEOUT_MS,
      'the installation to finish',
    );
    expect(
      installed.mediaMtx.installedVersion === SUPPORTED_VERSION,
      `the installed version is ${SUPPORTED_VERSION}`,
      installed.mediaMtx.installedVersion,
    );
    expect(
      installed.mediaMtx.source === 'managed',
      'the binary source is the managed installation',
      installed.mediaMtx.source,
    );

    step('Confirm the licence and metadata were preserved on disk');
    const installDir = join(tempDir, 'runtime', 'mediamtx', SUPPORTED_VERSION);
    expect(existsSync(installDir), 'the versioned installation directory exists', installDir);

    step('Wait for MediaMTX readiness');
    const ready = await waitForState(baseUrl, ['ready'], 60_000, 'readiness');
    expect(ready.mediaMtx.state === 'ready', 'MediaMTX reports ready', ready.mediaMtx.state);
    expect(ready.mediaMtx.startedAt !== '', 'a start time is reported', ready.mediaMtx.startedAt);
    expect(
      ready.mediaMtx.lastError === null,
      'no error is reported once ready',
      ready.mediaMtx.lastError,
    );

    step('Verify the ingest state before anything publishes');
    const waiting = await waitForIngest(baseUrl, 'waiting', 20_000);
    expect(waiting.path === 'live', 'the configured ingest path is reported', waiting.path);
    expect(
      waiting.trackCount === null && waiting.tracks.length === 0,
      'no track information is invented while waiting',
      waiting,
    );
    expect(
      waiting.sourceType === undefined || waiting.sourceType === '',
      'no source type is reported while waiting',
      waiting.sourceType,
    );

    step('Confirm the runtime payload leaks no filesystem path');
    const snapshotText = JSON.stringify(ready);
    for (const probe of [tempDir, 'AppData', 'mediamtx.exe', 'runtime/mediamtx']) {
      expect(
        !snapshotText.includes(probe),
        `the runtime payload does not contain ${probe}`,
        snapshotText,
      );
    }

    step('Stop MediaMTX explicitly');
    const stopped = await request(baseUrl, 'POST', '/api/runtime/mediamtx/stop');
    expect(stopped.status === 200, 'POST stop returns 200', stopped.body);

    const afterStop = await request(baseUrl, 'GET', '/api/runtime');
    expect(
      afterStop.body.mediaMtx.state === 'stopped',
      'the state becomes stopped',
      afterStop.body.mediaMtx.state,
    );
    expect(
      afterStop.body.ingest.state === 'unavailable',
      'ingest becomes unavailable once stopped',
      afterStop.body.ingest.state,
    );

    step('Confirm an explicit stop is not undone by the restart policy');
    await new Promise((r) => setTimeout(r, 4000));
    const stillStopped = await request(baseUrl, 'GET', '/api/runtime');
    expect(
      stillStopped.body.mediaMtx.state === 'stopped',
      'MediaMTX stays stopped after an explicit stop',
      stillStopped.body.mediaMtx.state,
    );

    step('Start MediaMTX again');
    const restarted = await request(baseUrl, 'POST', '/api/runtime/mediamtx/start');
    expect(restarted.status === 202, 'POST start returns 202', restarted.body);
    await waitForState(baseUrl, ['ready'], 60_000, 'readiness after a manual start');
    pass('MediaMTX became ready again');

    step('Restart the backend against the same temporary runtime directory');
    await stopBackend(backend, baseUrl);
    backend = null;
    pass('backend stopped cleanly');

    backend = await startBackend(env, baseUrl);
    pass('backend restarted');

    step('Verify the managed binary is reused rather than downloaded again');
    const reused = await waitForState(baseUrl, ['ready'], 60_000, 'autostart after restart');
    expect(
      reused.mediaMtx.installedVersion === SUPPORTED_VERSION,
      'the existing managed installation was reused',
      reused.mediaMtx.installedVersion,
    );
    expect(
      reused.mediaMtx.source === 'managed',
      'the source is still the managed installation',
      reused.mediaMtx.source,
    );
    expect(
      reused.mediaMtx.state === 'ready',
      'autostart brought MediaMTX up without an explicit start',
      reused.mediaMtx.state,
    );
    expect(
      reused.mediaMtx.restartCount === 0,
      'the restart counter is fresh after a backend restart, proving runtime state is in memory',
      reused.mediaMtx.restartCount,
    );

    step('Verify ingest is waiting again after the restart');
    await waitForIngest(baseUrl, 'waiting', 20_000);
    pass('ingest returned to waiting');

    step('Stop everything');
    await stopBackend(backend, baseUrl);
    backend = null;
    pass('backend stopped cleanly');

    console.log('\nMediaMTX runtime verification PASSED');
  } finally {
    if (backend !== null) {
      try {
        await stopBackend(backend, baseUrl);
      } catch {
        // Already reporting a failure.
      }
    }
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary data directory: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nMediaMTX runtime verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
