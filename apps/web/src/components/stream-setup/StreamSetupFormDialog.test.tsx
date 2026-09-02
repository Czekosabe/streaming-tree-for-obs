import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as streamSetupsApi from '@/api/stream-setups';
import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { renderWithProviders } from '@/test/render';

import { StreamSetupFormDialog } from './StreamSetupFormDialog';

vi.mock('@/api/stream-setups');

function platform(overrides: Partial<ConfiguredPlatform> = {}): ConfiguredPlatform {
  return {
    id: 'pf_1',
    providerId: 'twitch',
    displayName: 'Main Twitch',
    enabled: false,
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

beforeEach(() => {
  vi.clearAllMocks();
});

describe('StreamSetupFormDialog', () => {
  it('creates a new setup with the selected destinations', async () => {
    vi.mocked(streamSetupsApi).createStreamSetup.mockResolvedValue({
      id: 'setup_new', name: 'Gaming', note: '',
      destinations: [{ platformId: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch' }],
      metadataPresetId: null, metadataPresetName: '', metadataPresetMissing: false,
      createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-09-01T00:00:00Z',
    });
    const onClose = vi.fn();
    renderWithProviders(
      <StreamSetupFormDialog open onClose={onClose} platforms={[platform()]} metadataPresets={[]} editing={null} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Setup name'), 'Gaming');
    await user.click(screen.getByRole('checkbox'));
    await user.click(screen.getByRole('button', { name: 'Save setup' }));

    await waitFor(() =>
      expect(streamSetupsApi.createStreamSetup).toHaveBeenCalledWith({
        name: 'Gaming', note: '', destinationIds: ['pf_1'], metadataPresetId: null,
      }),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('pre-fills the form when editing an existing setup', () => {
    renderWithProviders(
      <StreamSetupFormDialog
        open
        onClose={vi.fn()}
        platforms={[platform()]}
        metadataPresets={[]}
        editing={{
          id: 'setup_1', name: 'Podcast', note: 'weekly',
          destinations: [{ platformId: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch' }],
          metadataPresetId: null, metadataPresetName: '', metadataPresetMissing: false,
          createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-09-01T00:00:00Z',
        }}
      />,
    );

    expect(screen.getByLabelText('Setup name')).toHaveValue('Podcast');
    expect(screen.getByRole('checkbox')).toBeChecked();
  });
});
