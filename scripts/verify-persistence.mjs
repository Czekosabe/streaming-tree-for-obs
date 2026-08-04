#!/usr/bin/env node
/**
 * Scripted persistence verification.
 *
 * Starts the real backend against a TEMPORARY database, exercises the platform
 * configuration API, restarts the process against the SAME database and checks
 * that everything survived. This is what proves persistence end to end; unit
 * tests alone cannot show that data outlives the process.
 *
 * The temporary directory is created under the OS temp location and removed at
 * the end, so the real user database is never opened, read or modified.
 *
 * Usage:  node scripts/verify-persistence.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

/** A port unlikely to collide with the dev server or a real backend. */
const PORT = 8199;
const BASE_URL = `http://127.0.0.1:${PORT}`;

const READINESS_TIMEOUT_MS = 30_000;
const SHUTDOWN_TIMEOUT_MS = 10_000;

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

  let payload = null;
  const text = await response.text();
  if (text !== '') {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }
  }

  return { status: response.status, headers: response.headers, body: payload };
}

/** Starts the backend and resolves once /api/health answers. */
async function startBackend(databasePath) {
  const child = spawn('go', ['run', './cmd/server'], {
    cwd: SERVER_DIR,
    env: {
      ...process.env,
      STREAMING_TREE_DB_PATH: databasePath,
      STREAMING_TREE_PORT: String(PORT),
      STREAMING_TREE_HOST: '127.0.0.1',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  let stderr = '';
  child.stderr.on('data', (chunk) => {
    stderr += chunk.toString();
  });
  child.stdout.on('data', () => {
    // Consumed so the pipe never fills up and blocks the child.
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
      const health = await fetch(`${BASE_URL}/api/health`);
      if (health.ok) {
        return child;
      }
    } catch {
      // Not listening yet.
    }
    await new Promise((r) => setTimeout(r, 300));
  }

  child.kill();
  throw new Error(`backend did not become ready within ${READINESS_TIMEOUT_MS} ms:\n${stderr}`);
}

/**
 * Stops the backend.
 *
 * `go run` spawns the compiled binary as a child, and on Windows a signal to
 * the wrapper does not always reach it, so the whole process tree is killed and
 * the port is then confirmed closed.
 */
async function stopBackend(child) {
  await new Promise((resolvePromise) => {
    const timer = setTimeout(() => resolvePromise(), SHUTDOWN_TIMEOUT_MS);
    child.on('exit', () => {
      clearTimeout(timer);
      resolvePromise();
    });

    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], { stdio: 'ignore' });
    } else {
      child.kill('SIGTERM');
    }
  });

  // Wait until nothing answers on the port, so the restart binds cleanly.
  const deadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
  while (Date.now() < deadline) {
    try {
      await fetch(`${BASE_URL}/api/health`);
    } catch {
      return;
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error('the backend is still answering after the shutdown request');
}

async function main() {
  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-verify-'));
  const databasePath = join(tempDir, 'verify.db');

  console.log('Scripted persistence verification');
  console.log(`Temporary database: ${databasePath}`);
  console.log('The real user database is never touched.');

  let backend = null;
  let createdId = null;

  try {
    step('Start the backend against the temporary database');
    backend = await startBackend(databasePath);
    pass('backend is ready');

    step('Fetch provider definitions');
    const definitions = await request('GET', '/api/platform-definitions');
    expect(definitions.status === 200, 'GET /api/platform-definitions returns 200', definitions.status);
    expect(
      Array.isArray(definitions.body?.definitions) && definitions.body.definitions.length === 4,
      'four built-in provider definitions are returned',
      definitions.body,
    );
    const twitchDefinition = definitions.body.definitions.find((d) => d.id === 'twitch');
    expect(twitchDefinition?.capabilities?.tags === true, 'Twitch reports tag support');

    step('Fetch the seeded configured platforms');
    const seeded = await request('GET', '/api/platforms');
    expect(seeded.status === 200, 'GET /api/platforms returns 200', seeded.status);
    expect(seeded.body.platforms.length === 4, 'four seeded platforms exist', seeded.body.platforms.length);
    expect(
      seeded.body.platforms.every((p) => p.enabled === false),
      'every seeded platform is disabled',
    );

    step('Create another configured platform');
    const created = await request('POST', '/api/platforms', {
      providerId: 'twitch',
      displayName: 'Verification channel',
      enabled: false,
    });
    expect(created.status === 201, 'POST /api/platforms returns 201', created.body);
    createdId = created.body.id;
    expect(typeof createdId === 'string' && createdId.length > 0, 'the backend generated an id');
    expect(
      created.headers.get('location') === `/api/platforms/${createdId}`,
      'the Location header points at the new resource',
      created.headers.get('location'),
    );

    step('Update the display name and enabled state');
    const updated = await request('PUT', `/api/platforms/${createdId}`, {
      displayName: 'Verification channel renamed',
      enabled: true,
      sortOrder: 99,
    });
    expect(updated.status === 200, 'PUT /api/platforms/{id} returns 200', updated.body);
    expect(updated.body.displayName === 'Verification channel renamed', 'the display name was updated');
    expect(updated.body.enabled === true, 'the enabled flag was updated');

    step('Save metadata with ordered Twitch tags');
    const tags = ['zebra', 'alpha', 'middle'];
    const savedMetadata = await request('PUT', `/api/platforms/${createdId}/metadata`, {
      title: 'Persistence check — zażółć gęślą jaźń',
      description: '',
      category: 'Software and Game Development',
      tags,
      language: 'pl',
      visibility: '',
      // Twitch has neither a mature-content flag nor a latency-mode field on
      // its real Modify Channel Information endpoint (verified in stage
      // 7A - see docs/provider-integrations/twitch.md); both capabilities
      // were corrected from an earlier approximation, so a Twitch platform
      // must send their default/unsupported values here.
      matureContent: false,
      dvr: false,
      latencyMode: '',
    });
    expect(savedMetadata.status === 200, 'PUT metadata returns 200', savedMetadata.body);
    expect(
      JSON.stringify(savedMetadata.body.tags) === JSON.stringify(tags),
      'tags come back in the order they were sent',
      savedMetadata.body.tags,
    );

    step('Read the metadata back and verify exact persistence');
    const reread = await request('GET', `/api/platforms/${createdId}/metadata`);
    expect(reread.status === 200, 'GET metadata returns 200', reread.status);
    expect(
      reread.body.title === 'Persistence check — zażółć gęślą jaźń',
      'the Unicode title is stored exactly as entered',
      reread.body.title,
    );
    expect(
      JSON.stringify(reread.body.tags) === JSON.stringify(tags),
      'tag order survived the round trip',
      reread.body.tags,
    );
    expect(reread.body.matureContent === false, 'the mature content flag persisted (unsupported by Twitch, so false)');
    expect(reread.body.latencyMode === '', 'the latency mode persisted (unsupported by Twitch, so empty)');

    step('Stop the backend');
    await stopBackend(backend);
    backend = null;
    pass('backend stopped cleanly');

    step('Restart the backend against the same database');
    backend = await startBackend(databasePath);
    pass('backend restarted');

    step('Verify the created platform survived the restart');
    const afterRestart = await request('GET', `/api/platforms/${createdId}`);
    expect(afterRestart.status === 200, 'the created platform still exists', afterRestart.status);
    expect(
      afterRestart.body.displayName === 'Verification channel renamed',
      'the updated display name survived the restart',
      afterRestart.body.displayName,
    );
    expect(afterRestart.body.enabled === true, 'the enabled flag survived the restart');
    expect(
      JSON.stringify(afterRestart.body.metadata.tags) === JSON.stringify(tags),
      'ordered tags survived the restart',
      afterRestart.body.metadata.tags,
    );
    expect(
      afterRestart.body.metadata.title === 'Persistence check — zażółć gęślą jaźń',
      'the metadata title survived the restart',
    );

    step('Verify the seed did not run a second time');
    const afterRestartList = await request('GET', '/api/platforms');
    expect(
      afterRestartList.body.platforms.length === 5,
      'there are still 5 platforms (4 seeded + 1 created), so the seed did not repeat',
      afterRestartList.body.platforms.length,
    );

    step('Delete the created platform');
    const deleted = await request('DELETE', `/api/platforms/${createdId}`);
    expect(deleted.status === 204, 'DELETE returns 204', deleted.status);

    step('Verify it is gone');
    const gone = await request('GET', `/api/platforms/${createdId}`);
    expect(gone.status === 404, 'GET after delete returns 404', gone.status);
    expect(gone.body?.error === 'not_found', 'the 404 uses the stable error envelope', gone.body);

    step('Stop the backend');
    await stopBackend(backend);
    backend = null;
    pass('backend stopped cleanly');

    console.log('\nPersistence verification PASSED');
  } finally {
    if (backend !== null) {
      try {
        await stopBackend(backend);
      } catch {
        // Already reporting a failure; nothing more useful to do here.
      }
    }
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary database directory: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nPersistence verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
