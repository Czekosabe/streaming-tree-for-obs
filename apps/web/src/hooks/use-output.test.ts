import { QueryClient } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';

import { outputKeys } from './use-output';

describe('output settings query key', () => {
  it('is stable for the same platform id', () => {
    expect(outputKeys.settings('pf_a')).toEqual(outputKeys.settings('pf_a'));
  });

  it('is scoped per platform', () => {
    expect(outputKeys.settings('pf_a')).not.toEqual(outputKeys.settings('pf_b'));
  });

  it('invalidating one platform does not disturb another\'s cache', async () => {
    const client = new QueryClient();
    client.setQueryData(outputKeys.settings('pf_a'), { serverUrl: '', autoRestart: true, updatedAt: '' });
    client.setQueryData(outputKeys.settings('pf_b'), { serverUrl: '', autoRestart: true, updatedAt: '' });

    await client.invalidateQueries({ queryKey: outputKeys.settings('pf_a') });

    expect(client.getQueryState(outputKeys.settings('pf_a'))?.isInvalidated).toBe(true);
    expect(client.getQueryState(outputKeys.settings('pf_b'))?.isInvalidated).toBe(false);
  });
});
