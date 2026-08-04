import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConnectedAccount, IntegrationConfig } from '@/api/account-schemas';
import * as accountsApi from '@/api/accounts';
import { renderWithProviders } from '@/test/render';

import { ConnectedAccountsPanel } from './ConnectedAccountsPanel';

vi.mock('@/api/accounts');

const accounts = vi.mocked(accountsApi);

const CONFIG: IntegrationConfig = { configured: true, source: 'database', clientId: 'abc123' };

const ACCOUNT: ConnectedAccount = {
  id: 'acct_1',
  providerId: 'twitch',
  login: 'streamer',
  displayName: 'Streamer',
  status: 'connected',
  scopes: ['channel:manage:broadcast'],
  createdAt: '2026-08-01T00:00:00Z',
  updatedAt: '2026-08-01T00:00:00Z',
};

// See TwitchDeviceFlowModal.test.tsx: restoreMocks does not clear call
// history on this automocked module between tests in the same file.
beforeEach(() => {
  vi.clearAllMocks();
});

describe('ConnectedAccountsPanel disconnect flow', () => {
  it('never disconnects via window.confirm and requires the application dialog to confirm', async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, 'confirm');
    accounts.fetchIntegrationConfig.mockResolvedValue(CONFIG);
    accounts.fetchAccounts.mockResolvedValue([ACCOUNT]);
    accounts.disconnectAccount.mockResolvedValue(undefined);

    renderWithProviders(<ConnectedAccountsPanel />);

    await screen.findByText('Streamer');
    await user.click(screen.getByRole('button', { name: /disconnect/i }));

    const dialog = await screen.findByRole('dialog', { name: /disconnect streamer/i });
    expect(confirmSpy).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: /^disconnect$/i }));

    await waitFor(() => expect(accounts.disconnectAccount.mock.calls[0]?.[0]).toBe('acct_1'));
    expect(confirmSpy).not.toHaveBeenCalled();
  });

  it('does not disconnect when the confirmation is cancelled', async () => {
    const user = userEvent.setup();
    accounts.fetchIntegrationConfig.mockResolvedValue(CONFIG);
    accounts.fetchAccounts.mockResolvedValue([ACCOUNT]);
    accounts.disconnectAccount.mockResolvedValue(undefined);

    renderWithProviders(<ConnectedAccountsPanel />);

    await screen.findByText('Streamer');
    await user.click(screen.getByRole('button', { name: /disconnect/i }));

    const dialog = await screen.findByRole('dialog', { name: /disconnect streamer/i });
    await user.click(within(dialog).getByRole('button', { name: /cancel/i }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(accounts.disconnectAccount).not.toHaveBeenCalled();
  });

  it('never renders a token, refresh token or device code anywhere in the panel', async () => {
    accounts.fetchIntegrationConfig.mockResolvedValue(CONFIG);
    accounts.fetchAccounts.mockResolvedValue([ACCOUNT]);

    const { container } = renderWithProviders(<ConnectedAccountsPanel />);

    await screen.findByText('Streamer');
    expect(container.textContent ?? '').not.toMatch(/access_token|refresh_token|device_code|bearer /i);
  });
});
