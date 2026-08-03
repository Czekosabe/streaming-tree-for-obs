/**
 * Small shared UI types for the platform area.
 *
 * The capability model itself is NOT defined here: provider capabilities,
 * limits and option lists come from the backend and are typed in
 * `src/api/platform-schemas.ts`. The frontend deliberately keeps no second,
 * independently authored capability table - the backend is the single source of
 * truth.
 *
 * What remains here are presentation-level types that have no backend
 * equivalent.
 */

import type { ParseKeys } from 'i18next';

/** An option whose label is ready to render. */
export type SelectOption = {
  value: string;
  label: string;
};

/** Any valid key of the `platforms` namespace. */
export type PlatformTranslationKey = ParseKeys<'platforms'>;

/**
 * Status vocabulary used by the shared status badge.
 *
 * These describe SYSTEM state (is the backend reachable, is a check running),
 * not transmission state. No streaming engine exists, so nothing in the
 * application ever reports a `live` transmission.
 */
export const PLATFORM_STATUSES = ['offline', 'starting', 'live', 'error'] as const;
export type PlatformStatus = (typeof PLATFORM_STATUSES)[number];

/**
 * Status label keys, resolved at render time so the model carries no display
 * language.
 */
export const PLATFORM_STATUS_LABEL_KEYS: Record<PlatformStatus, PlatformTranslationKey> = {
  offline: 'status.offline',
  starting: 'status.starting',
  live: 'status.live',
  error: 'status.error',
};
