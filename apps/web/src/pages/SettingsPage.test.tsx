import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as aboutApi from '@/api/about';
import * as accountsApi from '@/api/accounts';
import * as updatesApi from '@/api/updates';
import { renderWithProviders } from '@/test/render';

import { AboutLegalPage } from './AboutLegalPage';
import { SettingsPage } from './SettingsPage';

vi.mock('@/api/accounts');
vi.mock('@/api/about');
vi.mock('@/api/updates');

const ABOUT_RESPONSE = {
  productName: 'Streaming Tree for OBS',
  version: '0.1.0',
  isReleaseBuild: false,
  creatorName: 'Czekosabe',
  repositoryUrl: 'https://github.com/Czekosabe/streaming-tree-for-obs',
  creatorUrl: 'https://github.com/Czekosabe',
  supportUrl: 'https://streamelements.com/czekosabe/tip',
  applicationLicenseSpdx: 'GPL-3.0-or-later',
  applicationLicenseName: 'GNU General Public License v3 or later',
};

const UPDATE_STATUS_DISABLED = {
  enabled: false,
  releaseBuild: false,
  currentVersion: '0.1.0',
  autoCheck: true,
  state: 'disabled' as const,
  updateAvailable: false,
  installBlocked: false,
};

function renderApp(initialPath: string) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/settings/about" element={<AboutLegalPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
  vi.mocked(accountsApi).fetchIntegrationConfig.mockResolvedValue({
    configured: false,
    source: 'missing',
  });
  vi.mocked(accountsApi).fetchYouTubeIntegrationConfig.mockResolvedValue({
    configured: false,
    source: 'missing',
  });
  vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
  vi.mocked(updatesApi).fetchUpdateStatus.mockResolvedValue(UPDATE_STATUS_DISABLED);
});

describe('SettingsPage', () => {
  it('shows an About & Legal entry that navigates to the About & Legal page', async () => {
    renderApp('/settings');

    const entry = await screen.findByRole('link', { name: /about & legal/i });
    expect(entry).toHaveAttribute('href', '/settings/about');

    await userEvent.click(entry);

    expect(await screen.findByText('Streaming Tree for OBS')).toBeInTheDocument();
    expect(await screen.findByText('Czekosabe')).toBeInTheDocument();
  });
});
