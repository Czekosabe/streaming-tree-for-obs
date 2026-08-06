import { isCosmeticRemoveReason, type ChatOverlayRemoveReason, type PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

/**
 * Bounded, keyed-by-id state for the public overlay's visible item list,
 * split into two disjoint parts:
 *  - `itemsById`/`order`: authoritative active items - the backend's own
 *    current truth, exactly as before.
 *  - `leaving`/`leavingOrder`: a bounded set of items the backend has
 *    already removed for a *cosmetic* reason (natural expiry or
 *    capacity eviction) but that the renderer is still showing mid exit
 *    animation. An item is never in both sets at once.
 *
 * Every other removal reason (moderation deletion, a chat/user clear,
 * or - though this design never actually sends one, see
 * internal/chatoverlay.RemoveReason's own doc comment - any
 * settings-driven reason) is applied immediately: the id is deleted
 * from both `itemsById` and `leaving` in the same action, never staged
 * for animation. The backend remains the sole source of truth for
 * whether an item exists; `leaving` is frontend presentation state
 * only, and a client that ignored it entirely (immediately dropping
 * every removal) would still end up in a correct final state.
 *
 * Render order for active items is first-seen order, exactly like
 * operator-chat-reducer.ts, so an item updating in place never jumps
 * position. `reset` replaces the entire visible set (a config change,
 * gap recovery, or the initial snapshot/hydration) and clears all
 * leaving state immediately - a reset never triggers a mass exit
 * animation for items that merely aren't in the new set.
 */

/** Defensive cap on how many items may be mid-exit-animation at once -
 * independent of any single overlay's own `maxVisibleItems`, which the
 * pure reducer has no access to. Generous enough that a real overlay's
 * bounded cosmetic-removal rate never approaches it; existing purely so
 * a pathological server behavior can never grow this list without
 * bound. The oldest pending leaving item is dropped (immediately, no
 * animation) rather than growing further. */
export const MAX_LEAVING_ITEMS = 100;

export type ChatOverlayLeavingItem = {
  item: PublicChatOverlayItem;
  reason: ChatOverlayRemoveReason;
};

export type ChatOverlayState = {
  itemsById: Record<string, PublicChatOverlayItem>;
  /** Ids in first-seen order (since the last reset) - determines render order. */
  order: string[];
  leaving: Record<string, ChatOverlayLeavingItem>;
  /** Leaving ids in the order they started leaving - oldest first. */
  leavingOrder: string[];
  /** Highest revision sequence applied so far. */
  latestSequence: number;
};

export type ChatOverlayAction =
  | { type: 'upsert'; item: PublicChatOverlayItem }
  | { type: 'remove'; id: string; reason: ChatOverlayRemoveReason }
  | { type: 'reset'; items: PublicChatOverlayItem[] }
  /** Dispatched by the renderer itself (animation end or its own
   * fallback timeout firing first) once a leaving item's exit
   * animation has finished - never sent by the server. A no-op if the
   * id is no longer in `leaving` (already completed, or since
   * immediately removed by a later, non-cosmetic action). */
  | { type: 'completeLeaving'; id: string };

export function createChatOverlayState(): ChatOverlayState {
  return { itemsById: {}, order: [], leaving: {}, leavingOrder: [], latestSequence: 0 };
}

function upsertOne(state: ChatOverlayState, item: PublicChatOverlayItem): ChatOverlayState {
  // A newer upsert always cancels any pending exit for the same id -
  // the item is visible again, so any leaving copy of it is stale.
  const wasLeaving = item.id in state.leaving;
  let leaving = state.leaving;
  let leavingOrder = state.leavingOrder;
  if (wasLeaving) {
    leaving = { ...leaving };
    delete leaving[item.id];
    leavingOrder = leavingOrder.filter((id) => id !== item.id);
  }

  const existing = state.itemsById[item.id];
  if (existing !== undefined && existing.sequence >= item.sequence) {
    // A duplicate delivery or an out-of-order revision that arrived after
    // a newer one for the same id - never regress visible state. Still
    // apply the leaving-cancellation above, since the item is active
    // either way.
    return wasLeaving ? { ...state, leaving, leavingOrder } : state;
  }

  const itemsById = { ...state.itemsById, [item.id]: item };
  const latestSequence = Math.max(state.latestSequence, item.sequence);

  if (existing !== undefined) {
    return { ...state, itemsById, leaving, leavingOrder, latestSequence };
  }
  return { ...state, itemsById, order: [...state.order, item.id], leaving, leavingOrder, latestSequence };
}

function removeImmediately(state: ChatOverlayState, id: string): ChatOverlayState {
  let next = state;
  if (id in next.itemsById) {
    const itemsById = { ...next.itemsById };
    delete itemsById[id];
    next = { ...next, itemsById, order: next.order.filter((existing) => existing !== id) };
  }
  if (id in next.leaving) {
    const leaving = { ...next.leaving };
    delete leaving[id];
    next = { ...next, leaving, leavingOrder: next.leavingOrder.filter((existing) => existing !== id) };
  }
  return next;
}

function removeCosmetically(state: ChatOverlayState, id: string, reason: ChatOverlayRemoveReason): ChatOverlayState {
  const item = state.itemsById[id];
  if (item === undefined) {
    // Not currently active. Two harmless cases: a duplicate delivery of
    // the same cosmetic remove for an id already leaving - leave it
    // exactly as it is, still animating - or an id this client never
    // had (already fully removed, or never arrived) - a true no-op.
    // Neither case ever forces an immediate removal; only a genuinely
    // immediate reason (see removeImmediately's own call site in the
    // reducer) does that.
    return state;
  }

  const itemsById = { ...state.itemsById };
  delete itemsById[id];
  const order = state.order.filter((existing) => existing !== id);

  let leaving = { ...state.leaving, [id]: { item, reason } };
  let leavingOrder = state.leavingOrder.includes(id) ? state.leavingOrder : [...state.leavingOrder, id];

  if (leavingOrder.length > MAX_LEAVING_ITEMS) {
    const oldestId = leavingOrder[0]!;
    leaving = { ...leaving };
    delete leaving[oldestId];
    leavingOrder = leavingOrder.slice(1);
  }

  return { ...state, itemsById, order, leaving, leavingOrder };
}

function completeLeaving(state: ChatOverlayState, id: string): ChatOverlayState {
  if (!(id in state.leaving)) return state;
  const leaving = { ...state.leaving };
  delete leaving[id];
  return { ...state, leaving, leavingOrder: state.leavingOrder.filter((existing) => existing !== id) };
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
      // A reset always clears every pending leaving item immediately -
      // it replaces state wholesale rather than animating out whatever
      // was mid-transition before it (Part 11: "a configuration reset
      // should replace state immediately").
      return { ...next, latestSequence: Math.max(state.latestSequence, next.latestSequence) };
    }
    case 'upsert':
      return upsertOne(state, action.item);
    case 'remove':
      return isCosmeticRemoveReason(action.reason)
        ? removeCosmetically(state, action.id, action.reason)
        : removeImmediately(state, action.id);
    case 'completeLeaving':
      return completeLeaving(state, action.id);
  }
}

/** Active items in stable render order (first-seen order, not latest-revision order). */
export function chatOverlayItemsInOrder(state: ChatOverlayState): PublicChatOverlayItem[] {
  return state.order.map((id) => state.itemsById[id]!).filter((item) => item !== undefined);
}

/** Leaving items (mid exit-animation), oldest-started-leaving first. */
export function chatOverlayLeavingItemsInOrder(state: ChatOverlayState): ChatOverlayLeavingItem[] {
  return state.leavingOrder.map((id) => state.leaving[id]!).filter((entry) => entry !== undefined);
}
