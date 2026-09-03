import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';

import { BACKEND_BASE_URL, FRONTEND_BASE_URL } from './env.mjs';

/**
 * Real-browser regression suite for the operator dashboard frontend
 * (apps/web). See docs/development.md's "Real-browser E2E tests" section
 * for what this suite covers, what it deliberately does not replace, and
 * how to run it locally.
 *
 * Runs against a hermetic backend (`scripts/run-backend.mjs`, the same
 * `-tags integration` test server every `scripts/verify-*.mjs` script
 * uses) and the real Vite dev server (`scripts/run-frontend.mjs`) - never
 * the operator's real installed application, real credentials, or a
 * production build.
 *
 * A single worker keeps every test serialized against the one shared
 * backend/database this suite starts, so tests can rely on deterministic
 * fixture state (the four seeded platforms, a controllable onboarding
 * status) without racing each other - this is a bounded regression suite,
 * not a large parallel matrix, so the loss of test-level parallelism is an
 * acceptable, deliberate trade for reliability and CI cost.
 */
export default defineConfig({
  testDir: './specs',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never', outputFolder: 'e2e/report' }]] : 'list',
  timeout: 30_000,
  expect: { timeout: 5_000 },

  use: {
    baseURL: FRONTEND_BASE_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'off',
  },

  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],

  webServer: [
    {
      command: `node "${fileURLToPath(new URL('./scripts/run-backend.mjs', import.meta.url))}"`,
      url: `${BACKEND_BASE_URL}/api/health`,
      timeout: 120_000,
      reuseExistingServer: !process.env.CI,
      stdout: 'pipe',
      stderr: 'pipe',
    },
    {
      command: `node "${fileURLToPath(new URL('./scripts/run-frontend.mjs', import.meta.url))}"`,
      url: FRONTEND_BASE_URL,
      timeout: 60_000,
      reuseExistingServer: !process.env.CI,
      stdout: 'pipe',
      stderr: 'pipe',
    },
  ],
});
