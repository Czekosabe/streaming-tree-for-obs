import { QueryClient } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';

import { accountKeys } from './use-accounts';

describe('account query keys', () => {
  it('are stable and scoped per identifier', () => {
    expect(accountKeys.account('acct_a')).toEqual(accountKeys.account('acct_a'));
    expect(accountKeys.account('acct_a')).not.toEqual(accountKeys.account('acct_b'));
    expect(accountKeys.deviceFlow('devflow_a')).not.toEqual(accountKeys.deviceFlow('devflow_b'));
    expect(accountKeys.platformLink('pf_a')).not.toEqual(accountKeys.platformLink('pf_b'));
  });

  it('scopes category search by both account id and query text', () => {
    const a = accountKeys.categorySearch('acct_1', 'just chatting');
    const b = accountKeys.categorySearch('acct_1', 'valorant');
    const c = accountKeys.categorySearch('acct_2', 'just chatting');
    expect(a).not.toEqual(b);
    expect(a).not.toEqual(c);
  });

  it('never carries a secret-shaped value - only ids and search text', () => {
    const key = accountKeys.categorySearch('acct_1', 'query');
    const serialized = JSON.stringify(key);
    expect(serialized).not.toMatch(/token|secret|bearer/i);
  });

  it('invalidating the accounts list does not disturb one platform link cache', async () => {
    const client = new QueryClient();
    client.setQueryData(accountKeys.accounts, []);
    client.setQueryData(accountKeys.platformLink('pf_1'), null);

    await client.invalidateQueries({ queryKey: accountKeys.accounts });

    expect(client.getQueryState(accountKeys.accounts)?.isInvalidated).toBe(true);
    expect(client.getQueryState(accountKeys.platformLink('pf_1'))?.isInvalidated).toBe(false);
  });
});
