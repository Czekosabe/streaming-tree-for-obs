import { describe, expect, it } from 'vitest';

import type { PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

import {
  chatOverlayItemsInOrder,
  chatOverlayReducer,
  createChatOverlayState,
  type ChatOverlayState,
} from './chat-overlay-reducer';

function item(id: string, sequence: number, overrides: Partial<PublicChatOverlayItem> = {}): PublicChatOverlayItem {
  return {
    version: 1,
    sequence,
    id,
    kind: 'message',
    providerId: 'twitch',
    occurredAt: '2026-08-06T12:00:00Z',
    user: { anonymous: false, displayName: 'Viewer' },
    message: { plainText: 'hi', fragments: [{ type: 'text', text: 'hi' }] },
    deleted: false,
    synthetic: false,
    ...overrides,
  };
}

function order(state: ChatOverlayState): string[] {
  return chatOverlayItemsInOrder(state).map((i) => i.id);
}

describe('chatOverlayReducer', () => {
  it('upserts a new item, appended in first-seen order', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('b', 2) });
    expect(order(state)).toEqual(['a', 'b']);
  });

  it('a duplicate upsert for the same id (equal sequence) is ignored', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1, { message: { plainText: 'changed', fragments: [] } }) });
    expect(chatOverlayItemsInOrder(state)[0]?.message?.plainText).toBe('hi');
  });

  it('an out-of-order upsert (lower sequence arriving after a higher one) never regresses state', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 5) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 2, { message: { plainText: 'stale', fragments: [] } }) });
    expect(chatOverlayItemsInOrder(state)[0]?.sequence).toBe(5);
  });

  it('an upsert to an existing id updates it in place, without moving its position', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('b', 2) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 3, { deleted: true, message: undefined }) });
    expect(order(state)).toEqual(['a', 'b']);
    expect(chatOverlayItemsInOrder(state)[0]?.deleted).toBe(true);
  });

  it('remove deletes an id entirely, unlike operator-chat which only marks deleted', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('b', 2) });
    state = chatOverlayReducer(state, { type: 'remove', id: 'a' });
    expect(order(state)).toEqual(['b']);
  });

  it('remove of an unknown id is harmless', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'remove', id: 'never-seen' });
    expect(order(state)).toEqual(['a']);
  });

  it('reset replaces the entire visible set and re-establishes order', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('b', 2) });
    state = chatOverlayReducer(state, { type: 'reset', items: [item('c', 10), item('d', 11)] });
    expect(order(state)).toEqual(['c', 'd']);
  });

  it('reset with an empty list clears everything', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'reset', items: [] });
    expect(order(state)).toEqual([]);
  });

  it('a duplicate revision delivered twice is harmless (idempotent replay)', () => {
    let state = createChatOverlayState();
    const upsertA = { type: 'upsert' as const, item: item('a', 1) };
    state = chatOverlayReducer(state, upsertA);
    state = chatOverlayReducer(state, upsertA);
    expect(order(state)).toEqual(['a']);
  });
});
