import type { TFunction } from 'i18next';
import { describe, expect, it } from 'vitest';

import { runtimeErrorMessage } from './runtime-error-message';

/** Echoes the key so assertions are about which message was chosen. */
const echo = ((key: string) => key) as unknown as TFunction<'runtime'>;

describe('runtime error localization', () => {
  it('returns null when there is no error', () => {
    expect(runtimeErrorMessage(echo, null)).toBeNull();
    expect(runtimeErrorMessage(echo, undefined)).toBeNull();
  });

  it('localizes a known code', () => {
    const message = runtimeErrorMessage(echo, {
      code: 'mediamtx_checksum_mismatch',
      message: 'English fallback from the backend.',
    });

    expect(message).toBe('errors.checksumMismatch');
  });

  it.each([
    ['mediamtx_not_installed', 'errors.notInstalled'],
    ['mediamtx_port_in_use', 'errors.portInUse'],
    ['mediamtx_unsupported_platform', 'errors.unsupportedPlatform'],
    ['mediamtx_restart_limit_reached', 'errors.restartLimit'],
  ])('localizes %s', (code, expected) => {
    expect(runtimeErrorMessage(echo, { code, message: 'ignored' })).toBe(expected);
  });

  it('falls back to the English backend message for an unknown code', () => {
    // A newer backend rule must still produce a sentence, never an identifier.
    const message = runtimeErrorMessage(echo, {
      code: 'mediamtx_something_added_later',
      message: 'The backend explained this itself.',
    });

    expect(message).toBe('The backend explained this itself.');
  });

  it('never returns a bare error code', () => {
    const message = runtimeErrorMessage(echo, {
      code: 'mediamtx_unmapped',
      message: 'Readable sentence.',
    });

    expect(message).not.toBe('mediamtx_unmapped');
  });
});
