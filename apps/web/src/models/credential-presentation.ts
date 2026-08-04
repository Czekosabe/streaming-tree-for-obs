import type { ParseKeys } from 'i18next';

import type { CredentialStatus } from '@/api/credential-schemas';

/**
 * Maps credential status onto presentation: which label to show and how
 * urgent it reads.
 *
 * Pure and exhaustive, so the rules can be tested without rendering - mirrors
 * `runtime-presentation.ts`. Wording is deliberately limited to "Stored" /
 * "Missing" / a store-unavailable message: never "Valid", "Connected" or
 * "Authenticated", since a stored key has not been verified against the
 * platform.
 */

export type CredentialPresentationKey = ParseKeys<'platforms'>;

export type CredentialTone = 'neutral' | 'positive' | 'warning';

export type CredentialPresentation = {
  labelKey: CredentialPresentationKey;
  tone: CredentialTone;
};

export function presentCredentialStatus(
  status: CredentialStatus | undefined,
  loading: boolean,
): CredentialPresentation {
  if (loading) {
    return { labelKey: 'credentials.checking', tone: 'neutral' };
  }
  if (status === undefined) {
    return { labelKey: 'credentials.unknown', tone: 'warning' };
  }
  if (!status.store.available) {
    return { labelKey: 'credentials.storeUnavailable', tone: 'warning' };
  }
  if (status.streamKey.configured) {
    return { labelKey: 'credentials.stored', tone: 'positive' };
  }
  return { labelKey: 'credentials.missing', tone: 'neutral' };
}
