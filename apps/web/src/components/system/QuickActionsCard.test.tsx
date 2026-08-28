import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as branchesApi from '@/api/branches';
import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { renderWithProviders } from '@/test/render';

import { QuickActionsCard } from './QuickActionsCard';

vi.mock('@/api/branches');

function configuredPlatform(overrides: Partial<ConfiguredPlatform> = {}): ConfiguredPlatform {
  return {
    id: 'pf_1',
    providerId: 'twitch',
    displayName: 'Main Twitch channel',
    enabled: true,
    sortOrder: 0,
    createdAt: '2026-08-28T00:00:00Z',
    updatedAt: '2026-08-28T00:00:00Z',
    metadata: {
      title: '', description: '', category: '', categoryId: '', tags: [],
      language: '', visibility: '', matureContent: false, dvr: false, latencyMode: '',
      updatedAt: '2026-08-28T00:00:00Z',
    },
    ...overrides,
  };
}

function renderCard(platforms: readonly ConfiguredPlatform[]) {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="/" element={<QuickActionsCard platforms={platforms} />} />
        <Route path="/logs" element={<p>Logs page marker</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(branchesApi).fetchBranches.mockResolvedValue([]);
});

describe('QuickActionsCard', () => {
  it('reuses the same canonical actions StreamsPage exposes - never a second implementation', () => {
    renderCard([configuredPlatform()]);
    expect(screen.getByRole('button', { name: /start enabled destinations/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /stop all outputs/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /refresh status/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /open logs & diagnostics/i })).toBeInTheDocument();
  });

  it('disables "start enabled destinations" when there is nothing configured yet', () => {
    renderCard([]);
    expect(screen.getByRole('button', { name: /start enabled destinations/i })).toBeDisabled();
  });

  it('opens the real StartEnabledConfirmDialog rather than acting immediately', async () => {
    renderCard([configuredPlatform()]);
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: /start enabled destinations/i }));

    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(branchesApi.startEnabledBranches).not.toHaveBeenCalled();
  });

  it('navigates to Logs & Diagnostics rather than opening a second, duplicate logs surface', async () => {
    renderCard([configuredPlatform()]);
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: /open logs & diagnostics/i }));

    expect(await screen.findByText('Logs page marker')).toBeInTheDocument();
  });
});
