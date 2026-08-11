import type { TFunction } from 'i18next';
import { describe, expect, it } from 'vitest';

import type { PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

import { chatItemDataContext } from './chat-item-data-context';

/** Echoes the key instead of a translation - mirrors lib/api-error-message.test.ts's own `echo`. */
const echo = ((key: string) => key) as unknown as TFunction<'overlays'>;

function messageItem(overrides: Partial<PublicChatOverlayItem> = {}): PublicChatOverlayItem {
  return {
    version: 1,
    sequence: 1,
    id: 'm1',
    kind: 'message',
    providerId: 'twitch',
    occurredAt: '2026-08-06T12:00:00Z',
    user: { displayName: 'Ann', color: '#9146FF', avatarUrl: 'https://static-cdn.jtvnw.net/avatar.png', badges: [], anonymous: false },
    message: { plainText: 'hi', fragments: [{ type: 'text', text: 'hi' }] },
    deleted: false,
    synthetic: false,
    ...overrides,
  };
}

describe('chatItemDataContext', () => {
  it('resolves username/message/platform from a normal message item', () => {
    const ctx = chatItemDataContext(messageItem(), echo);
    expect(ctx.bindings.username).toBe('Ann');
    expect(ctx.bindings.message).toBe('hi');
    expect(ctx.bindings.platform).toBe('Twitch');
    expect(ctx.providerId).toBe('twitch');
  });

  it('never fabricates a username for an anonymous user', () => {
    const ctx = chatItemDataContext(messageItem({ user: { anonymous: true } }), echo);
    expect(ctx.bindings.username).toBeNull();
    expect(ctx.avatarUrl).toBeNull();
  });

  it('never carries message content for a deleted message', () => {
    const ctx = chatItemDataContext(messageItem({ deleted: true, message: undefined }), echo);
    expect(ctx.bindings.message).toBeNull();
    expect(ctx.messageFragments).toBeUndefined();
  });

  it('resolves eventType/quantity from an activity item, using the translated activity-type key', () => {
    const item: PublicChatOverlayItem = {
      version: 1, sequence: 1, id: 'a1', kind: 'activity', providerId: 'twitch', occurredAt: '2026-08-06T12:00:00Z',
      activity: { activityType: 'bits', quantity: 250 }, deleted: false, synthetic: false,
    };
    const ctx = chatItemDataContext(item, echo);
    expect(ctx.bindings.eventType).toBe('renderer.activityType.bits');
    expect(ctx.bindings.quantity).toBe(250);
    expect(ctx.bindings.message).toBeNull();
  });

  it('falls back to the raw activity type for an unrecognized one', () => {
    const item: PublicChatOverlayItem = {
      version: 1, sequence: 1, id: 'a2', kind: 'activity', providerId: 'twitch', occurredAt: '2026-08-06T12:00:00Z',
      activity: { activityType: 'mystery_event' }, deleted: false, synthetic: false,
    };
    expect(chatItemDataContext(item, echo).bindings.eventType).toBe('mystery_event');
  });

  it('carries account label when present, null when absent', () => {
    expect(chatItemDataContext(messageItem({ accountLabel: 'Main Channel' }), echo).bindings.accountLabel).toBe('Main Channel');
    expect(chatItemDataContext(messageItem({ accountLabel: undefined }), echo).bindings.accountLabel).toBeNull();
  });

  it('always reports groupCount 1 - chat items are never grouped', () => {
    expect(chatItemDataContext(messageItem(), echo).bindings.groupCount).toBe(1);
  });

  it('never fabricates renderedText - always null for chat', () => {
    expect(chatItemDataContext(messageItem(), echo).bindings.renderedText).toBeNull();
  });

  it('threads message fragments and badges through for a normal message', () => {
    const ctx = chatItemDataContext(
      messageItem({ user: { ...messageItem().user!, badges: [{ setId: 'moderator', id: '1' }] } }),
      echo,
    );
    expect(ctx.messageFragments).toEqual([{ type: 'text', text: 'hi' }]);
    expect(ctx.badges).toEqual([{ setId: 'moderator', id: '1' }]);
  });
});
