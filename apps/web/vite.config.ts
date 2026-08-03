import { fileURLToPath, URL } from 'node:url';

import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';

/**
 * Vite configuration.
 *
 * The dev server proxies `/api` to the local Go backend so that the frontend
 * can use same-origin relative URLs. This keeps the browser free of CORS
 * pre-flight requests during development while the backend still ships its own
 * CORS middleware for the cases where the frontend is served from a different
 * origin (for example a future remote deployment).
 */
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const backendTarget = env.VITE_DEV_API_PROXY_TARGET ?? 'http://127.0.0.1:8080';

  return {
    plugins: [react(), tailwindcss()],
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
  };
});
