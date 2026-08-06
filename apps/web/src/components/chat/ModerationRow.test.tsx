import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { OperatorChatItem } from '@/api/operator-chat-schemas';
import { DEFAULT_OPERATOR_CHAT_PREFERENCES } from '@/api/operator-chat-schemas';
import { renderWithProviders } from '@/test/render';

import { ModerationRow } from './ModerationRow';

function moderationItem(action: string): OperatorChatItem {
  return {
    version: 1,
    sequence: 1,
    id: `mod_${action}`,
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    kind: 'moderation',
    occurredAt: '2026-08-06T12:00:00Z',
    receivedAt: '2026-08-06T12:00:00Z',
    lifecycle: { deleted: false },
    synthetic: false,
    moderation: { action },
  };
}

function renderRow(item: OperatorChatItem) {
  return renderWithProviders(
    <ul>
      <ModerationRow item={item} preferences={DEFAULT_OPERATOR_CHAT_PREFERENCES} />
    </ul>,
  );
}

describe('ModerationRow', () => {
  it('renders a chat-cleared row', () => {
    renderRow(moderationItem('chat_cleared'));
    expect(screen.getByText('Chat was cleared')).toBeInTheDocument();
  });

  it('renders a user-messages-cleared row', () => {
    renderRow(moderationItem('user_messages_cleared'));
    expect(screen.getByText("A user's messages were cleared")).toBeInTheDocument();
  });

  it('renders a message-deleted-not-retained row without inventing content', () => {
    renderRow(moderationItem('message_deleted_not_retained'));
    expect(screen.getByText('A message was deleted (no longer retained)')).toBeInTheDocument();
  });

  it('falls back to the raw action for an unrecognized action rather than crashing', () => {
    renderRow(moderationItem('some_future_action'));
    expect(screen.getByText('some_future_action')).toBeInTheDocument();
  });
});
