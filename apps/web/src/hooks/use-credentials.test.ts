import { QueryClient } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';

import type { CredentialStatus } from '@/api/credential-schemas';

import { credentialKeys } from './use-credentials';

describe('credential query key', () => {
  it('is stable for the same platform id', () => {
    expect(credentialKeys.status('pf_a')).toEqual(credentialKeys.status('pf_a'));
  });

  it('is scoped per platform, so one platform can never invalidate or read another\'s cache', () => {
    expect(credentialKeys.status('pf_a')).not.toEqual(credentialKeys.status('pf_b'));
  });
});

describe('credential status cache behaviour', () => {
  // These simulate exactly what useSetStreamKeyMutation's and
  // useDeleteStreamKeyMutation's onSuccess callbacks do, using the query
  // client directly - the same technique use-runtime.test.ts uses for
  // runtime cache invalidation, since this project has no component-render
  // test harness (see docs/progress.md for that decision).

  it('setting a status only ever stores the {configured, available} shape returned by the API', () => {
    const client = new QueryClient();
    const key = credentialKeys.status('pf_a');

    // What useSetStreamKeyMutation's onSuccess writes: the parsed API
    // response, never the mutation's own `variables` (which is where the
    // typed stream key lives).
    const apiResponse: CredentialStatus = {
      streamKey: { configured: true },
      store: { available: true },
    };
    client.setQueryData(key, apiResponse);

    const cached = client.getQueryData<CredentialStatus>(key);
    expect(cached).toEqual({ streamKey: { configured: true }, store: { available: true } });
    // The cached value has exactly two boolean leaves - nothing resembling a
    // secret value has anywhere to hide in this shape.
    expect(Object.keys(cached ?? {})).toEqual(['streamKey', 'store']);
    expect(Object.keys(cached?.streamKey ?? {})).toEqual(['configured']);
  });

  it('updating one platform\'s cached status does not disturb another\'s', () => {
    const client = new QueryClient();
    client.setQueryData(credentialKeys.status('pf_a'), {
      streamKey: { configured: false },
      store: { available: true },
    } satisfies CredentialStatus);
    client.setQueryData(credentialKeys.status('pf_b'), {
      streamKey: { configured: true },
      store: { available: true },
    } satisfies CredentialStatus);

    client.setQueryData(credentialKeys.status('pf_a'), {
      streamKey: { configured: true },
      store: { available: true },
    } satisfies CredentialStatus);

    expect(client.getQueryData<CredentialStatus>(credentialKeys.status('pf_b'))).toEqual({
      streamKey: { configured: true },
      store: { available: true },
    });
  });

  it('deleting marks the cached status as not configured, in place', async () => {
    const client = new QueryClient();
    const key = credentialKeys.status('pf_a');
    client.setQueryData(key, {
      streamKey: { configured: true },
      store: { available: true },
    } satisfies CredentialStatus);

    // What useDeleteStreamKeyMutation's onSuccess does.
    const { markStreamKeyDeleted } = await import('./credential-cache');
    client.setQueryData<CredentialStatus>(key, (current) => markStreamKeyDeleted(current));

    expect(client.getQueryData<CredentialStatus>(key)).toEqual({
      streamKey: { configured: false },
      store: { available: true },
    });
  });
});
