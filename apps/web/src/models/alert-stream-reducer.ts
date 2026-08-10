import type { PublicAlert } from '@/api/alerts-schemas';

/**
 * Reducer for one public alert profile's playback stream - deliberately
 * much smaller than models/chat-overlay-reducer.ts's own: an alert
 * profile has exactly one "current" item, never a scrolling list, and
 * the public Browser Source never receives queued-future content (see
 * docs/progress.md's Stage 12A HTTP-API entry) - so there is nothing to
 * key by id or order.
 */

export type AlertStreamState = {
  current: PublicAlert | null;
  paused: boolean;
};

export function createAlertStreamState(): AlertStreamState {
  return { current: null, paused: false };
}

export type AlertStreamAction =
  | { type: 'show'; alert: PublicAlert; paused: boolean }
  | { type: 'hide'; paused: boolean }
  | { type: 'reset'; alert: PublicAlert | null; paused: boolean }
  | { type: 'paused'; paused: boolean };

export function alertStreamReducer(state: AlertStreamState, action: AlertStreamAction): AlertStreamState {
  switch (action.type) {
    case 'show':
      return { current: action.alert, paused: action.paused };
    case 'hide':
      return { current: null, paused: action.paused };
    case 'reset':
      return { current: action.alert, paused: action.paused };
    case 'paused':
      return { ...state, paused: action.paused };
    default:
      return state;
  }
}
