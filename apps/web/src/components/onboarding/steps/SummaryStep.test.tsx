import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as branchesApi from '@/api/branches';
import * as platformsApi from '@/api/platforms';
import * as runtimeApi from '@/api/runtime';
import type * as ApiClientModule from '@/lib/api-client';
import { renderWithProviders } from '@/test/render';

import { SummaryStep } from './SummaryStep';

vi.mock('@/api/runtime');
vi.mock('@/api/platforms');
vi.mock('@/api/branches');
vi.mock('@/api/accounts');
vi.mock('@/lib/api-client', async (importOriginal) => {
  const actual = await importOriginal<typeof ApiClientModule>();
  return {
    ...actual,
    apiGet: vi.fn().mockResolvedValue({ status: 'ok', service: 'streaming-tree', version: '0.1.0' }),
  };
});

const MEDIAMTX_READY = {
  supportedVersion: '1.19.3', source: 'managed' as const, state: 'ready' as const,
  autoStart: true, autoRestart: true, restartCount: 0, lastError: null,
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
    version: 1,
    mediaMtx: MEDIAMTX_READY,
    ingest: { state: 'waiting' as const, path: 'live', trackCount: null, tracks: [] },
    connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
  });
  vi.mocked(branchesApi).fetchBranches.mockResolvedValue([]);
});

describe('SummaryStep', () => {
  it('shows zero destinations and zero accounts as valid, not as a failure', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);

    renderWithProviders(<SummaryStep />);

    expect(await screen.findByText(/0 configured, 0 enabled, 0 active/i)).toBeInTheDocument();
    expect(screen.getByText(/0 connected/i)).toBeInTheDocument();
    expect(screen.getAllByText(/optional/i).length).toBeGreaterThanOrEqual(2);
  });

  it('counts real configured/enabled/active destinations and connected accounts', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([
      { id: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch', enabled: true },
      { id: 'pf_2', providerId: 'youtube', displayName: 'Backup YouTube', enabled: false },
    ] as never);
    vi.mocked(branchesApi).fetchBranches.mockResolvedValue([
      {
        platformId: 'pf_1', state: 'live', desiredRunning: true, blockers: [],
        restartCount: 0, progress: null, lastError: null,
      },
    ] as never);
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([
      {
        id: 'acc_1', providerId: 'twitch', login: 'streamer', displayName: 'Streamer',
        status: 'connected', scopes: [], createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
      },
    ]);

    renderWithProviders(<SummaryStep />);

    expect(await screen.findByText(/2 configured, 1 enabled, 1 active/i)).toBeInTheDocument();
    expect(screen.getByText(/1 connected/i)).toBeInTheDocument();
  });

  it('shows the real OBS ingest state', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: MEDIAMTX_READY,
      ingest: { state: 'receiving' as const, path: 'live', sourceType: 'rtmpConn', trackCount: 1, tracks: ['H264'] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<SummaryStep />);

    expect(await screen.findByText(/receiving/i)).toBeInTheDocument();
  });

  it('never fabricates a viewer count anywhere in the summary', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);

    renderWithProviders(<SummaryStep />);

    await screen.findByText(/0 connected/i);
    expect(screen.queryByText(/viewers?/i)).not.toBeInTheDocument();
  });
});
