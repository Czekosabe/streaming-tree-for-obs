import type { PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

/**
 * Bounded, keyed-by-id state for the public overlay's visible item list.
 *
 * Unlike operator-chat-reducer.ts (which only ever upserts - operator chat
 * never removes an item, it marks it deleted), the public overlay's own
 * revision protocol has three operations (upsert/remove/reset - see
 * internal/chatoverlay's own Revision type): a `remove` genuinely deletes
 * an id from state, since capacity eviction, expiry, moderation, and a
 * settings change that hides an item all need the item gone from the
 * screen, not merely marked.
 *
 * Render order is first-seen order, exactly like operator-chat-reducer.ts,
 * so an item updating in place never jumps position. `reset` replaces the
 * entire visible set (a config change, gap recovery, or the initial
 * snapshot) and re-establishes order from the given list.
 */

export type ChatOverlayState = {
  itemsById: Record<string, PublicChatOverlayItem>;
  /** Ids in first-seen order (since the last reset) - determines render order. */
  order: string[];
  /** Highest revision sequence applied so far. */
  latestSequence: number;
};

export type ChatOverlayAction =
  | { type: 'upsert'; item: PublicChatOverlayItem }
  | { type: 'remove'; id: string }
  | { type: 'reset'; items: PublicChatOverlayItem[] };

export function createChatOverlayState(): ChatOverlayState {
  return { itemsById: {}, order: [], latestSequence: 0 };
}

function upsertOne(state: ChatOverlayState, item: PublicChatOverlayItem): ChatOverlayState {
  const existing = state.itemsById[item.id];
  if (existing !== undefined && existing.sequence >= item.sequence) {
    // A duplicate delivery or an out-of-order revision that arrived after
    // a newer one for the same id - never regress visible state.
    return state;
  }

  const itemsById = { ...state.itemsById, [item.id]: item };
  const latestSequence = Math.max(state.latestSequence, item.sequence);

  if (existing !== undefined) {
    return { ...state, itemsById, latestSequence };
  }
  return { itemsById, order: [...state.order, item.id], latestSequence };
}

function removeOne(state: ChatOverlayState, id: string): ChatOverlayState {
  if (!(id in state.itemsById)) return state;
  const itemsById = { ...state.itemsById };
  delete itemsById[id];
  return { ...state, itemsById, order: state.order.filter((existing) => existing !== id) };
}

export function chatOverlayReducer(
  state: ChatOverlayState,
  action: ChatOverlayAction,
): ChatOverlayState {
  switch (action.type) {
    case 'reset': {
      let next = createChatOverlayState();
      for (const item of action.items) {
        next = upsertOne(next, item);
      }
      return { ...next, latestSequence: Math.max(state.latestSequence, next.latestSequence) };
    }
    case 'upsert':
      return upsertOne(state, action.item);
    case 'remove':
      return removeOne(state, action.id);
  }
}

/** Items in stable render order (first-seen order, not latest-revision order). */
export function chatOverlayItemsInOrder(state: ChatOverlayState): PublicChatOverlayItem[] {
  return state.order.map((id) => state.itemsById[id]!).filter((item) => item !== undefined);
}
