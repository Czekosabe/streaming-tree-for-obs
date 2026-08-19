import { z } from 'zod';

import { apiGet, apiPost, ApiError } from '@/lib/api-client';

import { sessionBootstrapSchema, type SessionBootstrap } from './auth-schemas';

const logoutResponseSchema = z.object({ status: z.string() });

/**
 * Stage 20D2B remote-management authentication API
 * (docs/remote-management.md §18, internal/httpapi/auth_routes.go).
 *
 * These routes exist at all only when the backend was started with
 * --remote-management - a plain desktop or --headless-only backend
 * answers every one of them with 404, which callers here surface as
 * `null` rather than an error, so the rest of the frontend can treat
 * "remote management is not active" as a normal, expected outcome.
 */

/** GET /api/auth/session - null means remote management is not active on this backend. */
export async function fetchSessionStatus(): Promise<SessionBootstrap | null> {
  try {
    return await apiGet('/api/auth/session', sessionBootstrapSchema);
  } catch (error) {
    if (error instanceof ApiError && error.kind === 'not-found') {
      return null;
    }
    throw error;
  }
}

/**
 * POST /api/auth/login. Throws ApiError on failure - callers
 * distinguish `status === 401` (wrong password) from `status === 429`
 * (rate limited) to show the right message; every other failure falls
 * back to a generic connection-error state.
 */
export async function login(password: string): Promise<SessionBootstrap> {
  return apiPost('/api/auth/login', { password }, sessionBootstrapSchema);
}

/**
 * POST /api/auth/logout - requires the caller to have already
 * attached the CSRF header (see lib/api-client.ts's setCSRFToken).
 */
export async function logout(): Promise<void> {
  await apiPost('/api/auth/logout', {}, logoutResponseSchema);
}
