import type { ParseKeys } from 'i18next';

import type { BranchSnapshot, BranchState } from '@/api/branch-schemas';
import type { PlatformStatus } from './platform';

/**
 * Maps branch state onto presentation: label, tone, and which controls are
 * usable. Pure and exhaustive, so the rules can be tested without rendering
 * and a new state cannot be forgotten - mirrors runtime-presentation.ts.
 */

export type BranchKey = ParseKeys<'runtime'>;

export function branchStateKey(state: BranchState): BranchKey {
  const keys: Record<BranchState, BranchKey> = {
    idle: 'branch.state.idle',
    blocked: 'branch.state.blocked',
    waiting_for_ingest: 'branch.state.waitingForIngest',
    starting: 'branch.state.starting',
    live: 'branch.state.live',
    restarting: 'branch.state.restarting',
    stopping: 'branch.state.stopping',
    error: 'branch.state.error',
  };
  return keys[state];
}

export function branchTone(state: BranchState): PlatformStatus {
  switch (state) {
    case 'live':
      return 'live';
    case 'starting':
    case 'restarting':
    case 'waiting_for_ingest':
      return 'starting';
    case 'error':
      return 'error';
    case 'idle':
    case 'blocked':
    case 'stopping':
      return 'offline';
  }
}

/** Translation key for one blocker identifier, or null for one this build
 * does not recognise - the caller falls back to the identifier itself. */
export function blockerKey(blocker: string): BranchKey | null {
  const keys: Record<string, BranchKey> = {
    platform_disabled: 'branch.blockers.platformDisabled',
    output_server_missing: 'branch.blockers.outputServerMissing',
    stream_key_missing: 'branch.blockers.streamKeyMissing',
    credential_store_unavailable: 'branch.blockers.credentialStoreUnavailable',
    ffmpeg_missing: 'branch.blockers.ffmpegMissing',
    ffmpeg_incompatible: 'branch.blockers.ffmpegIncompatible',
    mediamtx_not_ready: 'branch.blockers.mediamtxNotReady',
    ingest_not_receiving: 'branch.blockers.ingestNotReceiving',
  };
  return Object.prototype.hasOwnProperty.call(keys, blocker) ? (keys[blocker] ?? null) : null;
}

/** Which branch controls the current state allows.
 *
 * Start and Stop are never both enabled: a branch is either something you
 * could tell to run, or something you could tell to stop, never both at once.
 */
export type BranchControls = {
  canStart: boolean;
  canStop: boolean;
  canRestart: boolean;
};

export function branchControlsFor(state: BranchState): BranchControls {
  switch (state) {
    case 'idle':
    case 'blocked':
    case 'error':
      return { canStart: true, canStop: false, canRestart: false };
    case 'starting':
    case 'live':
    case 'restarting':
    case 'waiting_for_ingest':
      return { canStart: false, canStop: true, canRestart: true };
    case 'stopping':
      return { canStart: false, canStop: false, canRestart: false };
  }
}

/** Translation key for the FFmpeg dependency state. */
export function ffmpegStateKey(
  state: 'missing' | 'ready' | 'incompatible' | 'error',
): BranchKey {
  const keys: Record<typeof state, BranchKey> = {
    missing: 'ffmpeg.state.missing',
    ready: 'ffmpeg.state.ready',
    incompatible: 'ffmpeg.state.incompatible',
    error: 'ffmpeg.state.error',
  };
  return keys[state];
}

export function ffmpegTone(state: 'missing' | 'ready' | 'incompatible' | 'error'): PlatformStatus {
  switch (state) {
    case 'ready':
      return 'live';
    case 'missing':
      return 'offline';
    case 'incompatible':
    case 'error':
      return 'error';
  }
}

/** Finds a branch snapshot for one platform, or undefined if none is
 * tracked yet (a platform never started reports as idle by convention on
 * the backend, so this should only be undefined before the first fetch). */
export function branchFor(
  branches: BranchSnapshot[] | undefined,
  platformId: string,
): BranchSnapshot | undefined {
  return branches?.find((b) => b.platformId === platformId);
}
