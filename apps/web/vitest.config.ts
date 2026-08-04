import { fileURLToPath, URL } from 'node:url';

import { defineConfig } from 'vitest/config';

/**
 * Test configuration.
 *
 * `jsdom` is required because the localization layer touches `localStorage` and
 * `document.documentElement.lang`. No browser automation is involved.
 *
 * `.test.tsx` is included alongside `.test.ts` for the small, deliberately
 * narrow set of rendered-component tests (React Testing Library) covering
 * the Twitch device-flow modal and other interaction-heavy UI added in
 * stage 7A - see docs/progress.md for why a full component-rendering
 * harness was not adopted project-wide.
 */
export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['./vitest.setup.ts'],
    restoreMocks: true,
  },
});
