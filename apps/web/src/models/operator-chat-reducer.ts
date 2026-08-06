import type { OperatorChatItem } from '@/api/operator-chat-schemas';

/**
 * Bounded, keyed-by-id state for the operator-chat timeline.
 *
 * Every incoming revision (a brand-new item or a lifecycle update to an
 * existing one - see the backend's own "complete upsert" design) is folded
 * in by item id: a duplicate or out-of-order revision for an id already
 * seen at an equal-or-higher sequence is ignored. Render order is the
 * order an id was FIRST observed, not the sequence of its latest revision
 * - a message becoming deleted must not jump to the bottom of the
 * timeline.
 *
 * Bounded to `capacity` distinct ids: the oldest-first-seen id is evicted
 * once the cap is exceeded, mirroring the backend projection's own ring
 * buffer bound.
 */

export type OperatorChatState = {
  itemsById: Record<string, OperatorChatItem>;
  /** Ids in first-seen order - determines render order. */
  order: string[];
  /** Highest revision sequence applied so far. */
  latestSequence: number;
  capacity: number;
};

export type OperatorChatAction =
  | { type: 'upsert'; item: OperatorChatItem }
  | { type: 'reset'; items: OperatorChatItem[] };

export function createOperatorChatState(capacity: number): OperatorChatState {
  return { itemsById: {}, order: [], latestSequence: 0, capacity };
}

function upsertOne(state: OperatorChatState, item: OperatorChatItem): OperatorChatState {
  const existing = state.itemsById[item.id];
  if (existing !== undefined && existing.sequence >= item.sequence) {
    // A duplicate delivery or an out-of-order revision that arrived after
    // a newer one for the same id - never regress visible state.
    return state;
  }

  const itemsById = { ...state.itemsById, [item.id]: item };
  const latestSequence = Math.max(state.latestSequence, item.sequence);

  if (existing !== undefined) {
    // A lifecycle update to an id already in `order` - position unchanged.
    return { ...state, itemsById, latestSequence };
  }

  const order = [...state.order, item.id];
  if (order.length <= state.capacity) {
    return { itemsById, order, latestSequence, capacity: state.capacity };
  }

  const evictedId = order[0]!;
  const trimmedOrder = order.slice(1);
  const trimmedItems = { ...itemsById };
  delete trimmedItems[evictedId];
  return { itemsById: trimmedItems, order: trimmedOrder, latestSequence, capacity: state.capacity };
}

export function operatorChatReducer(
  state: OperatorChatState,
  action: OperatorChatAction,
): OperatorChatState {
  switch (action.type) {
    case 'reset': {
      let next = createOperatorChatState(state.capacity);
      for (const item of action.items) {
        next = upsertOne(next, item);
      }
      return next;
    }
    case 'upsert':
      return upsertOne(state, action.item);
  }
}

/** Items in stable render order (first-seen order, not latest-revision order). */
export function operatorChatItemsInOrder(state: OperatorChatState): OperatorChatItem[] {
  return state.order.map((id) => state.itemsById[id]!).filter((item) => item !== undefined);
}
