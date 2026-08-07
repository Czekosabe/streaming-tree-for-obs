import { describe, expect, it } from 'vitest';

import type { OperatorChatItem } from '@/api/operator-chat-schemas';

import { codePointLength, isMessageSendable, MAX_MESSAGE_CODE_POINTS, replyTargetFor } from './outbound-chat';

describe('codePointLength', () => {
  it('counts ordinary ASCII text', () => {
    expect(codePointLength('hello')).toBe(5);
  });

  it('counts an astral-plane emoji as one code point, not two UTF-16 units', () => {
    // A single emoji outside the BMP is two UTF-16 code units in JS but one
    // Unicode code point - matching the backend's own rune-based count.
    expect(codePointLength('🎉')).toBe(1);
    expect('🎉'.length).toBe(2); // sanity check: UTF-16 length differs
  });

  it('counts a multi-code-point flag emoji as multiple code points', () => {
    expect(codePointLength('🇵🇱')).toBe(2);
  });
});

describe('isMessageSendable', () => {
  it('rejects an empty string', () => {
    expect(isMessageSendable('')).toBe(false);
  });

  it('rejects whitespace-only text', () => {
    expect(isMessageSendable('   ')).toBe(false);
  });

  it('accepts ordinary text', () => {
    expect(isMessageSendable('hello chat')).toBe(true);
  });

  it('accepts exactly the maximum length', () => {
    expect(isMessageSendable('a'.repeat(MAX_MESSAGE_CODE_POINTS))).toBe(true);
  });

  it('rejects one code point over the maximum', () => {
    expect(isMessageSendable('a'.repeat(MAX_MESSAGE_CODE_POINTS + 1))).toBe(false);
  });
});

function baseItem(overrides: Partial<OperatorChatItem> = {}): OperatorChatItem {
  return {
    version: 1,
    sequence: 1,
    id: 'chat_1',
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    kind: 'message',
    occurredAt: '2026-08-06T12:00:00Z',
    receivedAt: '2026-08-06T12:00:00Z',
    lifecycle: { deleted: false },
    synthetic: false,
    providerMessageId: 'twitch_msg_1',
    user: { providerUserId: 'u1', displayName: 'Viewer', anonymous: false },
    message: { plainText: 'hello there', fragments: [{ type: 'text', text: 'hello there' }] },
    ...overrides,
  };
}

describe('replyTargetFor', () => {
  it('returns a target for an ordinary Twitch message', () => {
    const target = replyTargetFor(baseItem());
    expect(target).not.toBeNull();
    expect(target?.providerMessageId).toBe('twitch_msg_1');
    expect(target?.accountId).toBe('acct_1');
    expect(target?.authorDisplayName).toBe('Viewer');
  });

  it('returns null for an activity item', () => {
    expect(replyTargetFor(baseItem({ kind: 'activity', message: undefined }))).toBeNull();
  });

  it('returns null for a moderation item', () => {
    expect(replyTargetFor(baseItem({ kind: 'moderation', message: undefined }))).toBeNull();
  });

  it('returns null for a system item', () => {
    expect(replyTargetFor(baseItem({ kind: 'system', message: undefined }))).toBeNull();
  });

  it('returns null for a deleted message', () => {
    expect(replyTargetFor(baseItem({ lifecycle: { deleted: true } }))).toBeNull();
  });

  it('returns null for a non-Twitch item', () => {
    expect(replyTargetFor(baseItem({ providerId: 'youtube' }))).toBeNull();
  });

  it('returns null when providerMessageId is missing', () => {
    expect(replyTargetFor(baseItem({ providerMessageId: undefined }))).toBeNull();
  });

  it('never accepts a provider message id from anywhere but the item itself', () => {
    const target = replyTargetFor(baseItem({ providerMessageId: 'exact_id' }));
    expect(target?.providerMessageId).toBe('exact_id');
  });

  it('truncates a long message for the preview but keeps the full provider message id', () => {
    const longText = 'a'.repeat(200);
    const target = replyTargetFor(
      baseItem({ message: { plainText: longText, fragments: [{ type: 'text', text: longText }] } }),
    );
    expect(target?.preview.length).toBeLessThan(longText.length);
    expect(target?.providerMessageId).toBe('twitch_msg_1');
  });

  it('handles an anonymous user without fabricating a display name', () => {
    const target = replyTargetFor(baseItem({ user: { anonymous: true } }));
    expect(target?.authorDisplayName).toBe('');
  });
});
