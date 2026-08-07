import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { OperatorChatItem, OperatorChatPreferences } from '@/api/operator-chat-schemas';
import { DEFAULT_OPERATOR_CHAT_PREFERENCES } from '@/api/operator-chat-schemas';
import { renderWithProviders } from '@/test/render';

import { MessageRow } from './MessageRow';

function baseItem(overrides: Partial<OperatorChatItem> = {}): OperatorChatItem {
  return {
    version: 1,
    sequence: 1,
    id: 'chat_twitch_acct_1_msg_1',
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    kind: 'message',
    occurredAt: '2026-08-06T12:00:00Z',
    receivedAt: '2026-08-06T12:00:00Z',
    lifecycle: { deleted: false },
    synthetic: false,
    user: { providerUserId: 'u1', login: 'viewer', displayName: 'Viewer', anonymous: false },
    message: { plainText: 'hello chat', fragments: [{ type: 'text', text: 'hello chat' }] },
    ...overrides,
  };
}

function renderRow(item: OperatorChatItem, preferences: OperatorChatPreferences = DEFAULT_OPERATOR_CHAT_PREFERENCES) {
  return renderWithProviders(
    <ul>
      <MessageRow item={item} preferences={preferences} accountLabel={null} />
    </ul>,
  );
}

describe('MessageRow', () => {
  it('renders the display name and message text', () => {
    renderRow(baseItem());
    expect(screen.getByText('Viewer')).toBeInTheDocument();
    expect(screen.getByText('hello chat')).toBeInTheDocument();
  });

  it('renders an anonymous user without a fabricated identity', () => {
    renderRow(
      baseItem({
        user: { anonymous: true },
        message: { plainText: 'cheer', fragments: [{ type: 'text', text: 'cheer' }] },
      }),
    );
    expect(screen.getByText(/anonymous/i)).toBeInTheDocument();
    expect(screen.queryByText('Viewer')).not.toBeInTheDocument();
  });

  it('renders a text fragment, an emote fragment (as text without a resolvable URL), and a mention fragment in order', () => {
    renderRow(
      baseItem({
        message: {
          plainText: 'hi @Mod Kappa',
          fragments: [
            { type: 'text', text: 'hi ' },
            { type: 'mention', text: '@Mod', mentionLogin: 'mod', mentionDisplayName: 'Mod' },
            { type: 'text', text: ' ' },
            { type: 'emote', text: 'Kappa', emoteId: '25' },
          ],
        },
      }),
    );
    expect(screen.getByText('@Mod')).toBeInTheDocument();
    expect(screen.getByText('Kappa')).toBeInTheDocument();
  });

  it('renders a deleted message with its text preserved and a deleted marker', () => {
    renderRow(
      baseItem({
        lifecycle: { deleted: true, deletionReason: 'moderator_deleted' },
      }),
    );
    expect(screen.getByText('hello chat')).toBeInTheDocument();
    expect(screen.getByText(/deleted/i)).toBeInTheDocument();
  });

  it('shows a command tag for a message starting with the command prefix', () => {
    renderRow(
      baseItem({
        message: { plainText: '!uptime', fragments: [{ type: 'text', text: '!uptime' }] },
      }),
    );
    expect(screen.getByText('command')).toBeInTheDocument();
  });

  it('does not show a command tag for a plain message', () => {
    renderRow(baseItem());
    expect(screen.queryByText('command')).not.toBeInTheDocument();
  });

  it('shows the synthetic marker for a test event', () => {
    renderRow(baseItem({ synthetic: true }));
    expect(screen.getByText('test')).toBeInTheDocument();
  });

  it('preserves a long username and a long message verbatim', () => {
    const longName = 'a'.repeat(120);
    const longMessage = 'b'.repeat(600);
    renderRow(
      baseItem({
        user: { providerUserId: 'u1', displayName: longName, anonymous: false },
        message: { plainText: longMessage, fragments: [{ type: 'text', text: longMessage }] },
      }),
    );
    expect(screen.getByText(longName)).toBeInTheDocument();
    expect(screen.getByText(longMessage)).toBeInTheDocument();
  });

  it('never renders raw HTML from message text', () => {
    renderRow(
      baseItem({
        message: {
          plainText: '<script>alert(1)</script>',
          fragments: [{ type: 'text', text: '<script>alert(1)</script>' }],
        },
      }),
    );
    expect(screen.getByText('<script>alert(1)</script>')).toBeInTheDocument();
    expect(document.querySelector('script')).not.toBeInTheDocument();
  });

  it('calls onHideUser and onMarkBot when their actions are used', () => {
    const onHideUser = vi.fn();
    const onMarkBot = vi.fn();
    renderWithProviders(
      <ul>
        <MessageRow
          item={baseItem()}
          preferences={DEFAULT_OPERATOR_CHAT_PREFERENCES}
          accountLabel={null}
          onHideUser={onHideUser}
          onMarkBot={onMarkBot}
        />
      </ul>,
    );

    screen.getByLabelText('Hide this user').click();
    screen.getByLabelText('Mark as bot').click();
    expect(onHideUser).toHaveBeenCalledTimes(1);
    expect(onMarkBot).toHaveBeenCalledTimes(1);
  });

  it('omits the hide/bot actions when no handler is supplied', () => {
    renderRow(baseItem());
    expect(screen.queryByLabelText('Hide this user')).not.toBeInTheDocument();
  });

  it('calls onReply when the reply action is used', () => {
    const onReply = vi.fn();
    renderWithProviders(
      <ul>
        <MessageRow item={baseItem()} preferences={DEFAULT_OPERATOR_CHAT_PREFERENCES} accountLabel={null} onReply={onReply} />
      </ul>,
    );
    screen.getByLabelText('Reply to Viewer').click();
    expect(onReply).toHaveBeenCalledTimes(1);
  });

  it('omits the reply action when no handler is supplied', () => {
    renderRow(baseItem());
    expect(screen.queryByLabelText(/^Reply to/)).not.toBeInTheDocument();
  });

  it('omits the reply action for a deleted message even when a handler is supplied', () => {
    renderWithProviders(
      <ul>
        <MessageRow
          item={baseItem({ lifecycle: { deleted: true, deletionReason: 'moderator_deleted' } })}
          preferences={DEFAULT_OPERATOR_CHAT_PREFERENCES}
          accountLabel={null}
          onReply={vi.fn()}
        />
      </ul>,
    );
    expect(screen.queryByLabelText(/^Reply to/)).not.toBeInTheDocument();
  });
});
