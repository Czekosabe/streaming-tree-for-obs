import { QueryClient } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';

import type { BranchSnapshot } from '@/api/branch-schemas';

import { branchKeys, branchPollIntervalFor } from './use-branches';

function snapshot(overrides: Partial<BranchSnapshot> = {}): BranchSnapshot {
  return {
    platformId: 'pf_1',
    state: 'idle',
    desiredRunning: false,
    blockers: [],
    restartCount: 0,
    progress: null,
    lastError: null,
    ...overrides,
  };
}

describe('branchPollIntervalFor', () => {
  it('polls slowly before any data has loaded', () => {
    expect(branchPollIntervalFor(undefined)).toBe(5_000);
  });

  it('polls slowly when every branch is idle', () => {
    expect(branchPollIntervalFor([snapshot({ state: 'idle' })])).toBe(10_000);
  });

  it('polls slowly when every branch is blocked', () => {
    expect(branchPollIntervalFor([snapshot({ state: 'blocked' })])).toBe(10_000);
  });

  it.each(['starting', 'live', 'restarting', 'stopping', 'waiting_for_ingest'] as const)(
    'polls quickly while any branch is %s',
    (state) => {
      expect(branchPollIntervalFor([snapshot({ state })])).toBe(1_000);
    },
  );

  it('polls quickly if any one branch among several is active', () => {
    const branches = [snapshot({ platformId: 'pf_1', state: 'idle' }), snapshot({ platformId: 'pf_2', state: 'live' })];
    expect(branchPollIntervalFor(branches)).toBe(1_000);
  });

  it('polls quickly for a desired-running branch even if currently idle-looking', () => {
    // waiting_for_ingest is itself in the active list, but this also covers
    // the desiredRunning flag directly for a state not otherwise listed.
    const branches = [snapshot({ state: 'blocked', desiredRunning: true })];
    expect(branchPollIntervalFor(branches)).toBe(1_000);
  });

  it('polls slowly for an empty list', () => {
    expect(branchPollIntervalFor([])).toBe(10_000);
  });
});

describe('branch and ffmpeg query keys', () => {
  it('are stable and distinct', () => {
    expect(branchKeys.branches).toEqual(['branches']);
    expect(branchKeys.ffmpeg).toEqual(['ffmpeg-status']);
    expect(branchKeys.branches).not.toEqual(branchKeys.ffmpeg);
  });

  it('invalidating the branches key marks the cached list stale', async () => {
    const client = new QueryClient();
    client.setQueryData(branchKeys.branches, [snapshot()]);

    expect(client.getQueryState(branchKeys.branches)?.isInvalidated).toBe(false);
    await client.invalidateQueries({ queryKey: branchKeys.branches });
    expect(client.getQueryState(branchKeys.branches)?.isInvalidated).toBe(true);
  });

  it('invalidating the branches key does not disturb the ffmpeg status cache', async () => {
    const client = new QueryClient();
    client.setQueryData(branchKeys.ffmpeg, { version: 1 });

    await client.invalidateQueries({ queryKey: branchKeys.branches });

    expect(client.getQueryState(branchKeys.ffmpeg)?.isInvalidated).toBe(false);
  });
});
