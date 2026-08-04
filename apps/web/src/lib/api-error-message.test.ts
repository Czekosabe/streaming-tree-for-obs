import type { TFunction } from 'i18next';
import { describe, expect, it } from 'vitest';

import { ApiError } from './api-client';
import { resolveApiErrorMessage } from './api-error-message';

/**
 * Echoes the key instead of a translation, so the assertions are about which
 * message was chosen rather than about its wording.
 */
const echo = ((key: string, params?: Record<string, unknown>) =>
  params === undefined ? key : `${key}:${JSON.stringify(params)}`) as unknown as TFunction<'errors'>;

describe('mapping stable backend codes to localized messages', () => {
  it.each([
    ['not_found', 'codes.not_found'],
    ['method_not_allowed', 'codes.method_not_allowed'],
    ['internal_error', 'codes.internal_error'],
    ['platform_not_found', 'codes.platform_not_found'],
    ['credential_not_found', 'codes.credential_not_found'],
    ['credential_store_unavailable', 'codes.credential_store_unavailable'],
    ['credential_store_failure', 'codes.credential_store_failure'],
    ['branch_not_running', 'codes.branch_not_running'],
    ['branch_conflict', 'codes.branch_conflict'],
  ])('prefers the localized message for the %s code', (code, expected) => {
    const error = new ApiError('http', 'raw', {
      status: 400,
      code,
      serverMessage: 'English backend text.',
    });

    expect(resolveApiErrorMessage(echo, error)).toBe(expected);
  });

  it('prefers the code mapping over the transport message', () => {
    // A 404 with a known code must read as "this endpoint does not exist",
    // not as a generic HTTP failure.
    const error = new ApiError('not-found', 'raw', { status: 404, code: 'not_found' });

    expect(resolveApiErrorMessage(echo, error)).toBe('codes.not_found');
  });
});

describe('transport failures', () => {
  it('explains an unreachable backend', () => {
    expect(resolveApiErrorMessage(echo, new ApiError('network', 'raw'))).toBe('backend.network');
  });

  it('explains a timeout', () => {
    expect(resolveApiErrorMessage(echo, new ApiError('timeout', 'raw'))).toBe('backend.timeout');
  });

  it('explains a contract mismatch', () => {
    expect(resolveApiErrorMessage(echo, new ApiError('parse', 'raw'))).toBe('backend.parse');
  });

  it('falls back to the English server message for an unmapped HTTP failure', () => {
    // Arbitrary server text is shown verbatim, never machine-translated.
    const error = new ApiError('http', 'raw', {
      status: 418,
      code: 'some_new_code',
      serverMessage: 'The backend explained this itself.',
    });

    expect(resolveApiErrorMessage(echo, error)).toBe('The backend explained this itself.');
  });

  it('reports the status when the backend explained nothing', () => {
    const error = new ApiError('http', 'raw', { status: 502 });

    expect(resolveApiErrorMessage(echo, error)).toBe('backend.httpWithStatus:{"status":502}');
  });

  it('handles a value that is not an ApiError at all', () => {
    expect(resolveApiErrorMessage(echo, new Error('boom'))).toBe('backend.unknown');
    expect(resolveApiErrorMessage(echo, null)).toBe('backend.unknown');
  });
});
