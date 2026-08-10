import type { AlertHideReason, PublicAlert } from '@/api/alerts-schemas';

/**
 * Reducer for one public alert profile's playback stream - deliberately
 * much smaller than models/chat-overlay-reducer.ts's own: an alert
 * profile has exactly one "current" item, never a scrolling list, and
 * the public Browser Source never receives queued-future content (see
 * docs/progress.md's Stage 12A HTTP-API entry) - so there is nothing to
 * key by id or order.
 *
 * lastHideReason (Stage 12B) is purely informational - the renderer
 * itself transitions identically regardless of *why* the previous alert
 * left (Part 21: preemption is urgent, never a special-cased animation),
 * but it lets a consumer (a debug view, a test) observe that a hide was
 * specifically "preempted" without inferring it from timing.
 */

export type AlertStreamState = {
  current: PublicAlert | null;
  paused: boolean;
  lastHideReason: AlertHideReason | null;
};

export function createAlertStreamState(): AlertStreamState {
  return { current: null, paused: false, lastHideReason: null };
}

export type AlertStreamAction =
  | { type: 'show'; alert: PublicAlert; paused: boolean }
  | { type: 'hide'; paused: boolean; reason: AlertHideReason }
  | { type: 'reset'; alert: PublicAlert | null; paused: boolean }
  | { type: 'paused'; paused: boolean };

export function alertStreamReducer(state: AlertStreamState, action: AlertStreamAction): AlertStreamState {
  switch (action.type) {
    case 'show':
      return { current: action.alert, paused: action.paused, lastHideReason: null };
    case 'hide':
      return { current: null, paused: action.paused, lastHideReason: action.reason };
    case 'reset':
      return { current: action.alert, paused: action.paused, lastHideReason: null };
    case 'paused':
      return { ...state, paused: action.paused };
    default:
      return state;
  }
}
