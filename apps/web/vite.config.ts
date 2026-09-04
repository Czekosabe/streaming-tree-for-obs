import { fileURLToPath, URL } from 'node:url';

import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';
import { visualizer } from 'rollup-plugin-visualizer';

/**
 * Vite configuration.
 *
 * The dev server proxies `/api` to the local Go backend so that the frontend
 * can use same-origin relative URLs. This keeps the browser free of CORS
 * pre-flight requests during development while the backend still ships its own
 * CORS middleware for the cases where the frontend is served from a different
 * origin (for example a future remote deployment). `preview` (`vite preview`,
 * serving the real production `dist/` build) gets the identical proxy for the
 * same reason - it is how a real production build's startup/network
 * behavior gets verified against a real backend, not only inferred from
 * source (see the performance-hardening pass in docs/progress.md).
 *
 * `ANALYZE=1 npm run build` additionally emits `dist/bundle-analysis.json`
 * (rollup-plugin-visualizer's raw module/chunk composition data) - not part
 * of a normal build, and adds nothing to it; kept for the next time bundle
 * composition needs auditing rather than guessed at.
 */
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const backendTarget = env.VITE_DEV_API_PROXY_TARGET ?? 'http://127.0.0.1:8080';

  return {
    plugins: [
      react(),
      tailwindcss(),
      process.env.ANALYZE === '1' &&
        visualizer({
          filename: 'dist/bundle-analysis.json',
          template: 'raw-data',
          gzipSize: true,
          brotliSize: true,
        }),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 5173,
      strictPort: false,
      proxy: {
        '/api': {
          target: backendTarget,
          changeOrigin: true,
          // The backend may simply not be running yet. Vite logs the failure and
          // the frontend renders its "Backend unavailable" state.
          timeout: 5_000,
        },
      },
    },
    build: {
      outDir: 'dist',
      sourcemap: true,
    },
    preview: {
      proxy: {
        '/api': {
          target: backendTarget,
          changeOrigin: true,
          timeout: 5_000,
        },
      },
    },
  };
});
