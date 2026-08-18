import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as updatesApi from '@/api/updates';
import type { UpdateStatus } from '@/models/updates';
import { renderWithProviders } from '@/test/render';

import { UpdatesPanel } from './UpdatesPanel';

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

beforeEach(() => {
  vi.clearAllMocks();
});

describe('UpdatesPanel', () => {
  it('shows the honest development-build notice and no controls in a non-release build', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(status({ releaseBuild: false, state: 'disabled' }));

    renderWithProviders(<UpdatesPanel />);

    expect(
      await screen.findByText('Updates are available in packaged release builds.'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /check for updates/i })).not.toBeInTheDocument();
  });

  it('shows the honest platform-unsupported notice and no check button on a platform with no install path', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(
      status({ state: 'platform_unsupported', currentVersion: '0.2.0', installBlocked: true }),
    );

    renderWithProviders(<UpdatesPanel />);

    expect(await screen.findByText('0.2.0')).toBeInTheDocument();
    expect(
      await screen.findByText(
        'Automatic updates are not yet available on this platform. You can always download the latest release manually from GitHub.',
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /check for updates/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('switch', { name: /automatically check for updates/i })).not.toBeInTheDocument();
  });

  it('shows the current version and up-to-date state in a release build', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(status({ state: 'up_to_date', currentVersion: '0.2.0' }));

    renderWithProviders(<UpdatesPanel />);

    expect(await screen.findByText('0.2.0')).toBeInTheDocument();
    expect(await screen.findByText("You're up to date.")).toBeInTheDocument();
  });

  it('shows an available update with its version and release notes', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(
      status({
        state: 'available',
        latestVersion: '0.3.0',
        releaseNotes: 'Bug fixes and improvements.',
      }),
    );

    renderWithProviders(<UpdatesPanel />);

    expect(await screen.findByText('Version 0.3.0')).toBeInTheDocument();
    expect(await screen.findByText('Bug fixes and improvements.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /update now/i })).toBeInTheDocument();
  });

  it('disables Install and restart and shows the real reason while a stream is active', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(
      status({
        state: 'ready_to_install',
        latestVersion: '0.3.0',
        installBlocked: true,
        blockerCode: 'install_blocked_streaming_active',
      }),
    );

    renderWithProviders(<UpdatesPanel />);

    const installButton = await screen.findByRole('button', { name: /install and restart/i });
    expect(installButton).toBeDisabled();
    expect(
      await screen.findByText('Stop the active stream before installing the update.'),
    ).toBeInTheDocument();
  });

  it('enables Install and restart once nothing blocks it, and requires confirmation', async () => {
    const user = userEvent.setup();
    updates.fetchUpdateStatus.mockResolvedValue(
      status({ state: 'ready_to_install', latestVersion: '0.3.0', installBlocked: false }),
    );
    updates.installUpdate.mockResolvedValue(undefined);

    renderWithProviders(<UpdatesPanel />);

    const installButton = await screen.findByRole('button', { name: /install and restart/i });
    expect(installButton).toBeEnabled();

    await user.click(installButton);
    const dialog = await screen.findByRole('dialog', { name: /install update and restart/i });
    expect(updates.installUpdate).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: /install and restart/i }));

    await waitFor(() => expect(updates.installUpdate).toHaveBeenCalledTimes(1));
  });

  it('toggles the automatic-check preference', async () => {
    const user = userEvent.setup();
    updates.fetchUpdateStatus.mockResolvedValue(status({ autoCheck: true }));
    updates.setAutoCheckPreference.mockResolvedValue(status({ autoCheck: false }));

    renderWithProviders(<UpdatesPanel />);

    const toggle = await screen.findByRole('switch', { name: /automatically check for updates/i });
    await user.click(toggle);

    await waitFor(() => expect(updates.setAutoCheckPreference).toHaveBeenCalledWith(false));
  });

  it('shows the one-shot post-update success message', async () => {
    updates.fetchUpdateStatus.mockResolvedValue(
      status({ postUpdateOutcome: 'ok', postUpdateToVersion: '0.2.0' }),
    );

    renderWithProviders(<UpdatesPanel />);

    expect(await screen.findByText('Streaming Tree was updated to 0.2.0.')).toBeInTheDocument();
  });
});
