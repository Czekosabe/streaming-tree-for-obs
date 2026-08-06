/**
 * Pure autoscroll state machine for the chat timeline - see the Stage 9
 * task's Part 20. Kept separate from any DOM/ref code so the transitions
 * are directly testable: near-bottom detection, pausing on manual scroll-
 * up, and resuming (with an unseen-count reset) on "jump to latest".
 */

export type AutoscrollState = {
  /** True while new items should auto-scroll into view. */
  following: boolean;
  /** Items that arrived while not following - shown as "N new messages". */
  unseenCount: number;
};

export const initialAutoscrollState: AutoscrollState = { following: true, unseenCount: 0 };

/** Default distance (px) from the bottom still considered "at the bottom" -
 * a small tolerance so sub-pixel layout differences never falsely pause
 * autoscroll. */
export const NEAR_BOTTOM_THRESHOLD_PX = 48;

export function isNearBottom(
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
  threshold: number = NEAR_BOTTOM_THRESHOLD_PX,
): boolean {
  return scrollHeight - (scrollTop + clientHeight) <= threshold;
}

export type AutoscrollAction =
  | { type: 'scrolled'; nearBottom: boolean }
  | { type: 'items-appended'; count: number }
  | { type: 'jump-to-latest' };

export function autoscrollReducer(
  state: AutoscrollState,
  action: AutoscrollAction,
): AutoscrollState {
  switch (action.type) {
    case 'scrolled':
      if (action.nearBottom) {
        return { following: true, unseenCount: 0 };
      }
      return { following: false, unseenCount: state.unseenCount };
    case 'items-appended':
      if (state.following) {
        // Autoscroll will carry the viewport down - nothing goes unseen.
        return state;
      }
      return { following: false, unseenCount: state.unseenCount + action.count };
    case 'jump-to-latest':
      return { following: true, unseenCount: 0 };
  }
}
