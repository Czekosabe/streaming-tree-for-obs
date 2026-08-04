import { describe, expect, it } from 'vitest';

import type { CredentialStatus } from '@/api/credential-schemas';

import { markStreamKeyDeleted } from './credential-cache';

describe('markStreamKeyDeleted', () => {
  it('marks a configured status as not configured', () => {
    const before: CredentialStatus = {
      streamKey: { configured: true },
      store: { available: true },
    };

    const after = markStreamKeyDeleted(before);

    expect(after).toEqual({ streamKey: { configured: false }, store: { available: true } });
  });

  it('leaves store availability untouched', () => {
    const before: CredentialStatus = {
      streamKey: { configured: true },
      store: { available: false },
    };

    expect(markStreamKeyDeleted(before)?.store.available).toBe(false);
  });

  it('passes an empty cache through rather than inventing a status', () => {
    expect(markStreamKeyDeleted(undefined)).toBeUndefined();
  });

  it('does not mutate the input', () => {
    const before: CredentialStatus = {
      streamKey: { configured: true },
      store: { available: true },
    };

    markStreamKeyDeleted(before);

    expect(before.streamKey.configured).toBe(true);
  });
});
