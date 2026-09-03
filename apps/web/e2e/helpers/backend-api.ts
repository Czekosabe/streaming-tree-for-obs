/**
 * Thin direct-fetch helpers against the hermetic test backend
 * (`e2e/scripts/run-backend.mjs`), used by specs to put the backend into a
 * known, deterministic state before a browser interaction - never to
 * replace a browser assertion, only to set up or reset fixtures the same
 * way `scripts/verify-*.mjs` already does via raw `fetch`.
 */
import { BACKEND_BASE_URL } from '../env.mjs';

export type OnboardingStatus = 'pending' | 'completed' | 'dismissed';

export async function setOnboardingStatus(status: OnboardingStatus): Promise<void> {
  const response = await fetch(`${BACKEND_BASE_URL}/api/onboarding`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  });
  if (!response.ok) {
    throw new Error(`setOnboardingStatus(${status}) failed: HTTP ${response.status}`);
  }
}

export async function getOnboardingStatus(): Promise<OnboardingStatus> {
  const response = await fetch(`${BACKEND_BASE_URL}/api/onboarding`);
  if (!response.ok) {
    throw new Error(`getOnboardingStatus failed: HTTP ${response.status}`);
  }
  const body = (await response.json()) as { status: OnboardingStatus };
  return body.status;
}

export type SeededPlatform = {
  id: string;
  providerId: string;
  displayName: string;
  enabled: boolean;
};

/**
 * The four platforms `0002_seed_default_platforms.sql` inserts into every
 * fresh database, once, on first migration - stable IDs/names this suite
 * relies on as deterministic fixture data rather than creating its own.
 */
export async function listPlatforms(): Promise<SeededPlatform[]> {
  const response = await fetch(`${BACKEND_BASE_URL}/api/platforms`);
  if (!response.ok) {
    throw new Error(`listPlatforms failed: HTTP ${response.status}`);
  }
  const body = (await response.json()) as { platforms: SeededPlatform[] };
  return body.platforms;
}
