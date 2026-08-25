import type { ParseKeys } from 'i18next';

/**
 * Translation-key mapping for the updater's own stable, machine-readable
 * codes (docs/updater.md §17/§30/§33) - mirrors `branch-presentation.ts`'s
 * `blockerKey` convention exactly: a closed lookup table, falling back to a
 * generic key for anything this build does not recognise, so an unknown
 * future backend code never crashes the UI or renders a raw identifier.
 */

export type UpdatesKey = ParseKeys<'updates'>;

const BLOCKER_KEYS: Record<string, UpdatesKey> = {
  install_blocked_streaming_active: 'blocked.install_blocked_streaming_active',
  not_installed_context: 'blocked.not_installed_context',
  no_verified_candidate: 'blocked.no_verified_candidate',
  platform_unsupported: 'blocked.platform_unsupported',
  manual_build: 'blocked.manual_build',
};

/** Translation key for one install-blocker code, or the generic fallback. */
export function updateBlockerKey(code: string | undefined): UpdatesKey {
  if (code === undefined) return 'blocked.generic';
  return Object.prototype.hasOwnProperty.call(BLOCKER_KEYS, code)
    ? (BLOCKER_KEYS[code] ?? 'blocked.generic')
    : 'blocked.generic';
}

const ERROR_KEYS: Record<string, UpdatesKey> = {
  check_failed: 'error.check_failed',
  rate_limited: 'error.rate_limited',
  invalid_manifest: 'error.invalid_manifest',
  download_failed: 'error.download_failed',
  hash_mismatch: 'error.hash_mismatch',
  size_exceeded: 'error.size_exceeded',
  install_failed: 'error.install_failed',
};

/** Translation key for one updater error code, or the generic fallback. */
export function updateErrorKey(code: string | undefined): UpdatesKey {
  if (code === undefined) return 'error.generic';
  return Object.prototype.hasOwnProperty.call(ERROR_KEYS, code)
    ? (ERROR_KEYS[code] ?? 'error.generic')
    : 'error.generic';
}
