import type { ParseKeys, TFunction } from 'i18next';

import { ApiError } from './api-client';

/**
 * Localized messages for the backend's stable error codes.
 *
 * The codes themselves are API identifiers and stay in English; only the text
 * shown to the operator is translated. Adding a backend code without a mapping
 * here is safe - the resolver falls through to the English server message.
 */
const CODE_MESSAGE_KEYS = new Map<string, ParseKeys<'errors'>>([
  ['not_found', 'codes.not_found'],
  ['method_not_allowed', 'codes.method_not_allowed'],
  ['internal_error', 'codes.internal_error'],
  ['platform_not_found', 'codes.platform_not_found'],
  ['credential_not_found', 'codes.credential_not_found'],
  ['credential_store_unavailable', 'codes.credential_store_unavailable'],
  ['credential_store_failure', 'codes.credential_store_failure'],
  ['branch_not_running', 'codes.branch_not_running'],
  ['branch_conflict', 'codes.branch_conflict'],
]);

/**
 * Turns a failed request into one sentence the operator can act on.
 *
 * Resolution order:
 *  1. a localized message mapped from the backend's stable error code,
 *  2. the backend's own English message for HTTP failures it explained itself,
 *  3. a localized message for the transport-level failure kind.
 *
 * Arbitrary server text is never machine-translated - step 2 shows it verbatim.
 */
export function resolveApiErrorMessage(t: TFunction<'errors'>, error: unknown): string {
  if (!(error instanceof ApiError)) {
    return t('backend.unknown');
  }

  if (error.code !== null) {
    const mapped = CODE_MESSAGE_KEYS.get(error.code);
    if (mapped !== undefined) return t(mapped);
  }

  switch (error.kind) {
    case 'network':
      return t('backend.network');
    case 'timeout':
      return t('backend.timeout');
    case 'parse':
      return t('backend.parse');
    case 'http':
      if (error.serverMessage !== null) return error.serverMessage;
      return error.status === null
        ? t('backend.http')
        : t('backend.httpWithStatus', { status: error.status });
    default:
      return t('backend.unknown');
  }
}
