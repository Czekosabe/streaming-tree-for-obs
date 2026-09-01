import { screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as platformsApi from '@/api/platforms';
import { renderWithProviders } from '@/test/render';

import { DestinationsStep } from './DestinationsStep';

vi.mock('@/api/platforms');

const DEFINITION = {
  id: 'twitch',
  brandName: 'Twitch',
  shortLabel: 'Twitch',
  categoryFieldType: 'text',
  categoryRequiresRemoteId: false,
  capabilities: {
    title: true, description: false, category: true, tags: true, language: false,
    visibility: false, matureContent: false, dvr: false, latencyMode: false,
  },
  limits: { titleMaxLength: 140, descriptionMaxLength: 0, maxTags: 10, tagMaxLength: 25 },
  visibilityOptions: [],
  latencyOptions: [],
  languageOptions: [],
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(platformsApi).fetchProviderDefinitions.mockResolvedValue([DEFINITION]);
});

describe('DestinationsStep', () => {
  it('allows zero destinations - the empty state is not treated as a failure', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);

    renderWithProviders(<DestinationsStep />);

    expect(await screen.findByText(/no destinations configured yet/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', { name: /add destination/i })).toBeEnabled());
  });

  it('lists real configured destinations with their real enabled/disabled state', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([
      { id: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch', enabled: true },
      { id: 'pf_2', providerId: 'youtube', displayName: 'Backup YouTube', enabled: false },
    ] as never);

    renderWithProviders(<DestinationsStep />);

    expect(await screen.findByText('Main Twitch')).toBeInTheDocument();
    expect(screen.getByText('Backup YouTube')).toBeInTheDocument();
    expect(screen.getByText('Enabled')).toBeInTheDocument();
    expect(screen.getByText('Disabled')).toBeInTheDocument();
  });

  it('never invents a viewer count or other fabricated destination detail', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([
      { id: 'pf_1', providerId: 'twitch', displayName: 'Main Twitch', enabled: true },
    ] as never);

    renderWithProviders(<DestinationsStep />);

    await screen.findByText('Main Twitch');
    expect(screen.queryByText(/viewers?/i)).not.toBeInTheDocument();
  });
});
