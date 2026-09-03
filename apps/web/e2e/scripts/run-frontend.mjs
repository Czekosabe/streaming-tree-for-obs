#!/usr/bin/env node
/**
 * Playwright `webServer` entry: runs the real Vite dev server (the same one
 * `npm run dev` uses), pointed at the hermetic backend `run-backend.mjs`
 * starts, via the dev-only `VITE_DEV_API_PROXY_TARGET` proxy target Vite's
 * own config (`apps/web/vite.config.ts`) already supports - never a build,
 * never a second frontend implementation.
 *
 * A real browser therefore always sees a same-origin `/api/...` on
 * `FRONTEND_PORT`, exactly as it does in normal development; the proxy
 * (running in this Node process, not the browser) forwards each request to
 * the backend on `BACKEND_PORT`.
 */
import { spawn } from 'node:child_process';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { BACKEND_BASE_URL, FRONTEND_PORT } from '../env.mjs';

const WEB_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
// Invoked directly via `node <vite-cli.js>` rather than `npx vite` - `npx`
// resolves to a `.cmd` shim on Windows, which Node's `spawn` cannot exec
// without `shell: true`; running the CLI script straight through the same
// Node binary this script itself runs on is simpler and cross-platform.
const VITE_CLI = join(WEB_DIR, 'node_modules', 'vite', 'bin', 'vite.js');

// `--host 127.0.0.1` matters, not just cosmetic: Vite's own default host
// binding resolved only to the IPv6 loopback on this environment, which a
// literal `127.0.0.1` (IPv4) connection - including this suite's own
// FRONTEND_BASE_URL and Playwright's webServer readiness check - could
// never reach, causing every request to hang until it timed out.
const child = spawn(
  process.execPath,
  [VITE_CLI, '--host', '127.0.0.1', '--port', String(FRONTEND_PORT), '--strictPort'],
  {
    cwd: WEB_DIR,
    stdio: 'inherit',
    env: {
      ...process.env,
      VITE_DEV_API_PROXY_TARGET: BACKEND_BASE_URL,
    },
  },
);

child.on('exit', (code) => process.exit(code ?? 0));
child.on('error', (error) => {
  console.error('[e2e frontend] failed to start:', error);
  process.exit(1);
});
