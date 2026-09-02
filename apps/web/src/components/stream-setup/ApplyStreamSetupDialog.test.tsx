import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as streamSetupsApi from '@/api/stream-setups';
import type { StreamSetupPreview, StreamSetupProfile } from '@/api/stream-setup-schemas';
import { renderWithProviders } from '@/test/render';

import { ApplyStreamSetupDialog } from './ApplyStreamSetupDialog';

vi.mock('@/api/stream-setups');

const profile: StreamSetupProfile = {
  id: 'setup_1',
  name: 'Gaming',
  note: '',
  destinations: [{ platformId: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch' }],
  metadataPresetId: null,
  metadataPresetName: '',
  metadataPresetMissing: false,
  createdAt: '2026-09-01T00:00:00Z',
  updatedAt: '2026-09-01T00:00:00Z',
};

function preview(overrides: Partial<StreamSetupPreview> = {}): StreamSetupPreview {
  return {
    profile,
    destinations: [
      {
        platformId: 'pf_1',
        providerId: 'twitch',
        displayName: 'Main Twitch',
        currentlyEnabled: false,
        change: 'will_enable',
        active: false,
      },
    ],
    metadataPresetReferenced: false,
    metadataPresetMissing: false,
    metadataPresetName: '',
    metadataDestinationPreviews: [],
    blocked: false,
    blockedDestinationIds: [],
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ApplyStreamSetupDialog', () => {
  it('shows the classified destination list from the preview', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetupPreview.mockResolvedValue(preview());
    renderWithProviders(
      <ApplyStreamSetupDialog
        open onClose={vi.fn()} profile={profile} activeMetadataId={null} activeMetadataDirty={false}
      />,
    );

    expect(await screen.findByText('Main Twitch')).toBeInTheDocument();
    expect(screen.getByText('will enable')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply setup' })).toBeEnabled();
  });

  it('disables Apply and shows the blocked message when an affected destination is live', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetupPreview.mockResolvedValue(
      preview({ blocked: true, blockedDestinationIds: ['pf_1'] }),
    );
    renderWithProviders(
      <ApplyStreamSetupDialog
        open onClose={vi.fn()} profile={profile} activeMetadataId={null} activeMetadataDirty={false}
      />,
    );

    expect(
      await screen.findByText('Stop streaming on the affected destination(s) before changing this setup.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply setup' })).toBeDisabled();
  });

  it('shows a warning when the referenced metadata preset is missing', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetupPreview.mockResolvedValue(
      preview({ metadataPresetMissing: true, metadataPresetReferenced: true }),
    );
    renderWithProviders(
      <ApplyStreamSetupDialog
        open onClose={vi.fn()} profile={profile} activeMetadataId={null} activeMetadataDirty={false}
      />,
    );

    expect(
      await screen.findByText(
        'The metadata preset this setup referenced has been deleted. Destination membership can still be applied; no metadata will be changed.',
      ),
    ).toBeInTheDocument();
  });

  it('applies directly when there is no unsaved-edit conflict', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetupPreview.mockResolvedValue(preview());
    vi.mocked(streamSetupsApi).applyStreamSetup.mockResolvedValue({
      destinationsChanged: 1,
      metadataApplied: false,
    });
    renderWithProviders(
      <ApplyStreamSetupDialog
        open onClose={vi.fn()} profile={profile} activeMetadataId={null} activeMetadataDirty={false}
      />,
    );

    const user = userEvent.setup();
    await screen.findByText('Main Twitch');
    await user.click(screen.getByRole('button', { name: 'Apply setup' }));

    await waitFor(() => expect(streamSetupsApi.applyStreamSetup).toHaveBeenCalledWith('setup_1'));
  });

  it('confirms before discarding unsaved metadata edits on a destination this apply would touch', async () => {
    vi.mocked(streamSetupsApi).fetchStreamSetupPreview.mockResolvedValue(
      preview({
        metadataPresetReferenced: true,
        metadataDestinationPreviews: [
          { platformId: 'pf_1', providerId: 'twitch', valid: true, fields: [] },
        ],
      }),
    );
    vi.mocked(streamSetupsApi).applyStreamSetup.mockResolvedValue({
      destinationsChanged: 1,
      metadataApplied: true,
    });
    renderWithProviders(
      <ApplyStreamSetupDialog
        open onClose={vi.fn()} profile={profile} activeMetadataId="pf_1" activeMetadataDirty
      />,
    );

    const user = userEvent.setup();
    await screen.findByText('Main Twitch');
    await user.click(screen.getByRole('button', { name: 'Apply setup' }));

    expect(streamSetupsApi.applyStreamSetup).not.toHaveBeenCalled();
    expect(await screen.findByText('Discard unsaved changes?')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Discard and apply' }));
    await waitFor(() => expect(streamSetupsApi.applyStreamSetup).toHaveBeenCalledWith('setup_1'));
  });
});
