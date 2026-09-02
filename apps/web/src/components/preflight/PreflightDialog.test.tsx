import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as branchesApi from '@/api/branches';
import * as preflightApi from '@/api/preflight';
import type { PreflightReport } from '@/api/preflight-schemas';
import type { ConfiguredPlatform } from '@/api/platform-schemas';
import * as streamSetupsApi from '@/api/stream-setups';
import { renderWithProviders } from '@/test/render';

import { PreflightDialog } from './PreflightDialog';

vi.mock('@/api/preflight');
vi.mock('@/api/stream-setups');
vi.mock('@/api/branches');

function platform(overrides: Partial<ConfiguredPlatform> = {}): ConfiguredPlatform {
  return {
    id: 'pf_1',
    providerId: 'twitch',
    displayName: 'Main Twitch',
    enabled: true,
    sortOrder: 0,
    createdAt: '2026-08-01T00:00:00Z',
    updatedAt: '2026-08-01T00:00:00Z',
    metadata: {
      title: '', description: '', category: '', categoryId: '', tags: [],
      language: '', visibility: '', matureContent: false, dvr: false, latencyMode: '',
      updatedAt: '2026-08-01T00:00:00Z',
    },
    ...overrides,
  };
}

function report(overrides: Partial<PreflightReport> = {}): PreflightReport {
  return {
    status: 'ready',
    findings: [],
    destinations: [
      { platformId: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch', findings: [] },
    ],
    streamingActive: false,
    ...overrides,
  };
}

function renderDialog(onClose = vi.fn()) {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route
          path="/"
          element={
            <PreflightDialog
              open
              onClose={onClose}
              platforms={[platform()]}
              onOpenDestinationSettings={vi.fn()}
              onEditMetadata={vi.fn()}
              onOpenStreamSetups={vi.fn()}
            />
          }
        />
        <Route path="/settings" element={<p>Settings page marker</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(streamSetupsApi).fetchStreamSetups.mockResolvedValue([]);
  vi.mocked(branchesApi).fetchBranches.mockResolvedValue([]);
});

describe('PreflightDialog', () => {
  it('shows the ready status when there are no findings', async () => {
    vi.mocked(preflightApi).fetchPreflight.mockResolvedValue(report());
    renderDialog();

    expect(await screen.findByText('Ready to stream')).toBeInTheDocument();
    expect(screen.getByText('Main Twitch')).toBeInTheDocument();
  });

  it('shows not ready with a blocker and its action', async () => {
    vi.mocked(preflightApi).fetchPreflight.mockResolvedValue(
      report({
        status: 'not_ready',
        findings: [
          { code: 'stream_key_missing', severity: 'blocker', platformId: 'pf_1',
            action: { code: 'add_stream_key', platformId: 'pf_1' } },
        ],
        destinations: [
          { platformId: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch', findings: [
            { code: 'stream_key_missing', severity: 'blocker', platformId: 'pf_1',
              action: { code: 'add_stream_key', platformId: 'pf_1' } },
          ] },
        ],
      }),
    );
    const onOpenDestinationSettings = vi.fn();
    renderWithProviders(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route
            path="/"
            element={
              <PreflightDialog
                open
                onClose={vi.fn()}
                platforms={[platform()]}
                onOpenDestinationSettings={onOpenDestinationSettings}
                onEditMetadata={vi.fn()}
                onOpenStreamSetups={vi.fn()}
              />
            }
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByText('Not ready')).toBeInTheDocument();
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Add stream key' }));
    expect(onOpenDestinationSettings).toHaveBeenCalledWith('pf_1');
  });

  it('shows the streaming-active notice instead of findings while a broadcast is active', async () => {
    vi.mocked(preflightApi).fetchPreflight.mockResolvedValue(report({ streamingActive: true }));
    renderDialog();

    expect(
      await screen.findByText('Pre-stream check unavailable while streaming. Check the Dashboard or History for current stream status instead.'),
    ).toBeInTheDocument();
    expect(screen.queryByText('Ready to stream')).not.toBeInTheDocument();
  });

  it('re-fetches with the selected profile id when the profile selector changes', async () => {
    vi.mocked(preflightApi).fetchPreflight.mockResolvedValue(report());
    vi.mocked(streamSetupsApi).fetchStreamSetups.mockResolvedValue([
      {
        id: 'setup_1', name: 'Gaming', note: '', destinations: [], metadataPresetId: null,
        metadataPresetName: '', metadataPresetMissing: false,
        createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-09-01T00:00:00Z',
      },
    ]);
    renderDialog();

    await screen.findByText('Ready to stream');
    const user = userEvent.setup();
    await user.selectOptions(screen.getByRole('combobox'), 'Gaming');

    await waitFor(() => expect(preflightApi.fetchPreflight).toHaveBeenCalledWith('setup_1', expect.anything()));
  });
});
