import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConnectedAccount, IntegrationConfig } from '@/api/account-schemas';
import * as accountsApi from '@/api/accounts';
import { renderWithProviders } from '@/test/render';

import { YouTubeAccountsPanel } from './YouTubeAccountsPanel';

vi.mock('@/api/accounts');

const accounts = vi.mocked(accountsApi);

const CONFIG: IntegrationConfig = { configured: true, source: 'database', clientId: 'abc123' };

const ACCOUNT: ConnectedAccount = {
  id: 'acct_yt_1',
  providerId: 'youtube',
  login: 'My Channel',
  displayName: 'My Channel',
  status: 'connected',
  scopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
  createdAt: '2026-08-05T00:00:00Z',
  updatedAt: '2026-08-05T00:00:00Z',
};

// See TwitchDeviceFlowModal.test.tsx: restoreMocks does not clear call
// history on this automocked module between tests in the same file.
beforeEach(() => {
  vi.clearAllMocks();
});

describe('YouTubeAccountsPanel disconnect flow', () => {
  it('never disconnects via window.confirm and requires the application dialog to confirm', async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, 'confirm');
    accounts.fetchYouTubeIntegrationConfig.mockResolvedValue(CONFIG);
    accounts.fetchAccounts.mockResolvedValue([ACCOUNT]);
    accounts.disconnectAccount.mockResolvedValue(undefined);

    renderWithProviders(<YouTubeAccountsPanel />);

    await screen.findByText('My Channel');
    await user.click(screen.getByRole('button', { name: /disconnect/i }));

    const dialog = await screen.findByRole('dialog', { name: /disconnect my channel/i });
    expect(confirmSpy).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: /^disconnect$/i }));

    await waitFor(() => expect(accounts.disconnectAccount.mock.calls[0]?.[0]).toBe('acct_yt_1'));
    expect(confirmSpy).not.toHaveBeenCalled();
  });

  it('does not disconnect when the confirmation is cancelled', async () => {
    const user = userEvent.setup();
    accounts.fetchYouTubeIntegrationConfig.mockResolvedValue(CONFIG);
    accounts.fetchAccounts.mockResolvedValue([ACCOUNT]);
    accounts.disconnectAccount.mockResolvedValue(undefined);

    renderWithProviders(<YouTubeAccountsPanel />);

    await screen.findByText('My Channel');
    await user.click(screen.getByRole('button', { name: /disconnect/i }));

    const dialog = await screen.findByRole('dialog', { name: /disconnect my channel/i });
    await user.click(within(dialog).getByRole('button', { name: /cancel/i }));

    expect(accounts.disconnectAccount).not.toHaveBeenCalled();
  });

  it('never renders the Twitch panel heading or a Twitch account row', async () => {
    accounts.fetchYouTubeIntegrationConfig.mockResolvedValue(CONFIG);
    accounts.fetchAccounts.mockResolvedValue([ACCOUNT]);

    renderWithProviders(<YouTubeAccountsPanel />);

    await screen.findByText('My Channel');
    expect(screen.queryByText(/twitch integration/i)).not.toBeInTheDocument();
  });

  it('disables Connect YouTube until a Client ID is configured', async () => {
    accounts.fetchYouTubeIntegrationConfig.mockResolvedValue({ configured: false, source: 'missing' });
    accounts.fetchAccounts.mockResolvedValue([]);

    renderWithProviders(<YouTubeAccountsPanel />);

    const connectButton = await screen.findByRole('button', { name: /connect youtube/i });
    expect(connectButton).toBeDisabled();
  });
});
