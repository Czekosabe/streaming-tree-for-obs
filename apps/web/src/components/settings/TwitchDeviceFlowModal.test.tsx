import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { DeviceFlowSnapshot } from '@/api/account-schemas';
import * as accountsApi from '@/api/accounts';
import { renderWithProviders } from '@/test/render';

import { TwitchDeviceFlowModal } from './TwitchDeviceFlowModal';

vi.mock('@/api/accounts');

const accounts = vi.mocked(accountsApi);

/**
 * Vitest's `restoreMocks` config option does not clear call history on the
 * `vi.fn()` instances an automocked module (`vi.mock('@/api/accounts')`)
 * exports between tests in the same file - only `vi.clearAllMocks()` does,
 * so it is called explicitly rather than relying on that global setting.
 */
beforeEach(() => {
  vi.clearAllMocks();
});

const BASE_SNAPSHOT: DeviceFlowSnapshot = {
  attemptId: 'attempt_1',
  providerId: 'twitch',
  state: 'waiting_for_user',
  userCode: 'ABCD-EFGH',
  verificationUri: 'https://www.twitch.tv/activate',
  createdAt: '2026-08-04T12:00:00Z',
  expiresAt: new Date(Date.now() + 10 * 60_000).toISOString(),
  intervalSeconds: 5,
};

function snapshot(overrides: Partial<DeviceFlowSnapshot>): DeviceFlowSnapshot {
  return { ...BASE_SNAPSHOT, ...overrides };
}

describe('TwitchDeviceFlowModal', () => {
  it('starts an attempt on open and displays the user code, never an arbitrary extra field', async () => {
    accounts.startDeviceFlow.mockResolvedValue(snapshot({}));
    // A hypothetical backend response carrying a device code: proves the
    // component only ever renders fields its schema declares, not whatever
    // JSON happens to arrive.
    accounts.fetchDeviceFlow.mockResolvedValue({
      ...snapshot({}),
      deviceCode: 'devc_should_never_render',
    } as unknown as DeviceFlowSnapshot);

    renderWithProviders(<TwitchDeviceFlowModal open onClose={vi.fn()} />);

    expect(await screen.findByText('ABCD-EFGH')).toBeInTheDocument();
    expect(accounts.startDeviceFlow).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/devc_should_never_render/)).not.toBeInTheDocument();
  });

  it('shows copy feedback after the user code is copied', async () => {
    const user = userEvent.setup();
    accounts.startDeviceFlow.mockResolvedValue(snapshot({}));
    accounts.fetchDeviceFlow.mockResolvedValue(snapshot({}));

    // `userEvent.setup()` installs its own navigator.clipboard stub, so this
    // must be defined after that call or it gets overwritten.
    const writeTextMock = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextMock },
      configurable: true,
    });

    renderWithProviders(<TwitchDeviceFlowModal open onClose={vi.fn()} />);
    await screen.findByText('ABCD-EFGH');

    await user.click(screen.getByRole('button', { name: /copy code/i }));

    expect(writeTextMock).toHaveBeenCalledWith('ABCD-EFGH');
    await waitFor(() => expect(screen.getByText('Copied')).toBeInTheDocument());
  });

  it('shows a pending authorization state while waiting for the user', async () => {
    accounts.startDeviceFlow.mockResolvedValue(snapshot({ state: 'waiting_for_user' }));
    accounts.fetchDeviceFlow.mockResolvedValue(snapshot({ state: 'waiting_for_user' }));

    renderWithProviders(<TwitchDeviceFlowModal open onClose={vi.fn()} />);

    expect(await screen.findByText(/waiting for you to authorize on twitch/i)).toBeInTheDocument();
  });

  it('cancels the active attempt and closes when dismissed before completion', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    accounts.startDeviceFlow.mockResolvedValue(snapshot({ state: 'waiting_for_user' }));
    accounts.fetchDeviceFlow.mockResolvedValue(snapshot({ state: 'waiting_for_user' }));
    accounts.cancelDeviceFlow.mockResolvedValue(snapshot({ state: 'cancelled' }));

    renderWithProviders(<TwitchDeviceFlowModal open onClose={onClose} />);
    await screen.findByText('ABCD-EFGH');

    await user.click(screen.getByRole('button', { name: /^cancel$/i }));

    expect(accounts.cancelDeviceFlow.mock.calls[0]?.[0]).toBe('attempt_1');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('offers only Close, not Cancel, once the code has expired', async () => {
    accounts.startDeviceFlow.mockResolvedValue(snapshot({ state: 'expired', userCode: undefined }));
    accounts.fetchDeviceFlow.mockResolvedValue(snapshot({ state: 'expired', userCode: undefined }));

    renderWithProviders(<TwitchDeviceFlowModal open onClose={vi.fn()} />);

    const dialog = await screen.findByRole('dialog');
    const footer = await waitFor(() => {
      const element = dialog.querySelector('footer');
      if (element === null) throw new Error('expected a footer to render');
      return element;
    });
    const footerButton = within(footer).getByRole('button');
    expect(footerButton).toHaveTextContent(/close/i);
    expect(within(footer).queryByRole('button', { name: /^cancel$/i })).not.toBeInTheDocument();
  });

  it('calls onAuthorized once the attempt reaches the authorized state', async () => {
    const onAuthorized = vi.fn();
    accounts.startDeviceFlow.mockResolvedValue(
      snapshot({ state: 'authorized', connectedAccountId: 'acct_1' }),
    );
    accounts.fetchDeviceFlow.mockResolvedValue(
      snapshot({ state: 'authorized', connectedAccountId: 'acct_1' }),
    );

    renderWithProviders(<TwitchDeviceFlowModal open onClose={vi.fn()} onAuthorized={onAuthorized} />);

    await waitFor(() => expect(onAuthorized).toHaveBeenCalledTimes(1));
  });

  it('never starts a second attempt while the modal stays open', async () => {
    accounts.startDeviceFlow.mockResolvedValue(snapshot({}));
    accounts.fetchDeviceFlow.mockResolvedValue(snapshot({}));

    const { rerender } = renderWithProviders(<TwitchDeviceFlowModal open onClose={vi.fn()} />);
    await screen.findByText('ABCD-EFGH');
    const callsAfterOpen = accounts.startDeviceFlow.mock.calls.length;

    rerender(<TwitchDeviceFlowModal open onClose={vi.fn()} />);

    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(accounts.startDeviceFlow.mock.calls.length).toBe(callsAfterOpen);
  });
});
