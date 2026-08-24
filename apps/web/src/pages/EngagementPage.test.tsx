import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as engagementApi from '@/api/engagement';
import { renderWithProviders } from '@/test/render';

import { EngagementPage } from './EngagementPage';

// AppShell's sidebar renders NavLink, which needs a Router context - App.tsx
// normally supplies BrowserRouter; a MemoryRouter is the standard
// react-router-dom test substitute.
function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <EngagementPage />
    </MemoryRouter>,
  );
}

vi.mock('@/api/accounts');
vi.mock('@/api/engagement');

class FakeEventSource {
  constructor(public url: string) {}
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('EventSource', FakeEventSource);
});

describe('EngagementPage', () => {
  it('loads and shows the Event Bus status once data resolves', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
    vi.mocked(engagementApi).fetchEngagementStatus.mockResolvedValue({
      schemaVersion: 1,
      bufferCapacity: 1000,
      retainedCount: 0,
      oldestSequence: 0,
      newestSequence: 0,
      activeSubscribers: 0,
      connectors: [],
    });

    renderPage();

    expect(await screen.findByText('0/1000')).toBeInTheDocument();
  });

  it('shows an error state, not a stuck "loading" message, when the Event Bus status request fails', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
    vi.mocked(engagementApi).fetchEngagementStatus.mockRejectedValue(new Error('network down'));

    renderPage();

    expect(await screen.findByText(/event bus status could not be loaded/i)).toBeInTheDocument();
    expect(screen.queryByText(/loading event bus status/i)).not.toBeInTheDocument();
  });

  it('shows a clear empty state when no Twitch or YouTube account is connected', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
    vi.mocked(engagementApi).fetchEngagementStatus.mockResolvedValue({
      schemaVersion: 1,
      bufferCapacity: 1000,
      retainedCount: 0,
      oldestSequence: 0,
      newestSequence: 0,
      activeSubscribers: 0,
      connectors: [],
    });

    renderPage();

    expect(await screen.findByText(/no connected twitch or youtube account yet/i)).toBeInTheDocument();
  });

  it('renders a connector card for each connected Twitch and YouTube account (Stage 15A)', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([
      {
        id: 'acct_twitch_1',
        providerId: 'twitch',
        login: 'streamer',
        displayName: 'Streamer',
        status: 'connected',
        scopes: ['channel:manage:broadcast'],
        createdAt: '2026-08-05T00:00:00Z',
        updatedAt: '2026-08-05T00:00:00Z',
      },
      {
        id: 'acct_youtube_1',
        providerId: 'youtube',
        login: 'My Channel',
        displayName: 'My Channel',
        status: 'connected',
        scopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
        createdAt: '2026-08-05T00:00:00Z',
        updatedAt: '2026-08-05T00:00:00Z',
      },
    ]);
    vi.mocked(engagementApi).fetchEngagementStatus.mockResolvedValue({
      schemaVersion: 1,
      bufferCapacity: 1000,
      retainedCount: 0,
      oldestSequence: 0,
      newestSequence: 0,
      activeSubscribers: 0,
      connectors: [],
    });
    vi.mocked(engagementApi).fetchAccountEngagement.mockImplementation((accountId: string) =>
      Promise.resolve({
        accountId, provider: accountId === 'acct_youtube_1' ? 'youtube' : 'twitch',
        enabled: false, state: 'disabled', reconnectCount: 0,
        activeSubscriptionCount: 0, expectedSubscriptionCount: 0,
        requiredScopes: [], grantedScopes: [], permissionUpgradeRequired: false,
      }),
    );

    renderPage();

    expect(await screen.findByText('Streamer')).toBeInTheDocument();
    expect(await screen.findByText('My Channel')).toBeInTheDocument();
  });
});
