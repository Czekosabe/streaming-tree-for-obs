import { describe, expect, it } from 'vitest';

import type { ChatOverlayRemoveReason, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

import {
  chatOverlayItemsInOrder,
  chatOverlayLeavingItemsInOrder,
  chatOverlayReducer,
  createChatOverlayState,
  MAX_LEAVING_ITEMS,
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

function leavingOrder(state: ChatOverlayState): string[] {
  return chatOverlayLeavingItemsInOrder(state).map((entry) => entry.item.id);
}

function remove(id: string, reason: ChatOverlayRemoveReason) {
  return { type: 'remove' as const, id, reason };
}

describe('chatOverlayReducer - active items', () => {
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

describe('chatOverlayReducer - immediate removal (every non-cosmetic reason)', () => {
  const immediateReasons: ChatOverlayRemoveReason[] = [
    'message_deleted',
    'chat_cleared',
    'user_messages_cleared',
    'unknown',
  ];

  it.each(immediateReasons)('reason "%s" deletes the id entirely, with nothing staged as leaving', (reason) => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, { type: 'upsert', item: item('b', 2) });
    state = chatOverlayReducer(state, remove('a', reason));
    expect(order(state)).toEqual(['b']);
    expect(leavingOrder(state)).toEqual([]);
  });

  it('remove of an unknown id is harmless', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('never-seen', 'message_deleted'));
    expect(order(state)).toEqual(['a']);
  });

  it('an immediate removal also purges a pending cosmetic leaving copy of the same id', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('a', 'expired')); // now leaving
    expect(leavingOrder(state)).toEqual(['a']);
    state = chatOverlayReducer(state, remove('a', 'message_deleted')); // immediate trumps
    expect(order(state)).toEqual([]);
    expect(leavingOrder(state)).toEqual([]);
  });
});

describe('chatOverlayReducer - cosmetic removal (leaving state)', () => {
  const cosmeticReasons: ChatOverlayRemoveReason[] = ['expired', 'capacity_evicted'];

  it.each(cosmeticReasons)('reason "%s" moves the item to leaving, out of active', (reason) => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('a', reason));
    expect(order(state)).toEqual([]);
    expect(leavingOrder(state)).toEqual(['a']);
    expect(chatOverlayLeavingItemsInOrder(state)[0]?.reason).toBe(reason);
  });

  it('the leaving item keeps its original content for the animation', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1, { message: { plainText: 'still here', fragments: [] } }) });
    state = chatOverlayReducer(state, remove('a', 'expired'));
    expect(chatOverlayLeavingItemsInOrder(state)[0]?.item.message?.plainText).toBe('still here');
  });

  it('a cosmetic remove of an id that is not currently active is a no-op', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, remove('never-seen', 'expired'));
    expect(order(state)).toEqual([]);
    expect(leavingOrder(state)).toEqual([]);
  });

  it('completeLeaving removes the item from leaving state', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('a', 'expired'));
    state = chatOverlayReducer(state, { type: 'completeLeaving', id: 'a' });
    expect(leavingOrder(state)).toEqual([]);
    expect(order(state)).toEqual([]);
  });

  it('completeLeaving for an id not in leaving is harmless', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'completeLeaving', id: 'never-leaving' });
    expect(leavingOrder(state)).toEqual([]);
  });

  it('a newer upsert cancels a pending cosmetic removal of the same id, restoring it to active', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('a', 'expired'));
    expect(leavingOrder(state)).toEqual(['a']);

    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 2) });
    expect(leavingOrder(state)).toEqual([]);
    expect(order(state)).toEqual(['a']);
  });

  it('reset clears every pending leaving item immediately, without animating them out', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('a', 'expired'));
    expect(leavingOrder(state)).toEqual(['a']);

    state = chatOverlayReducer(state, { type: 'reset', items: [item('b', 10)] });
    expect(leavingOrder(state)).toEqual([]);
    expect(order(state)).toEqual(['b']);
  });

  it('duplicate cosmetic remove of the same id is idempotent', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    const removeA = remove('a', 'expired');
    state = chatOverlayReducer(state, removeA);
    state = chatOverlayReducer(state, removeA);
    expect(leavingOrder(state)).toEqual(['a']);
  });

  it('out-of-order cosmetic remove after the item is already gone is harmless', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('a', 'expired'));
    state = chatOverlayReducer(state, { type: 'completeLeaving', id: 'a' });
    // A stale, redelivered remove for the same id after it already fully
    // completed must not resurrect it or throw.
    state = chatOverlayReducer(state, remove('a', 'expired'));
    expect(order(state)).toEqual([]);
    expect(leavingOrder(state)).toEqual([]);
  });

  it('leaving items are bounded: the oldest is dropped once MAX_LEAVING_ITEMS is exceeded', () => {
    let state = createChatOverlayState();
    for (let i = 0; i < MAX_LEAVING_ITEMS + 5; i += 1) {
      const id = `item-${i}`;
      state = chatOverlayReducer(state, { type: 'upsert', item: item(id, i) });
      state = chatOverlayReducer(state, remove(id, 'expired'));
    }
    expect(leavingOrder(state).length).toBe(MAX_LEAVING_ITEMS);
    // The very first items pushed out first (FIFO bound).
    expect(leavingOrder(state)).not.toContain('item-0');
    expect(leavingOrder(state)).toContain(`item-${MAX_LEAVING_ITEMS + 4}`);
  });

  it('an item can be both evicted from active and still tracked as leaving without ever appearing in both lists at once', () => {
    let state = createChatOverlayState();
    state = chatOverlayReducer(state, { type: 'upsert', item: item('a', 1) });
    state = chatOverlayReducer(state, remove('a', 'capacity_evicted'));
    expect(order(state).includes('a')).toBe(false);
    expect(leavingOrder(state).includes('a')).toBe(true);
  });
});
