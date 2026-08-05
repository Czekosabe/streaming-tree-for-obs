import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { OAuthAttemptSnapshot } from '@/api/account-schemas';
import * as accountsApi from '@/api/accounts';
import { renderWithProviders } from '@/test/render';

import { YouTubeOAuthModal } from './YouTubeOAuthModal';

vi.mock('@/api/accounts');

const accounts = vi.mocked(accountsApi);

beforeEach(() => {
  vi.clearAllMocks();
});

const BASE_SNAPSHOT: OAuthAttemptSnapshot = {
  attemptId: 'ytauth_1',
  providerId: 'youtube',
  state: 'waiting_for_browser',
  authorizationUrl: 'https://accounts.google.com/o/oauth2/v2/auth?client_id=cid&state=fake-state-value',
  createdAt: '2026-08-05T12:00:00Z',
  expiresAt: new Date(Date.now() + 10 * 60_000).toISOString(),
};

function snapshot(overrides: Partial<OAuthAttemptSnapshot>): OAuthAttemptSnapshot {
  return { ...BASE_SNAPSHOT, ...overrides };
}

describe('YouTubeOAuthModal', () => {
  it('starts an attempt on open and offers an explicit "Open Google" action, never a raw code/state field', async () => {
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(snapshot({}));
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(snapshot({}));

    renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} />);

    const link = await screen.findByRole('link', { name: /open google authorization/i });
    expect(link).toHaveAttribute('href', BASE_SNAPSHOT.authorizationUrl);
    expect(accounts.startYouTubeOAuthAttempt).toHaveBeenCalledTimes(1);

    // The public schema has no code/state/verifier field to begin with, but
    // this also proves no such value is ever rendered as visible text.
    expect(screen.queryByText(/fake-state-value/)).not.toBeInTheDocument();
  });

  it('never opens a popup automatically - the authorization link requires an explicit click and opens in a new tab', async () => {
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(snapshot({}));
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(snapshot({}));

    renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} />);

    const link = await screen.findByRole('link', { name: /open google authorization/i });
    // target="_blank" + rel="noreferrer noopener": a real anchor a user must
    // click, not a window.open() call fired on mount.
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
  });

  it('shows a waiting state while the browser flow is in progress', async () => {
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'waiting_for_browser' }));
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'waiting_for_browser' }));

    renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} />);

    expect(await screen.findByText(/waiting for you to finish signing in with google/i)).toBeInTheDocument();
  });

  it('cancels the active attempt and closes when dismissed before completion', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'waiting_for_browser' }));
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'waiting_for_browser' }));
    accounts.cancelYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'cancelled' }));

    renderWithProviders(<YouTubeOAuthModal open onClose={onClose} />);
    await screen.findByRole('link', { name: /open google authorization/i });

    await user.click(screen.getByRole('button', { name: /^cancel$/i }));

    expect(accounts.cancelYouTubeOAuthAttempt.mock.calls[0]?.[0]).toBe('ytauth_1');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('offers only Close, not Cancel, once the attempt has expired', async () => {
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'expired', authorizationUrl: undefined }));
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'expired', authorizationUrl: undefined }));

    renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} />);

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

  it('shows channel selection when more than one channel is offered, and connects the one clicked', async () => {
    const user = userEvent.setup();
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(
      snapshot({
        state: 'awaiting_channel_selection',
        authorizationUrl: undefined,
        channels: [
          { channelId: 'UC1', title: 'First Channel' },
          { channelId: 'UC2', title: 'Second Channel' },
        ],
      }),
    );
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(
      snapshot({
        state: 'awaiting_channel_selection',
        authorizationUrl: undefined,
        channels: [
          { channelId: 'UC1', title: 'First Channel' },
          { channelId: 'UC2', title: 'Second Channel' },
        ],
      }),
    );
    accounts.selectYouTubeChannel.mockResolvedValue(
      snapshot({ state: 'authorized', connectedAccountId: 'acct_yt_1' }),
    );

    renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} />);

    expect(await screen.findByText('First Channel')).toBeInTheDocument();
    expect(screen.getByText('Second Channel')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /second channel/i }));

    await waitFor(() =>
      expect(accounts.selectYouTubeChannel).toHaveBeenCalledWith('ytauth_1', 'UC2'),
    );
  });

  it('calls onAuthorized once the attempt reaches the authorized state', async () => {
    const onAuthorized = vi.fn();
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(
      snapshot({ state: 'authorized', connectedAccountId: 'acct_1', authorizationUrl: undefined }),
    );
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(
      snapshot({ state: 'authorized', connectedAccountId: 'acct_1', authorizationUrl: undefined }),
    );

    renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} onAuthorized={onAuthorized} />);

    await waitFor(() => expect(onAuthorized).toHaveBeenCalledTimes(1));
  });

  it('never starts a second attempt while the modal stays open', async () => {
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(snapshot({}));
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(snapshot({}));

    const { rerender } = renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} />);
    await screen.findByRole('link', { name: /open google authorization/i });
    const callsAfterOpen = accounts.startYouTubeOAuthAttempt.mock.calls.length;

    rerender(<YouTubeOAuthModal open onClose={vi.fn()} />);

    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(accounts.startYouTubeOAuthAttempt.mock.calls.length).toBe(callsAfterOpen);
  });

  it('shows a denial state distinctly from expiration', async () => {
    accounts.startYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'denied', authorizationUrl: undefined }));
    accounts.fetchYouTubeOAuthAttempt.mockResolvedValue(snapshot({ state: 'denied', authorizationUrl: undefined }));

    renderWithProviders(<YouTubeOAuthModal open onClose={vi.fn()} />);

    expect(await screen.findByText(/authorization was denied on google/i)).toBeInTheDocument();
  });
});
