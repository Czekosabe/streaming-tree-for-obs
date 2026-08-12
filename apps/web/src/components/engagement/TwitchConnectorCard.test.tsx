import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConnectedAccount } from '@/api/account-schemas';
import * as engagementApi from '@/api/engagement';
import type { AccountEngagement } from '@/api/engagement-schemas';
import { renderWithProviders } from '@/test/render';

import { TwitchConnectorCard } from './TwitchConnectorCard';

vi.mock('@/api/engagement');

const engagement = vi.mocked(engagementApi);

beforeEach(() => {
  vi.clearAllMocks();
});

const ACCOUNT: ConnectedAccount = {
  id: 'acct_twitch_1',
  providerId: 'twitch',
  login: 'streamer',
  displayName: 'Streamer',
  status: 'connected',
  scopes: ['channel:manage:broadcast'],
  createdAt: '2026-08-05T00:00:00Z',
  updatedAt: '2026-08-05T00:00:00Z',
};

function connected(overrides: Partial<AccountEngagement> = {}): AccountEngagement {
  return {
    accountId: 'acct_twitch_1',
    provider: 'twitch',
    enabled: true,
    state: 'connected',
    reconnectCount: 0,
    activeSubscriptionCount: 13,
    expectedSubscriptionCount: 13,
    requiredScopes: [
      'user:read:chat',
      'moderator:read:followers',
      'channel:read:subscriptions',
      'bits:read',
      'channel:read:redemptions',
    ],
    grantedScopes: ['channel:manage:broadcast', 'user:read:chat'],
    permissionUpgradeRequired: false,
    ...overrides,
  };
}

describe('TwitchConnectorCard connected state', () => {
  it('shows the connected status and subscription counts', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<TwitchConnectorCard account={ACCOUNT} />);

    await screen.findByText(/connected/i);
    expect(screen.getByText('13/13')).toBeInTheDocument();
  });

  it('never renders a session id, reconnect URL, or token-shaped value', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<TwitchConnectorCard account={ACCOUNT} />);
    await screen.findByText(/connected/i);

    const rendered = document.body.textContent ?? '';
    expect(rendered).not.toMatch(/access[_-]?token/i);
    expect(rendered).not.toMatch(/session[_-]?id/i);
    expect(rendered).not.toMatch(/reconnect[_-]?url/i);
  });
});

describe('TwitchConnectorCard permission upgrade', () => {
  it('shows the upgrade action when scopes are missing and starts an authorization attempt on click', async () => {
    const user = userEvent.setup();
    engagement.fetchAccountEngagement.mockResolvedValue(
      connected({
        state: 'blocked',
        activeSubscriptionCount: 0,
        grantedScopes: ['channel:manage:broadcast'],
        permissionUpgradeRequired: true,
      }),
    );
    engagement.authorizeEngagement.mockResolvedValue({
      attemptId: 'devflow_1',
      providerId: 'twitch',
      state: 'waiting_for_user',
      userCode: 'ABCD-EFGH',
      verificationUri: 'https://www.twitch.tv/activate',
      createdAt: '2026-08-05T00:00:00Z',
    });

    renderWithProviders(<TwitchConnectorCard account={ACCOUNT} />);

    const upgradeButton = await screen.findByRole('button', { name: /authorize engagement access/i });
    await user.click(upgradeButton);

    await waitFor(() => expect(engagement.authorizeEngagement).toHaveBeenCalledWith('acct_twitch_1'));
    await screen.findByText(/ABCD-EFGH/);
  });

  it('does not show the upgrade action once every engagement scope is granted', async () => {
    engagement.fetchAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<TwitchConnectorCard account={ACCOUNT} />);
    await screen.findByText(/connected/i);

    expect(screen.queryByRole('button', { name: /authorize engagement access/i })).not.toBeInTheDocument();
  });
});

describe('TwitchConnectorCard enable/disable', () => {
  it('enables without a confirmation dialog', async () => {
    const user = userEvent.setup();
    engagement.fetchAccountEngagement.mockResolvedValue(connected({ enabled: false, state: 'disabled' }));
    engagement.setAccountEngagement.mockResolvedValue(connected());

    renderWithProviders(<TwitchConnectorCard account={ACCOUNT} />);

    const toggle = await screen.findByRole('switch');
    await user.click(toggle);

    await waitFor(() =>
      expect(engagement.setAccountEngagement).toHaveBeenCalledWith('acct_twitch_1', { enabled: true }),
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('requires confirmation before disabling an active connector, never using window.confirm', async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, 'confirm');
    engagement.fetchAccountEngagement.mockResolvedValue(connected());
    engagement.setAccountEngagement.mockResolvedValue(connected({ enabled: false, state: 'disabled' }));

    renderWithProviders(<TwitchConnectorCard account={ACCOUNT} />);

    const toggle = await screen.findByRole('switch');
    await user.click(toggle);

    expect(confirmSpy).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog');
    expect(engagement.setAccountEngagement).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: /^disable$/i }));

    await waitFor(() =>
      expect(engagement.setAccountEngagement).toHaveBeenCalledWith('acct_twitch_1', { enabled: false }),
    );
  });
});
