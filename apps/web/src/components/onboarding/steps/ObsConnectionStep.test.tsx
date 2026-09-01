import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as runtimeApi from '@/api/runtime';
import { renderWithProviders } from '@/test/render';

import { ObsConnectionStep } from './ObsConnectionStep';

vi.mock('@/api/runtime');

const MEDIAMTX_READY = {
  supportedVersion: '1.19.3', source: 'managed' as const, state: 'ready' as const,
  autoStart: true, autoRestart: true, restartCount: 0, lastError: null,
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ObsConnectionStep', () => {
  it('shows the real server and stream key values, and states they are not a secret', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: MEDIAMTX_READY,
      ingest: { state: 'waiting' as const, path: 'live', trackCount: null, tracks: [] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<ObsConnectionStep />);

    expect(await screen.findByText('rtmp://127.0.0.1:1935')).toBeInTheDocument();
    expect(screen.getByText('live')).toBeInTheDocument();
    expect(screen.getByText(/not a secret/i)).toBeInTheDocument();
  });

  it('shows "waiting for OBS" style copy when nothing is publishing yet', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: MEDIAMTX_READY,
      ingest: { state: 'waiting' as const, path: 'live', trackCount: null, tracks: [] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<ObsConnectionStep />);

    expect(await screen.findByText(/waiting for obs/i)).toBeInTheDocument();
  });

  it('shows the real connected state once a publisher is receiving', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: MEDIAMTX_READY,
      ingest: { state: 'receiving' as const, path: 'live', sourceType: 'rtmpConn', trackCount: 1, tracks: ['H264'] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<ObsConnectionStep />);

    expect(await screen.findByText(/receiving/i)).toBeInTheDocument();
  });

  it('never uses obs-websocket or plugin-installation wording', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: MEDIAMTX_READY,
      ingest: { state: 'waiting' as const, path: 'live', trackCount: null, tracks: [] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<ObsConnectionStep />);
    await screen.findByText('rtmp://127.0.0.1:1935');

    expect(screen.queryByText(/obs-websocket/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/install.*plugin/i)).not.toBeInTheDocument();
  });
});
