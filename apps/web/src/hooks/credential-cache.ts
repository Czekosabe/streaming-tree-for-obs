import type { CredentialStatus } from '@/api/credential-schemas';

/**
 * Pure cache transformation used by the credential-delete mutation.
 *
 * Extracted from the hook so the update rule can be tested directly, without
 * rendering a component or driving a query client - mirrors `platform-cache.ts`.
 */

/**
 * Marks the cached status as "not configured" after a successful delete.
 *
 * Returns `current` unchanged when nothing was cached yet, rather than
 * inventing a `store.available` value this function has no way to know.
 */
export function markStreamKeyDeleted(
  current: CredentialStatus | undefined,
): CredentialStatus | undefined {
  if (current === undefined) return current;
  return { ...current, streamKey: { configured: false } };
}
