import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as metadataPresetsApi from '@/api/metadata-presets';
import * as streamSetupsApi from '@/api/stream-setups';
import type { StreamSetupProfile } from '@/api/stream-setup-schemas';
import { renderWithProviders } from '@/test/render';

import { StreamSetupsDialog } from './StreamSetupsDialog';

vi.mock('@/api/stream-setups');
vi.mock('@/api/metadata-presets');

function renderDialog(onClose = vi.fn()) {
  return renderWithProviders(
    <StreamSetupsDialog
      open
      onClose={onClose}
      platforms={[]}
      activeMetadataId={null}
      activeMetadataDirty={false}
    />,
  );
}

function profile(overrides: Partial<StreamSetupProfile> = {}): StreamSetupProfile {
  return {
    id: 'setup_1',
    name: 'Gaming',
    note: 'Weekly gaming stream',
    destinations: [{ platformId: 'pf_1', providerId: 'twitch', displayName: 'My Twitch' }],
    metadataPresetId: null,
    metadataPresetName: '',
    metadataPresetMissing: false,
    createdAt: '2026-09-01T00:00:00Z',
    updatedAt: '2026-09-01T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(metadataPresetsApi).fetchMetadataPresets.mockResolvedValue([]);
});

describe('StreamSetupsDialog', () => {
  it('shows a creator-oriented empty state when there are no setups', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetups.mockResolvedValue([]);
    renderDialog();

    expect(await screen.findByText('No stream setups yet')).toBeInTheDocument();
  });

  it('lists an existing setup with its name and note', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetups.mockResolvedValue([profile()]);
    renderDialog();

    expect(await screen.findByText('Gaming')).toBeInTheDocument();
    expect(screen.getByText('Weekly gaming stream')).toBeInTheDocument();
  });

  it('shows a warning when the setup references a deleted metadata preset', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetups.mockResolvedValue([
      profile({ metadataPresetMissing: true, metadataPresetName: 'Old preset' }),
    ]);
    renderDialog();

    expect(await screen.findByText('Metadata preset missing')).toBeInTheDocument();
  });

  it('duplicates a setup under a name the user confirms', async () => {
    const existing = profile();
    vi.mocked(streamSetupsApi).fetchStreamSetups.mockResolvedValue([existing]);
    vi.mocked(streamSetupsApi).duplicateStreamSetup.mockResolvedValue({
      ...existing,
      id: 'setup_2',
      name: 'Gaming (copy)',
    });
    renderDialog();

    await screen.findByText('Gaming');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Duplicate' }));

    const input = screen.getByLabelText('New setup name');
    expect(input).toHaveValue('Gaming (copy)');
    await user.click(screen.getByRole('button', { name: 'Save name' }));

    await waitFor(() =>
      expect(streamSetupsApi.duplicateStreamSetup).toHaveBeenCalledWith('setup_1', 'Gaming (copy)'),
    );
  });

  it('deletes a setup only after the confirmation dialog is accepted', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetups.mockResolvedValue([profile()]);
    vi.mocked(streamSetupsApi).deleteStreamSetup.mockResolvedValue(undefined);
    renderDialog();

    await screen.findByText('Gaming');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Delete setup' }));

    expect(streamSetupsApi.deleteStreamSetup).not.toHaveBeenCalled();

    const dialog = await screen.findByText('Delete this setup?');
    const confirmDialog = dialog.closest('[role="dialog"]') ?? document.body;
    await user.click(within(confirmDialog as HTMLElement).getByRole('button', { name: 'Delete setup' }));

    await waitFor(() => expect(streamSetupsApi.deleteStreamSetup).toHaveBeenCalledWith('setup_1'));
  });
});
