#!/usr/bin/env node
/**
 * Playwright `webServer` entry: builds and runs the same `-tags integration`
 * test server every `scripts/verify-*.mjs` script already uses
 * (`apps/server/cmd/testserver`) - never `cmd/server`, never the operator's
 * real installed application. Its own doc comment
 * (`apps/server/cmd/testserver/main.go`) is the authority on why this is
 * safe: an in-memory fake credential store instead of the OS keychain, a
 * fake TTS provider instead of real SAPI, and no MediaMTX/FFmpeg binary
 * required (both paths are left empty below, exactly like the existing
 * verify scripts).
 *
 * A fresh, unique temp data directory is created on every run, so this
 * suite never reads or writes the operator's real application data
 * directory and never depends on state left behind by a previous run.
 *
 * Playwright itself is responsible for killing this process (and, on
 * Windows, the process tree beneath it - the same reason every
 * scripts/verify-*.mjs script uses `taskkill /T /F` in its own teardown)
 * once the test run ends or the `url` health check stops being needed; this
 * script does not need its own signal-forwarding logic.
 */
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { BACKEND_PORT } from '../env.mjs';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

function run(cmd, args, opts) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(cmd, args, { stdio: 'inherit', ...opts });
    child.on('error', reject);
    child.on('exit', (code) => {
      if (code === 0) resolvePromise();
      else reject(new Error(`${cmd} ${args.join(' ')} exited with code ${code}`));
    });
  });
}

const dataDir = mkdtempSync(join(tmpdir(), 'streaming-tree-e2e-backend-'));
const exePath = join(dataDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');

console.log(`[e2e backend] building the integration test server -> ${exePath}`);
await run('go', ['build', '-tags', 'integration', '-o', exePath, './cmd/testserver'], { cwd: SERVER_DIR });

console.log(`[e2e backend] starting on 127.0.0.1:${BACKEND_PORT}, data dir ${dataDir}`);
const child = spawn(exePath, [], {
  cwd: SERVER_DIR,
  stdio: 'inherit',
  env: {
    ...process.env,
    STREAMING_TREE_HOST: '127.0.0.1',
    STREAMING_TREE_PORT: String(BACKEND_PORT),
    STREAMING_TREE_DATA_DIR: dataDir,
    // No MediaMTX/FFmpeg binary is ever downloaded or required for this
    // suite - it exercises frontend UI/routing, not real ingest/streaming.
    STREAMING_TREE_MEDIAMTX_PATH: '',
    STREAMING_TREE_FFMPEG_PATH: '',
  },
});

child.on('exit', (code) => process.exit(code ?? 0));
child.on('error', (error) => {
  console.error('[e2e backend] failed to start:', error);
  process.exit(1);
});
