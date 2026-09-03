// Shared constants for the real-browser E2E harness (Playwright).
//
// Deliberately distinct from the repository's own dev-server defaults
// (backend :8080, frontend :5173) so this suite never collides with a
// developer's already-running `npm run dev` / `go run ./cmd/server`, and
// never talks to a real, production-shaped backend by accident.
//
// Plain JS (not TypeScript) so it can be imported unchanged from both the
// Node harness scripts (`scripts/*.mjs`) and `playwright.config.ts`.

export const BACKEND_PORT = 48765;
export const FRONTEND_PORT = 45173;
export const BACKEND_BASE_URL = `http://127.0.0.1:${BACKEND_PORT}`;
export const FRONTEND_BASE_URL = `http://127.0.0.1:${FRONTEND_PORT}`;
