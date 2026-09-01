import { screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import { renderWithProviders } from '@/test/render';

import { AccountsStep } from './AccountsStep';

vi.mock('@/api/accounts');

function renderStep() {
  return renderWithProviders(
    <MemoryRouter>
      <Routes>
        <Route path="/" element={<AccountsStep />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('AccountsStep', () => {
  it('explains the distinction between a destination and a connected account', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
    renderStep();

    expect(await screen.findByText(/where your video is sent/i)).toBeInTheDocument();
    expect(screen.getByText(/authorizes chat, events/i)).toBeInTheDocument();
  });

  it('allows zero connected accounts - never required to proceed', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
    renderStep();

    expect(await screen.findByText(/no accounts connected yet/i)).toBeInTheDocument();
  });

  it('lists real connected accounts with their real status', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([
      {
        id: 'acc_1', providerId: 'twitch', login: 'streamer', displayName: 'Streamer',
        status: 'connected', scopes: [], createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
      },
      {
        id: 'acc_2', providerId: 'youtube', login: 'other', displayName: 'Other Channel',
        status: 'reconnect_required', scopes: [], createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
      },
    ]);

    renderStep();

    expect(await screen.findByText('Streamer')).toBeInTheDocument();
    expect(screen.getByText('Other Channel')).toBeInTheDocument();
    expect(screen.getByText(/^connected$/i)).toBeInTheDocument();
    expect(screen.getByText(/reconnect needed/i)).toBeInTheDocument();
  });

  it('links to the real account management UI instead of embedding a second OAuth flow', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
    renderStep();

    const manageLink = await screen.findByRole('link', { name: /manage connected accounts/i });
    expect(manageLink).toHaveAttribute('href', '/settings');
  });
});
