import { describe, expect, it } from 'vitest';

import { BRANCH_STATES, type BranchSnapshot } from '@/api/branch-schemas';

import {
  blockerKey,
  branchControlsFor,
  branchFor,
  branchStateKey,
  branchTone,
  ffmpegStateKey,
  ffmpegTone,
} from './branch-presentation';

describe('branchStateKey', () => {
  it.each(BRANCH_STATES)('returns a key for every documented state: %s', (state) => {
    expect(branchStateKey(state)).toMatch(/^branch\.state\./);
  });
});

describe('branchTone', () => {
  it('reports live only for the live state', () => {
    for (const state of BRANCH_STATES) {
      expect(branchTone(state) === 'live').toBe(state === 'live');
    }
  });

  it('reports error only for the error state', () => {
    for (const state of BRANCH_STATES) {
      expect(branchTone(state) === 'error').toBe(state === 'error');
    }
  });

  it.each(BRANCH_STATES)('returns a valid tone for every state: %s', (state) => {
    expect(['live', 'starting', 'error', 'offline']).toContain(branchTone(state));
  });
});

describe('branchControlsFor', () => {
  it.each(BRANCH_STATES)('Start and Stop are never both enabled: %s', (state) => {
    const controls = branchControlsFor(state);
    expect(controls.canStart && controls.canStop).toBe(false);
  });

  it('allows starting an idle branch', () => {
    expect(branchControlsFor('idle').canStart).toBe(true);
  });

  it('allows starting a blocked branch (to re-check eligibility)', () => {
    expect(branchControlsFor('blocked').canStart).toBe(true);
  });

  it('allows starting an errored branch (to retry)', () => {
    expect(branchControlsFor('error').canStart).toBe(true);
  });

  it('allows stopping and restarting a live branch, never starting it', () => {
    const controls = branchControlsFor('live');
    expect(controls.canStart).toBe(false);
    expect(controls.canStop).toBe(true);
    expect(controls.canRestart).toBe(true);
  });

  it('allows stopping a waiting-for-ingest branch', () => {
    expect(branchControlsFor('waiting_for_ingest').canStop).toBe(true);
  });

  it('disables every control while stopping', () => {
    const controls = branchControlsFor('stopping');
    expect(controls.canStart).toBe(false);
    expect(controls.canStop).toBe(false);
    expect(controls.canRestart).toBe(false);
  });
});

describe('blockerKey', () => {
  it('maps every backend blocker identifier to a translation key', () => {
    const blockers = [
      'platform_disabled',
      'output_server_missing',
      'stream_key_missing',
      'credential_store_unavailable',
      'ffmpeg_missing',
      'ffmpeg_incompatible',
      'mediamtx_not_ready',
      'ingest_not_receiving',
    ];
    for (const blocker of blockers) {
      expect(blockerKey(blocker)).toMatch(/^branch\.blockers\./);
    }
  });

  it('returns null for an identifier this build does not know', () => {
    // The caller falls back to the raw identifier; a user must still see
    // something, never a crash.
    expect(blockerKey('invented_later')).toBeNull();
  });
});

describe('ffmpegStateKey and ffmpegTone', () => {
  it.each(['missing', 'ready', 'incompatible', 'error'] as const)(
    'returns a key and a valid tone for %s',
    (state) => {
      expect(ffmpegStateKey(state)).toMatch(/^ffmpeg\.state\./);
      expect(['live', 'starting', 'error', 'offline']).toContain(ffmpegTone(state));
    },
  );

  it('reports ready as the only positive tone', () => {
    expect(ffmpegTone('ready')).toBe('live');
    expect(ffmpegTone('missing')).not.toBe('live');
    expect(ffmpegTone('incompatible')).not.toBe('live');
    expect(ffmpegTone('error')).not.toBe('live');
  });
});

describe('branchFor', () => {
  const snapshot = (platformId: string): BranchSnapshot => ({
    platformId,
    state: 'idle',
    desiredRunning: false,
    blockers: [],
    restartCount: 0,
    progress: null,
    lastError: null,
  });

  it('finds the matching branch by platform id', () => {
    const branches = [snapshot('pf_1'), snapshot('pf_2')];
    expect(branchFor(branches, 'pf_2')?.platformId).toBe('pf_2');
  });

  it('returns undefined when no branch matches', () => {
    expect(branchFor([snapshot('pf_1')], 'pf_missing')).toBeUndefined();
  });

  it('returns undefined for an undefined list', () => {
    expect(branchFor(undefined, 'pf_1')).toBeUndefined();
  });
});
