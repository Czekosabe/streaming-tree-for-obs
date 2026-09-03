import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConfiguredPlatform, ProviderDefinition } from '@/api/platform-schemas';
import * as accountsApi from '@/api/accounts';
import * as branchesApi from '@/api/branches';
import * as credentialsApi from '@/api/credentials';
import * as outputApi from '@/api/output';
import * as platformsApi from '@/api/platforms';
import { renderWithProviders } from '@/test/render';

import { PlatformsPage } from './PlatformsPage';

vi.mock('@/api/platforms');
vi.mock('@/api/branches');
vi.mock('@/api/credentials');
vi.mock('@/api/accounts');
vi.mock('@/api/output');

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

const KICK_PROVIDER: ProviderDefinition = {
  ...TWITCH_PROVIDER,
  id: 'kick',
  brandName: 'Kick',
  shortLabel: 'KI',
  categoryRequiresRemoteId: false,
};

// Twitch is a seeded, real destination someone has actually set up: enabled,
// a stored stream key, and a live branch.
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
    title: 'Live coding',
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

// Kick represents the seeded-but-untouched destination the operator has not
// configured yet - disabled, no stream key, no branch.
const KICK_PLATFORM: ConfiguredPlatform = {
  id: 'pf_kick',
  providerId: 'kick',
  displayName: 'Kick',
  enabled: false,
  sortOrder: 1,
  createdAt: '2026-08-01T00:00:00Z',
  updatedAt: '2026-08-01T00:00:00Z',
  provider: KICK_PROVIDER,
  metadata: {
    title: '',
    description: '',
    category: '',
    categoryId: '',
    tags: [],
    language: 'en',
    visibility: 'public',
    matureContent: false,
    dvr: false,
    latencyMode: 'normal',
    updatedAt: '2026-08-01T00:00:00Z',
  },
};

function renderPage(initialPath = '/platforms') {
  return renderWithProviders(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/platforms" element={<PlatformsPage />} />
        <Route path="/metadata" element={<div>metadata-page-marker</div>} />
        <Route path="/settings" element={<div>settings-page-marker</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(platformsApi).fetchProviderDefinitions.mockResolvedValue([TWITCH_PROVIDER, KICK_PROVIDER]);
  vi.mocked(branchesApi).fetchBranches.mockResolvedValue([]);
  vi.mocked(credentialsApi).fetchCredentialStatus.mockResolvedValue({
    streamKey: { configured: false },
    store: { available: true },
  });
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
  vi.mocked(accountsApi).fetchPlatformAccountLink.mockResolvedValue(null);
  vi.mocked(accountsApi).fetchRemoteTarget.mockResolvedValue(null);
  vi.mocked(outputApi).fetchOutputSettings.mockResolvedValue({
    serverUrl: '',
    autoRestart: false,
    updatedAt: '2026-08-01T00:00:00Z',
  });
});

describe('PlatformsPage', () => {
  it('renders a real management page, not the old placeholder', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);

    renderPage();

    expect(await screen.findByText('Main Twitch')).toBeInTheDocument();
    expect(screen.queryByText(/not implemented yet/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/planned for this view/i)).not.toBeInTheDocument();
  });

  it('represents a seeded, unconfigured destination accurately - never as already configured', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([KICK_PLATFORM]);
    vi.mocked(credentialsApi).fetchCredentialStatus.mockResolvedValue({
      streamKey: { configured: false },
      store: { available: true },
    });

    renderPage();

    expect(await screen.findByRole('heading', { name: 'Kick' })).toBeInTheDocument();
    expect(screen.getByText('Disabled')).toBeInTheDocument();
    expect(screen.getByText('Missing')).toBeInTheDocument();
  });

  it('computes the Configured summary count from real credential status, never from destination existence alone', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM, KICK_PLATFORM]);
    vi.mocked(credentialsApi).fetchCredentialStatus.mockImplementation((platformId) =>
      Promise.resolve({
        streamKey: { configured: platformId === 'pf_twitch' },
        store: { available: true },
      }),
    );
    vi.mocked(branchesApi).fetchBranches.mockResolvedValue([
      { platformId: 'pf_twitch', state: 'live', desiredRunning: true, blockers: [], restartCount: 0, progress: null, lastError: null },
    ] as never);

    renderPage();

    await screen.findByText('Main Twitch');
    const summary = screen.getByRole('group', { name: /destination summary/i });
    expect(summary).toHaveTextContent('2');
    expect(summary).toHaveTextContent('1');
  });

  it('never counts a connected account as a configured destination', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([KICK_PLATFORM]);
    vi.mocked(credentialsApi).fetchCredentialStatus.mockResolvedValue({
      streamKey: { configured: false },
      store: { available: true },
    });
    // A connected account exists for this provider, but that is a different
    // domain concept from a configured destination credential - see
    // AccountLinkSection's own doc comment.
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([
      {
        id: 'acc_1', providerId: 'kick', login: 'streamer', displayName: 'Streamer',
        status: 'connected', scopes: [], createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
      },
    ]);

    renderPage();

    await screen.findByRole('heading', { name: 'Kick' });
    const configuredStat = screen.getByText('Configured').previousElementSibling;
    expect(configuredStat).toHaveTextContent('0');
  });

  it('Add platform opens the same AddPlatformDialog Dashboard uses', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);

    renderPage();
    await waitFor(() => expect(platformsApi.fetchProviderDefinitions).toHaveBeenCalled());

    await userEvent.click(screen.getAllByRole('button', { name: /add platform/i })[0]!);

    expect(await screen.findByRole('dialog', { name: /^add platform$/i })).toBeInTheDocument();
  });

  it('a card\'s settings action opens the same PlatformSettingsDialog Dashboard uses', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);

    renderPage();
    await screen.findByText('Main Twitch');

    await userEvent.click(screen.getByRole('button', { name: /open settings for main twitch/i }));

    expect(await screen.findByRole('dialog', { name: /platform settings/i })).toBeInTheDocument();
  });

  it('a card\'s Edit metadata action navigates to the Metadata page', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([TWITCH_PLATFORM]);

    renderPage();
    await screen.findByText('Main Twitch');

    await userEvent.click(screen.getByRole('button', { name: /edit metadata for main twitch/i }));

    expect(await screen.findByText('metadata-page-marker')).toBeInTheDocument();
  });

  it('links to the canonical connected-accounts surface instead of duplicating account linking', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);

    renderPage();
    await waitFor(() => expect(platformsApi.fetchPlatforms).toHaveBeenCalled());

    await userEvent.click(screen.getByRole('button', { name: /connected accounts/i }));

    expect(await screen.findByText('settings-page-marker')).toBeInTheDocument();
  });

  it('shows an error state with retry on failure', async () => {
    vi.mocked(platformsApi).fetchPlatforms.mockRejectedValue(new Error('network down'));

    renderPage();

    expect(await screen.findByText(/could not be loaded/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument();
  });
});
