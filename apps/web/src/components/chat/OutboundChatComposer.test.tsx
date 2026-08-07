import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as engagementApi from '@/api/engagement';
import * as outboundChatApi from '@/api/outbound-chat';
import { ApiError } from '@/lib/api-client';
import { renderWithProviders } from '@/test/render';

import { OutboundChatComposer } from './OutboundChatComposer';

vi.mock('@/api/outbound-chat');
vi.mock('@/api/engagement');

const account1 = {
  id: 'acct_1', providerId: 'twitch', login: 'streamer', displayName: 'Streamer',
  status: 'connected' as const, scopes: [], createdAt: '2026-08-06T00:00:00Z', updatedAt: '2026-08-06T00:00:00Z',
};
const account2 = {
  id: 'acct_2', providerId: 'twitch', login: 'second', displayName: 'Second Account',
  status: 'connected' as const, scopes: [], createdAt: '2026-08-06T00:00:00Z', updatedAt: '2026-08-06T00:00:00Z',
};

function readyStatus(overrides: Record<string, unknown> = {}) {
  return {
    providerId: 'twitch', capability: 'ready' as const, dispatcherState: 'idle' as const,
    queueDepth: 0, queueCapacity: 20, canSendNow: true,
    sharedChatWarning: 'twitch_shared_chat_distribution_possible',
    ...overrides,
  };
}

function connectedEngagement() {
  return { accountId: 'acct_1', enabled: true, state: 'connected' as const, reconnectCount: 0, activeSubscriptionCount: 13, expectedSubscriptionCount: 13, requiredScopes: [], grantedScopes: [], permissionUpgradeRequired: false };
}

beforeEach(() => {
  vi.mocked(engagementApi).fetchAccountEngagement.mockResolvedValue(connectedEngagement());
});

describe('OutboundChatComposer', () => {
  it('renders nothing when there is no connected Twitch account', () => {
    const { container } = renderWithProviders(
      <OutboundChatComposer twitchAccounts={[]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    expect(container.querySelector('[data-testid="outbound-chat-composer"]')).toBeNull();
  });

  it('shows the permission-required card with an authorize action', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(
      readyStatus({ capability: 'permission_required', canSendNow: false }),
    );
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    expect(await screen.findByRole('button', { name: /grant outbound chat permission/i })).toBeInTheDocument();
  });

  it('starts the authorize mutation and shows the user code once accepted', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(
      readyStatus({ capability: 'permission_required', canSendNow: false }),
    );
    vi.mocked(outboundChatApi).authorizeOutboundChat.mockResolvedValue({
      attemptId: 'attempt_1', providerId: 'twitch', state: 'polling', userCode: 'ABCD-EFGH',
      verificationUri: 'https://twitch.tv/activate', createdAt: '2026-08-07T00:00:00Z',
    });
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    await user.click(await screen.findByRole('button', { name: /grant outbound chat permission/i }));
    expect(await screen.findByText(/ABCD-EFGH/)).toBeInTheDocument();
  });

  it('shows a healthy composer with a live character counter', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    const textarea = await screen.findByLabelText(/message/i);
    await waitFor(() => expect(textarea).toBeEnabled());
    expect(textarea).toBeEnabled();
    await user.type(textarea, 'hi');
    expect(screen.getByText('2/500')).toBeInTheDocument();
  });

  it('disables the send button for empty input', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    await screen.findByLabelText(/message/i);
    expect(screen.getByRole('button', { name: /^send$/i })).toBeDisabled();
  });

  it('sends a message and clears the input on success', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    vi.mocked(outboundChatApi).sendOutboundChatMessage.mockResolvedValue({
      sent: true, providerMessageId: 'm1', sentAt: '2026-08-07T00:00:00Z',
    });
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    const textarea = await screen.findByLabelText(/message/i);
    await waitFor(() => expect(textarea).toBeEnabled());
    await user.type(textarea, 'hello chat');
    await user.click(screen.getByRole('button', { name: /^send$/i }));

    await waitFor(() => expect(outboundChatApi.sendOutboundChatMessage).toHaveBeenCalledWith('acct_1', { message: 'hello chat' }));
    await waitFor(() => expect(textarea).toHaveValue(''));
  });

  it('shows a dropped-message explanation without echoing the sent text', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    vi.mocked(outboundChatApi).sendOutboundChatMessage.mockRejectedValue(
      new ApiError('http', 'dropped', { status: 422, code: 'outbound_chat_message_dropped' }),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    const textarea = await screen.findByLabelText(/message/i);
    await waitFor(() => expect(textarea).toBeEnabled());
    await user.type(textarea, 'spammy text');
    await user.click(screen.getByRole('button', { name: /^send$/i }));

    expect(await screen.findByText(/did not deliver/i)).toBeInTheDocument();
    // The message is preserved for the operator to edit/retry, per the
    // stage's own requirement - never silently cleared on a drop.
    expect(textarea).toHaveValue('spammy text');
  });

  it('shows a rate-limited explanation on a rate-limited send', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    vi.mocked(outboundChatApi).sendOutboundChatMessage.mockRejectedValue(
      new ApiError('http', 'rate limited', { status: 429, code: 'outbound_chat_rate_limited' }),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    const textarea = await screen.findByLabelText(/message/i);
    await waitFor(() => expect(textarea).toBeEnabled());
    await user.type(textarea, 'fast fast fast');
    await user.click(screen.getByRole('button', { name: /^send$/i }));
    expect(await screen.findByText(/rate limited/i)).toBeInTheDocument();
  });

  it('shows an uncertain-delivery explanation', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    vi.mocked(outboundChatApi).sendOutboundChatMessage.mockRejectedValue(
      new ApiError('http', 'unknown', { status: 502, code: 'outbound_chat_delivery_unknown' }),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    const textarea = await screen.findByLabelText(/message/i);
    await waitFor(() => expect(textarea).toBeEnabled());
    await user.type(textarea, 'uncertain');
    await user.click(screen.getByRole('button', { name: /^send$/i }));
    expect(await screen.findByText(/may or may not have been delivered/i)).toBeInTheDocument();
  });

  it('shows a backend-unavailable state when the status query fails', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockRejectedValue(new Error('network down'));
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    expect(await screen.findByText(/unavailable/i)).toBeInTheDocument();
  });

  it('always shows the Shared Chat warning while ready, never claiming a session is active', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    const warning = await screen.findByRole('note');
    expect(warning).toHaveTextContent(/may distribute/i);
    expect(warning).not.toHaveTextContent(/is currently active/i);
  });

  it('shows an account selector when more than one Twitch account exists', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    renderWithProviders(
      <OutboundChatComposer
        twitchAccounts={[account1, account2]}
        replyTarget={null}
        onCancelReply={() => {}}
        onReplySent={() => {}}
      />,
    );
    expect(await screen.findByLabelText(/send from/i)).toBeInTheDocument();
  });

  it('explains that no local echo is expected when inbound engagement is disabled', async () => {
    vi.mocked(engagementApi).fetchAccountEngagement.mockResolvedValue({
      accountId: 'acct_1', enabled: false, state: 'disabled' as const, reconnectCount: 0,
      activeSubscriptionCount: 0, expectedSubscriptionCount: 0, requiredScopes: [], grantedScopes: [], permissionUpgradeRequired: false,
    });
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    renderWithProviders(
      <OutboundChatComposer twitchAccounts={[account1]} replyTarget={null} onCancelReply={() => {}} onReplySent={() => {}} />,
    );
    expect(await screen.findByText(/no local echo/i)).toBeInTheDocument();
  });

  it('shows a reply preview, locks the account selector, and forwards replyParentMessageId', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    vi.mocked(outboundChatApi).sendOutboundChatMessage.mockResolvedValue({ sent: true, providerMessageId: 'm2' });
    const onReplySent = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer
        twitchAccounts={[account1, account2]}
        replyTarget={{ accountId: 'acct_1', providerMessageId: 'parent_1', authorDisplayName: 'Viewer', preview: 'original text' }}
        onCancelReply={() => {}}
        onReplySent={onReplySent}
      />,
    );
    expect(await screen.findByText(/replying to viewer/i)).toBeInTheDocument();
    expect(screen.getByText('original text')).toBeInTheDocument();
    expect(screen.getByLabelText(/send from/i)).toBeDisabled();

    const textarea = screen.getByLabelText(/message/i);
    await waitFor(() => expect(textarea).toBeEnabled());
    await user.type(textarea, 'a reply');
    await user.click(screen.getByRole('button', { name: /^send$/i }));

    await waitFor(() =>
      expect(outboundChatApi.sendOutboundChatMessage).toHaveBeenCalledWith('acct_1', {
        message: 'a reply', replyParentMessageId: 'parent_1',
      }),
    );
    await waitFor(() => expect(onReplySent).toHaveBeenCalled());
  });

  it('calls onCancelReply when the cancel button is clicked', async () => {
    vi.mocked(outboundChatApi).fetchOutboundChatStatus.mockResolvedValue(readyStatus());
    const onCancelReply = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(
      <OutboundChatComposer
        twitchAccounts={[account1]}
        replyTarget={{ accountId: 'acct_1', providerMessageId: 'parent_1', authorDisplayName: 'Viewer', preview: 'x' }}
        onCancelReply={onCancelReply}
        onReplySent={() => {}}
      />,
    );
    await user.click(await screen.findByLabelText(/cancel reply/i));
    expect(onCancelReply).toHaveBeenCalled();
  });
});
