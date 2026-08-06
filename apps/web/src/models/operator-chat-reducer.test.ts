import { describe, expect, it } from 'vitest';

import type { OperatorChatItem } from '@/api/operator-chat-schemas';

import {
  createOperatorChatState,
  operatorChatItemsInOrder,
  operatorChatReducer,
} from './operator-chat-reducer';

function item(overrides: Partial<OperatorChatItem> & { id: string; sequence: number }): OperatorChatItem {
  return {
    version: 1,
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    kind: 'message',
    occurredAt: '2026-08-06T12:00:00Z',
    receivedAt: '2026-08-06T12:00:00Z',
    lifecycle: { deleted: false },
    synthetic: false,
    ...overrides,
  };
}

describe('operatorChatReducer', () => {
  it('adds a new item to the timeline', () => {
    const state = createOperatorChatState(10);
    const next = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 1 }) });
    expect(operatorChatItemsInOrder(next).map((i) => i.id)).toEqual(['a']);
  });

  it('keeps items in first-seen order across multiple upserts', () => {
    let state = createOperatorChatState(10);
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 1 }) });
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'b', sequence: 2 }) });
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'c', sequence: 3 }) });
    expect(operatorChatItemsInOrder(state).map((i) => i.id)).toEqual(['a', 'b', 'c']);
  });

  it('a lifecycle update to an existing item does not move its position', () => {
    let state = createOperatorChatState(10);
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 1 }) });
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'b', sequence: 2 }) });
    // 'a' is updated (e.g. marked deleted) with a higher sequence than 'b'.
    state = operatorChatReducer(state, {
      type: 'upsert',
      item: item({ id: 'a', sequence: 3, lifecycle: { deleted: true, deletionReason: 'moderator_deleted' } }),
    });
    const ordered = operatorChatItemsInOrder(state);
    expect(ordered.map((i) => i.id)).toEqual(['a', 'b']);
    expect(ordered[0]?.lifecycle.deleted).toBe(true);
  });

  it('ignores a duplicate revision at an equal or lower sequence', () => {
    let state = createOperatorChatState(10);
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 5 }) });
    const beforeDuplicate = state;
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 5 }) });
    expect(state).toBe(beforeDuplicate);

    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 3 }) });
    expect(state).toBe(beforeDuplicate);
  });

  it('evicts the oldest-first-seen item once capacity is exceeded', () => {
    let state = createOperatorChatState(2);
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 1 }) });
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'b', sequence: 2 }) });
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'c', sequence: 3 }) });
    expect(operatorChatItemsInOrder(state).map((i) => i.id)).toEqual(['b', 'c']);
  });

  it('reset replaces the entire state from a snapshot, in order', () => {
    let state = createOperatorChatState(10);
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'stale', sequence: 1 }) });
    state = operatorChatReducer(state, {
      type: 'reset',
      items: [item({ id: 'x', sequence: 10 }), item({ id: 'y', sequence: 11 })],
    });
    expect(operatorChatItemsInOrder(state).map((i) => i.id)).toEqual(['x', 'y']);
    expect(state.latestSequence).toBe(11);
  });

  it('reset preserves capacity from the previous state', () => {
    let state = createOperatorChatState(1);
    state = operatorChatReducer(state, {
      type: 'reset',
      items: [item({ id: 'x', sequence: 1 }), item({ id: 'y', sequence: 2 })],
    });
    expect(operatorChatItemsInOrder(state).map((i) => i.id)).toEqual(['y']);
  });

  it('tracks the highest sequence seen across upserts', () => {
    let state = createOperatorChatState(10);
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'a', sequence: 5 }) });
    state = operatorChatReducer(state, { type: 'upsert', item: item({ id: 'b', sequence: 2 }) });
    expect(state.latestSequence).toBe(5);
  });
});
