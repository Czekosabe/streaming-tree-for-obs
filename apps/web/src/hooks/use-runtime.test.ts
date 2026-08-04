import { QueryClient } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';

import { MEDIAMTX_STATES, type MediaMtxState } from '@/api/runtime-schemas';

import { pollIntervalFor, runtimeKeys } from './use-runtime';

describe('polling interval selection', () => {
  it('polls fast while something can change at any moment', () => {
    // A publisher can connect or drop without any user action.
    for (const state of ['ready', 'starting', 'stopping'] as const) {
      expect(pollIntervalFor(state)).toBe(1_000);
    }
  });

  it('polls at a moderate rate while installing', () => {
    expect(pollIntervalFor('installing')).toBe(2_000);
  });

  it('polls slowly when nothing changes without a user action', () => {
    for (const state of ['missing', 'incompatible', 'stopped', 'error'] as const) {
      expect(pollIntervalFor(state)).toBe(10_000);
    }
  });

  it('falls back to a moderate interval for an unknown state', () => {
    expect(pollIntervalFor(undefined)).toBe(5_000);
  });

  it.each(MEDIAMTX_STATES)('returns a positive interval for %s', (state: MediaMtxState) => {
    expect(pollIntervalFor(state)).toBeGreaterThan(0);
  });

  it('never polls an idle service as fast as an active one', () => {
    expect(pollIntervalFor('stopped')).toBeGreaterThan(pollIntervalFor('ready'));
  });
});

describe('runtime cache invalidation', () => {
  it('uses a single stable key, so a command cannot invalidate the wrong cache', () => {
    expect(runtimeKeys.runtime).toEqual(['runtime']);
  });

  it('invalidating the runtime key marks the cached snapshot stale', async () => {
    const client = new QueryClient();
    client.setQueryData(runtimeKeys.runtime, { version: 1 });

    expect(client.getQueryState(runtimeKeys.runtime)?.isInvalidated).toBe(false);

    await client.invalidateQueries({ queryKey: runtimeKeys.runtime });

    // This is what every command does on settle, so the panel refetches after
    // install, start, stop and restart alike.
    expect(client.getQueryState(runtimeKeys.runtime)?.isInvalidated).toBe(true);
  });

  it('does not disturb unrelated caches', async () => {
    const client = new QueryClient();
    client.setQueryData(runtimeKeys.runtime, { version: 1 });
    client.setQueryData(['platforms'], []);

    await client.invalidateQueries({ queryKey: runtimeKeys.runtime });

    expect(client.getQueryState(['platforms'])?.isInvalidated).toBe(false);
  });
});
