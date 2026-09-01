import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as branchesApi from '@/api/branches';
import * as runtimeApi from '@/api/runtime';
import type * as ApiClientModule from '@/lib/api-client';
import { renderWithProviders } from '@/test/render';

import { ReadinessStep } from './ReadinessStep';

vi.mock('@/api/runtime');
vi.mock('@/api/branches');
vi.mock('@/lib/api-client', async (importOriginal) => {
  const actual = await importOriginal<typeof ApiClientModule>();
  return {
    ...actual,
    apiGet: vi.fn().mockResolvedValue({ status: 'ok', service: 'streaming-tree', version: '0.1.0' }),
  };
});

const BASE_SNAPSHOT = {
  version: 1,
  ingest: { state: 'unavailable' as const, path: 'live', trackCount: null, tracks: [] },
  connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
};

const FFMPEG_MISSING = {
  version: 1,
  ffmpeg: {
    state: 'missing' as const, source: 'missing' as const, minimumVersion: '4.4',
    capabilities: { rtmpInput: false, rtmpOutput: false, rtmpsOutput: false, flvMuxer: false, progress: false },
    lastError: null,
  },
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(branchesApi).fetchFFmpegStatus.mockResolvedValue(FFMPEG_MISSING);
});

describe('ReadinessStep', () => {
  it('offers an Install action when MediaMTX is missing', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      ...BASE_SNAPSHOT,
      mediaMtx: {
        supportedVersion: '1.19.3', source: 'missing' as const, state: 'missing' as const,
        autoStart: true, autoRestart: true, restartCount: 0, lastError: null,
      },
    });

    renderWithProviders(<ReadinessStep />);

    expect(await screen.findByRole('button', { name: /install/i })).toBeInTheDocument();
  });

  it('offers Stop/Restart, not Install, once MediaMTX is ready', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      ...BASE_SNAPSHOT,
      mediaMtx: {
        supportedVersion: '1.19.3', source: 'managed' as const, state: 'ready' as const,
        autoStart: true, autoRestart: true, restartCount: 0, lastError: null,
      },
    });

    renderWithProviders(<ReadinessStep />);

    expect(await screen.findByRole('button', { name: /stop/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^install$/i })).not.toBeInTheDocument();
  });
});
