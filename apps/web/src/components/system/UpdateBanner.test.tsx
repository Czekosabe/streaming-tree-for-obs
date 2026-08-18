import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as updatesApi from '@/api/updates';
import type { UpdateStatus } from '@/models/updates';
import { renderWithProviders } from '@/test/render';

import { UpdateBanner } from './UpdateBanner';

vi.mock('@/api/updates');

const updates = vi.mocked(updatesApi);

function status(overrides: Partial<UpdateStatus> = {}): UpdateStatus {
  return {
    enabled: true,
    releaseBuild: true,
    currentVersion: '0.1.0',
    autoCheck: true,
    state: 'idle',
    updateAvailable: false,
    installBlocked: false,
    ...overrides,
  };
}

function renderBanner() {
  return renderWithProviders(
    <MemoryRouter>
      <UpdateBanner />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('UpdateBanner', () => {
  it('renders nothing while up to date', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(status({ state: 'up_to_date' }));

    const { container } = renderBanner();

    await waitFor(() => expect(updates.fetchUpdateStatus).toHaveBeenCalled());
    expect(container.textContent).toBe('');
  });

  it('renders nothing in a development build', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(status({ releaseBuild: false, state: 'disabled' }));

    const { container } = renderBanner();

    await waitFor(() => expect(updates.fetchUpdateStatus).toHaveBeenCalled());
    expect(container.textContent).toBe('');
  });

  // Each test below uses its own version string: the banner's "Later"
  // dismissal is module-level, in-memory state that intentionally persists
  // across a page's whole lifetime (docs/updater.md §32) - and therefore
  // also across tests within this same file, so a shared version number
  // would let one test's dismissal leak into another.

  it('shows the available version once an update is found', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(status({ state: 'available', latestVersion: '0.3.1' }));

    renderBanner();

    expect(await screen.findByText(/Streaming Tree 0\.3\.1 is available\./)).toBeInTheDocument();
  });

  it('is never blocking - both Update now and Later remain reachable', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(status({ state: 'available', latestVersion: '0.3.2' }));

    renderBanner();

    expect(await screen.findByRole('link', { name: /update now/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /later/i })).toBeInTheDocument();
  });

  it('hides for that version once Later is clicked', async () => {
    const user = userEvent.setup();
    updates.fetchUpdateStatus.mockResolvedValue(status({ state: 'available', latestVersion: '0.3.3' }));

    renderBanner();

    await screen.findByText(/Streaming Tree 0\.3\.3 is available\./);
    await user.click(screen.getByRole('button', { name: /later/i }));

    expect(screen.queryByText(/Streaming Tree 0\.3\.3 is available\./)).not.toBeInTheDocument();
  });
});
