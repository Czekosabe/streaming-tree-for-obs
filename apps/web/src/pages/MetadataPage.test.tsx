import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConfiguredPlatform, ProviderDefinition } from '@/api/platform-schemas';
import * as metadataPresetsApi from '@/api/metadata-presets';
import * as platformsApi from '@/api/platforms';
import { renderWithProviders } from '@/test/render';

import { MetadataPage } from './MetadataPage';

vi.mock('@/api/platforms');
vi.mock('@/api/metadata-presets');

const TWITCH_PROVIDER: ProviderDefinition = {
  id: 'twitch',
  brandName: 'Twitch',
  shortLabel: 'TW',
  categoryFieldType: 'category',
  categoryRequiresRemoteId: true,
  capabilities: {
    title: true,
    description: false,
    category: true,
    tags: true,
    language: true,
    visibility: false,
    matureContent: false,
    dvr: false,
    latencyMode: false,
  },
  limits: { titleMaxLength: 140, descriptionMaxLength: 0, maxTags: 10, tagMaxLength: 25 },
  visibilityOptions: [],
  latencyOptions: [],
  languageOptions: ['en'],
};

const TWITCH_PLATFORM: ConfiguredPlatform = {
  id: 'pf_twitch',
  providerId: 'twitch',
  displayName: 'Main Twitch',
  enabled: true,
  sortOrder: 0,
  createdAt: '2026-08-01T00:00:00Z',
  updatedAt: '2026-08-01T00:00:00Z',
  provider: TWITCH_PROVIDER,
  metadata: {
    title: 'Live coding session',
    description: '',
    category: 'Software and Game Development',
    categoryId: '1469308723',
    tags: [],
    language: 'en',
    visibility: 'public',
    matureContent: false,
    dvr: true,
    latencyMode: 'normal',
    updatedAt: '2026-08-01T00:00:00Z',
  },
};

const KICK_PLATFORM: ConfiguredPlatform = {
  ...TWITCH_PLATFORM,
  id: 'pf_kick',
  providerId: 'kick',
  displayName: 'My Kick',
  provider: { ...TWITCH_PROVIDER, id: 'kick', brandName: 'Kick', shortLabel: 'KI' },
  metadata: { ...TWITCH_PLATFORM.metadata, title: 'Kick stream title' },
};

function renderPage(entry: { pathname: string; state?: unknown } = { pathname: '/metadata' }) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/metadata" element={<MetadataPage />} />
        <Route path="/platforms" element={<div>platforms-page-marker</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(metadataPresetsApi).fetchMetadataPresets.mockResolvedValue([]);
});

describe('MetadataPage', () => {
  it('renders a real metadata management page, not the old placeholder', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);

    renderPage();

    expect(await screen.findByDisplayValue('Live coding session')).toBeInTheDocument();
    expect(screen.queryByText(/not implemented yet/i)).not.toBeInTheDocument();
  });

  it('saves an edit through the canonical mutation path and reflects it as saved', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);
    vi.mocked(platformsApi).savePlatformMetadata.mockResolvedValue({
      ...TWITCH_PLATFORM.metadata,
      title: 'New title',
    });

    renderPage();
    const titleInput = await screen.findByDisplayValue('Live coding session');
    await userEvent.clear(titleInput);
    await userEvent.type(titleInput, 'New title');

    await userEvent.click(screen.getByRole('button', { name: /^save metadata$/i }));

    await waitFor(() => expect(platformsApi.savePlatformMetadata).toHaveBeenCalledWith('pf_twitch', expect.objectContaining({ title: 'New title' })));
    expect(await screen.findByText(/saved to the local database/i)).toBeInTheDocument();
  });

  it('keeps a save failure visible instead of silently discarding the edit', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);
    vi.mocked(platformsApi).savePlatformMetadata.mockRejectedValue(new Error('backend unreachable'));

    renderPage();
    await screen.findByDisplayValue('Live coding session');

    await userEvent.click(screen.getByRole('button', { name: /^save metadata$/i }));

    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });

  it('exposes Manage presets, which is how Apply is reached', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);

    renderPage();
    await screen.findByDisplayValue('Live coding session');

    await userEvent.click(screen.getByRole('button', { name: /^presets$/i }));

    expect(await screen.findByRole('dialog', { name: /metadata presets/i })).toBeInTheDocument();
  });

  it('exposes Save as preset directly from the form', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);

    renderPage();
    await screen.findByDisplayValue('Live coding session');

    await userEvent.click(screen.getByRole('button', { name: /^save as preset$/i }));

    expect(await screen.findByRole('dialog', { name: /^save as preset$/i })).toBeInTheDocument();
  });

  it('does not render a field the active provider does not support', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);

    renderPage();
    await screen.findByDisplayValue('Live coding session');

    // TWITCH_PROVIDER's capabilities set visibility/matureContent/dvr/
    // latencyMode/description all false - the form must be honest about
    // that, never rendering a control for a field the provider cannot
    // actually publish.
    expect(screen.queryByLabelText(/^visibility$/i)).not.toBeInTheDocument();
  });

  it('preselects the destination handed off from the Platforms page', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM, KICK_PLATFORM]);

    renderPage({ pathname: '/metadata', state: { platformId: 'pf_kick' } });

    expect(await screen.findByDisplayValue('Kick stream title')).toBeInTheDocument();
  });

  it('shows a real empty state pointing to Platforms when there are no destinations yet', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);

    renderPage();

    expect(await screen.findByText(/no destinations yet/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /go to platforms/i }));
    expect(await screen.findByText('platforms-page-marker')).toBeInTheDocument();
  });
});
