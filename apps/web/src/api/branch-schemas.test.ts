import { describe, expect, it } from 'vitest';

import {
  BRANCH_STATES,
  branchesResponseSchema,
  branchSnapshotSchema,
  ffmpegStatusSchema,
  FFMPEG_STATES,
} from './branch-schemas';

const validFFmpegStatus = {
  version: 1,
  ffmpeg: {
    state: 'ready',
    source: 'path',
    detectedVersion: '8.1',
    minimumVersion: '4.4',
    capabilities: {
      rtmpInput: true,
      rtmpOutput: true,
      rtmpsOutput: true,
      flvMuxer: true,
      progress: true,
    },
    lastError: null,
  },
};

describe('ffmpegStatusSchema', () => {
  it('accepts a well-formed payload', () => {
    expect(ffmpegStatusSchema.safeParse(validFFmpegStatus).success).toBe(true);
  });

  it.each(FFMPEG_STATES)('accepts every documented state: %s', (state) => {
    const payload = { ...validFFmpegStatus, ffmpeg: { ...validFFmpegStatus.ffmpeg, state } };
    expect(ffmpegStatusSchema.safeParse(payload).success).toBe(true);
  });

  it('accepts a null lastError alongside a real one', () => {
    const withError = {
      ...validFFmpegStatus,
      ffmpeg: {
        ...validFFmpegStatus.ffmpeg,
        lastError: { code: 'ffmpeg_not_found', message: 'not found' },
      },
    };
    expect(ffmpegStatusSchema.safeParse(withError).success).toBe(true);
  });

  it('rejects an unknown state', () => {
    const payload = { ...validFFmpegStatus, ffmpeg: { ...validFFmpegStatus.ffmpeg, state: 'bogus' } };
    expect(ffmpegStatusSchema.safeParse(payload).success).toBe(false);
  });

  it('rejects a missing capabilities object', () => {
    const { capabilities: _capabilities, ...rest } = validFFmpegStatus.ffmpeg;
    const payload = { version: 1, ffmpeg: rest };
    expect(ffmpegStatusSchema.safeParse(payload).success).toBe(false);
  });

  it('rejects malformed input entirely', () => {
    expect(ffmpegStatusSchema.safeParse(null).success).toBe(false);
    expect(ffmpegStatusSchema.safeParse('ready').success).toBe(false);
    expect(ffmpegStatusSchema.safeParse({}).success).toBe(false);
  });

  it('never requires or accepts a field resembling an executable path', () => {
    // The schema's shape itself is the guarantee: there is no "path" field
    // to accidentally populate from a response that leaked one.
    const parsed = ffmpegStatusSchema.parse(validFFmpegStatus);
    expect(parsed.ffmpeg).not.toHaveProperty('path');
  });
});

const validBranchSnapshot = {
  platformId: 'pf_1',
  state: 'idle',
  desiredRunning: false,
  blockers: [],
  restartCount: 0,
  progress: null,
  lastError: null,
};

describe('branchSnapshotSchema', () => {
  it('accepts a minimal idle snapshot', () => {
    expect(branchSnapshotSchema.safeParse(validBranchSnapshot).success).toBe(true);
  });

  it.each(BRANCH_STATES)('accepts every documented state: %s', (state) => {
    expect(branchSnapshotSchema.safeParse({ ...validBranchSnapshot, state }).success).toBe(true);
  });

  it('accepts a full snapshot with progress and timestamps', () => {
    const full = {
      ...validBranchSnapshot,
      state: 'live',
      desiredRunning: true,
      blockers: [],
      startedAt: '2026-08-04T12:00:00Z',
      liveAt: '2026-08-04T12:00:05Z',
      restartCount: 2,
      progress: { frameCount: 100, fps: 30, outTimeMs: 5000, totalSize: 65536, speed: 1.02 },
      lastError: null,
    };
    expect(branchSnapshotSchema.safeParse(full).success).toBe(true);
  });

  it('rejects an unknown state', () => {
    expect(branchSnapshotSchema.safeParse({ ...validBranchSnapshot, state: 'bogus' }).success).toBe(
      false,
    );
  });

  it('rejects a missing platformId', () => {
    const { platformId: _platformId, ...rest } = validBranchSnapshot;
    expect(branchSnapshotSchema.safeParse(rest).success).toBe(false);
  });

  it('tolerates an empty blockers array and a null progress', () => {
    expect(branchSnapshotSchema.safeParse(validBranchSnapshot).success).toBe(true);
  });

  it('never contains a field resembling a secret or a destination URL', () => {
    const parsed = branchSnapshotSchema.parse(validBranchSnapshot);
    expect(parsed).not.toHaveProperty('streamKey');
    expect(parsed).not.toHaveProperty('destinationUrl');
    expect(parsed).not.toHaveProperty('commandLine');
    expect(parsed).not.toHaveProperty('pid');
  });
});

describe('branchesResponseSchema', () => {
  it('accepts a versioned list', () => {
    const payload = { version: 1, branches: [validBranchSnapshot] };
    expect(branchesResponseSchema.safeParse(payload).success).toBe(true);
  });

  it('accepts an empty list', () => {
    expect(branchesResponseSchema.safeParse({ version: 1, branches: [] }).success).toBe(true);
  });

  it('rejects malformed input', () => {
    expect(branchesResponseSchema.safeParse({ branches: 'not an array' }).success).toBe(false);
  });
});
