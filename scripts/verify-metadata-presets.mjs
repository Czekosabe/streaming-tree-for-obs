#!/usr/bin/env node
/**
 * Scripted metadata-preset verification (Stage 22, docs/metadata-presets.md).
 *
 * Starts the real backend against a TEMPORARY database, exercises the
 * whole preset CRUD + apply API end to end, restarts the process against
 * the same database and checks the preset survived. This is the one
 * check that proves, against the real HTTP stack (not a unit-test fake),
 * that:
 *   - a preset survives a restart,
 *   - applying it never blends one provider's category into another's,
 *   - applying it never issues any provider publish call (this script
 *     links no Twitch/YouTube account to any destination at all, so a
 *     remote publish attempt would have nothing to publish through and
 *     would fail loudly rather than silently succeed),
 *   - an all-or-nothing apply across two destinations really writes
 *     nothing when one of them is invalid,
 *   - deleting a preset never touches a destination it was applied to.
 *
 * The temporary directory is created under the OS temp location and
 * removed at the end, so the real user database is never opened, read
 * or modified.
 *
 * Usage:  node scripts/verify-metadata-presets.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

/** A port unlikely to collide with the dev server, a real backend, or another verify-*.mjs script. */
const PORT = 8210;
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
  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-verify-presets-'));
  const databasePath = join(tempDir, 'verify.db');

  console.log('Scripted metadata-preset verification');
  console.log(`Temporary database: ${databasePath}`);
  console.log('The real user database is never touched.');
  console.log('No Twitch/YouTube account is linked to any destination in this script,');
  console.log('so a real provider publish attempt during Apply would fail loudly rather');
  console.log('than silently succeed - Apply completing at all is itself proof nothing');
  console.log('was published.');

  let backend = null;
  let presetId = null;
  let crossProviderPresetId = null;

  try {
    step('Start the backend against the temporary database');
    backend = await startBackend(databasePath);
    pass('backend is ready');

    step('Give the seeded Twitch destination some initial metadata');
    const initial = await request('PUT', '/api/platforms/pf_seed_twitch/metadata', {
      title: 'Old title', description: '', category: 'Just Chatting', categoryId: '509658',
      tags: ['old'], language: 'en', visibility: '', matureContent: false, dvr: false, latencyMode: '',
    });
    expect(initial.status === 200, 'PUT the Twitch destination metadata returns 200', initial.body);

    step('Create a reusable metadata preset');
    const created = await request('POST', '/api/metadata-presets', {
      name: 'Coding stream',
      note: 'For dev streams',
      title: 'Live coding — zażółć gęślą jaźń',
      description: 'A longer description only YouTube can hold',
      tags: ['coding', 'go'],
      language: 'en',
      visibility: 'public',
      matureContent: false,
      dvr: false,
      latencyMode: '',
      providers: {
        // Deliberately DIFFERENT category identifiers per provider - the
        // exact scenario docs/metadata-presets.md §1 warns must never be
        // conflated (a Twitch game_id is not a YouTube videoCategory id).
        twitch: { category: 'Software and Game Development', categoryId: '1469308723' },
        youtube: { category: 'Science & Technology', categoryId: '28' },
      },
    });
    expect(created.status === 201, 'POST /api/metadata-presets returns 201', created.body);
    presetId = created.body.id;
    expect(typeof presetId === 'string' && presetId.length > 0, 'the backend generated a preset id');
    expect(created.body.providers.twitch.categoryId === '1469308723', 'the Twitch category id round-trips');
    expect(created.body.providers.youtube.categoryId === '28', 'the YouTube category id round-trips');

    step('Stop the backend');
    await stopBackend(backend);
    backend = null;
    pass('backend stopped cleanly');

    step('Restart the backend against the same database');
    backend = await startBackend(databasePath);
    pass('backend restarted');

    step('Verify the preset survived the restart');
    const afterRestart = await request('GET', `/api/metadata-presets/${presetId}`);
    expect(afterRestart.status === 200, 'the preset still exists after restart', afterRestart.status);
    expect(afterRestart.body.name === 'Coding stream', 'the preset name survived the restart');
    expect(
      afterRestart.body.title === 'Live coding — zażółć gęślą jaźń',
      'the Unicode title survived the restart',
      afterRestart.body.title,
    );

    step('Preview applying the preset to the Twitch destination');
    const preview = await request(
      'GET',
      `/api/metadata-presets/${presetId}/apply-preview?platformIds=pf_seed_twitch`,
    );
    expect(preview.status === 200, 'apply-preview returns 200', preview.body);
    const twitchPreview = preview.body[0];
    expect(twitchPreview.valid === true, 'the candidate validates for Twitch', twitchPreview);
    const fieldStatus = Object.fromEntries(twitchPreview.fields.map((f) => [f.field, f.status]));
    expect(fieldStatus.title === 'will_change', 'title is classified will_change (differs from "Old title")');
    expect(fieldStatus.category === 'will_change', 'category is classified will_change');
    expect(
      fieldStatus.description === 'not_supported',
      'description is classified not_supported for Twitch (docs/metadata-presets.md §1)',
      fieldStatus,
    );
    expect(fieldStatus.visibility === 'not_supported', 'visibility is classified not_supported for Twitch');

    step('Apply the preset to the Twitch destination');
    const applied = await request('POST', `/api/metadata-presets/${presetId}/apply`, {
      platformIds: ['pf_seed_twitch'],
    });
    expect(applied.status === 200, 'apply returns 200', applied.body);
    const appliedTwitch = applied.body.platforms.pf_seed_twitch;
    expect(appliedTwitch.title === 'Live coding — zażółć gęślą jaźń', 'the applied title matches the preset');
    expect(appliedTwitch.category === 'Software and Game Development', 'the Twitch category was applied');
    expect(appliedTwitch.categoryId === '1469308723', 'the Twitch category id was applied - not YouTube\'s');
    expect(appliedTwitch.description === '', 'description stayed empty - Twitch does not support it');

    step('Re-read the Twitch destination and confirm the write actually landed');
    const rereadTwitch = await request('GET', '/api/platforms/pf_seed_twitch/metadata');
    expect(rereadTwitch.status === 200, 'GET the Twitch destination metadata returns 200');
    expect(
      rereadTwitch.body.title === 'Live coding — zażółć gęślą jaźń',
      'the destination itself now holds the applied title',
      rereadTwitch.body.title,
    );
    expect(
      JSON.stringify(rereadTwitch.body.tags) === JSON.stringify(['coding', 'go']),
      'the destination now holds the preset\'s tags',
      rereadTwitch.body.tags,
    );

    step('Create a second preset scoped to BOTH Twitch and YouTube, to prove cross-provider isolation at apply time');
    const crossPreset = await request('POST', '/api/metadata-presets', {
      name: 'Cross-provider check', note: '', title: 'Same title everywhere', description: '',
      tags: [], language: 'en', visibility: '', matureContent: false, dvr: false, latencyMode: '',
      providers: {
        twitch: { category: 'Just Chatting', categoryId: '509658' },
        youtube: { category: 'Science & Technology', categoryId: '28' },
      },
    });
    expect(crossPreset.status === 201, 'creating the cross-provider preset returns 201', crossPreset.body);
    crossProviderPresetId = crossPreset.body.id;

    const appliedYouTube = await request('POST', `/api/metadata-presets/${crossProviderPresetId}/apply`, {
      platformIds: ['pf_seed_youtube'],
    });
    expect(appliedYouTube.status === 200, 'applying to the YouTube destination returns 200', appliedYouTube.body);
    const appliedYT = appliedYouTube.body.platforms.pf_seed_youtube;
    expect(
      appliedYT.categoryId === '28',
      'the YouTube destination received the YouTube category id, never the Twitch one',
      appliedYT,
    );
    expect(appliedYT.categoryId !== '509658', 'the Twitch category id was never applied to YouTube');

    step('Attempt an all-or-nothing apply where one destination is invalid');
    const beforeBadApply = await request('GET', '/api/platforms/pf_seed_youtube/metadata');
    const badApply = await request('POST', `/api/metadata-presets/${presetId}/apply`, {
      // pf_seed_youtube would validate fine on its own; pf_missing does not
      // exist at all, so the whole request must be rejected and NOTHING
      // written for pf_seed_youtube either.
      platformIds: ['pf_seed_youtube', 'pf_missing'],
    });
    expect(
      badApply.status === 404 || badApply.status === 422,
      'the all-or-nothing apply is rejected rather than partially applied',
      badApply,
    );
    const afterBadApply = await request('GET', '/api/platforms/pf_seed_youtube/metadata');
    expect(
      afterBadApply.body.title === beforeBadApply.body.title,
      'the YouTube destination is untouched after the rejected all-or-nothing apply',
      { before: beforeBadApply.body.title, after: afterBadApply.body.title },
    );

    step('Delete the first preset');
    const deleted = await request('DELETE', `/api/metadata-presets/${presetId}`);
    expect(deleted.status === 204, 'DELETE returns 204', deleted.status);
    presetId = null;

    step('Verify the destination it was applied to keeps its metadata unchanged');
    const afterDelete = await request('GET', '/api/platforms/pf_seed_twitch/metadata');
    expect(afterDelete.status === 200, 'GET the Twitch destination metadata returns 200');
    expect(
      afterDelete.body.title === 'Live coding — zażółć gęślą jaźń',
      'the destination\'s already-applied metadata survives the preset\'s deletion',
      afterDelete.body.title,
    );

    step('Stop the backend');
    await stopBackend(backend);
    backend = null;
    pass('backend stopped cleanly');

    console.log('\nMetadata-preset verification PASSED');
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
  console.error(`\nMetadata-preset verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
