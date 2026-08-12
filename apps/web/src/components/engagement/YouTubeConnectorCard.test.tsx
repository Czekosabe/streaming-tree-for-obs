import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConnectedAccount } from '@/api/account-schemas';
import * as engagementApi from '@/api/engagement';
import type { AccountEngagement } from '@/api/engagement-schemas';
import { renderWithProviders } from '@/test/render';

import { YouTubeConnectorCard } from './YouTubeConnectorCard';

vi.mock('@/api/engagement');

const engagement = vi.mocked(engagementApi);

beforeEach(() => {
  vi.clearAllMocks();
});

const ACCOUNT: ConnectedAccount = {
  id: 'acct_youtube_1',
  providerId: 'youtube',
  login: 'My Channel',
  displayName: 'My Channel',
  status: 'connected',
  scopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
  createdAt: '2026-08-05T00:00:00Z',
  updatedAt: '2026-08-05T00:00:00Z',
};

function connected(overrides: Partial<AccountEngagement> = {}): AccountEngagement {
  return {
    accountId: 'acct_youtube_1',
    provider: 'youtube',
    enabled: true,
    state: 'connected',
    reconnectCount: 0,
    selectedBroadcastId: 'broadcast_1',
    requiredScopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
    grantedScopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
    permissionUpgradeRequired: false,
    ...overrides,
  };
}

describe('YouTubeConnectorCard connected state', () => {
  it('shows the connected status', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);

    await screen.findByText(/connected/i);
  });

  it('never shows a permission-upgrade action - YouTube needs no additional scope', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);
    await screen.findByText(/connected/i);

    expect(screen.queryByRole('button', { name: /authorize/i })).not.toBeInTheDocument();
  });

  it('never renders a session id, reconnect URL, or token-shaped value', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);
    await screen.findByText(/connected/i);

    const rendered = document.body.textContent ?? '';
    expect(rendered).not.toMatch(/access[_-]?token/i);
    expect(rendered).not.toMatch(/session[_-]?id/i);
    expect(rendered).not.toMatch(/reconnect[_-]?url/i);
  });

  it('warns when enabled but no broadcast is selected', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(
      connected({ state: 'waiting_for_welcome', selectedBroadcastId: undefined }),
    );

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);

    await screen.findByText(/no broadcast is currently selected/i);
  });

  it('does not warn about a missing broadcast once one is selected', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);
    await screen.findByText(/connected/i);

    expect(screen.queryByText(/no broadcast is currently selected/i)).not.toBeInTheDocument();
  });
});

describe('YouTubeConnectorCard enable/disable', () => {
  it('enables without a confirmation dialog', async () => {
    const user = userEvent.setup();
    engagement.fetchAccountEngagement.mockResolvedValue(connected({ enabled: false, state: 'disabled' }));
    engagement.setAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);

    const toggle = await screen.findByRole('switch');
    await user.click(toggle);

    await waitFor(() =>
      expect(engagement.setAccountEngagement).toHaveBeenCalledWith('acct_youtube_1', { enabled: true }),
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('requires confirmation before disabling an active connector', async () => {
    const user = userEvent.setup();
    engagement.fetchAccountEngagement.mockResolvedValue(connected());
    engagement.setAccountEngagement.mockResolvedValue(connected({ enabled: false, state: 'disabled' }));

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);

    const toggle = await screen.findByRole('switch');
    await user.click(toggle);

    const dialog = await screen.findByRole('dialog');
    expect(engagement.setAccountEngagement).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: /^disable$/i }));

    await waitFor(() =>
      expect(engagement.setAccountEngagement).toHaveBeenCalledWith('acct_youtube_1', { enabled: false }),
    );
  });
});

describe('YouTubeConnectorCard restart', () => {
  it('offers a restart action only in the error state', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected({ state: 'error', lastError: 'boom' }));

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);

    await screen.findByRole('button', { name: /restart connector/i });
  });

  it('does not offer a restart action while connected', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<YouTubeConnectorCard account={ACCOUNT} />);
    await screen.findByText(/connected/i);

    expect(screen.queryByRole('button', { name: /restart connector/i })).not.toBeInTheDocument();
  });
});
