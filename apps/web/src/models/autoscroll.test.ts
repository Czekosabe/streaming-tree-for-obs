import { describe, expect, it } from 'vitest';

import { autoscrollReducer, initialAutoscrollState, isNearBottom } from './autoscroll';

describe('isNearBottom', () => {
  it('is true when scrolled exactly to the bottom', () => {
    expect(isNearBottom(500, 600, 100)).toBe(true);
  });

  it('is true within the threshold', () => {
    expect(isNearBottom(470, 600, 100, 48)).toBe(true);
  });

  it('is false well above the bottom', () => {
    expect(isNearBottom(0, 1000, 100, 48)).toBe(false);
  });

  it('is false just past the threshold', () => {
    expect(isNearBottom(400, 600, 100, 48)).toBe(false);
  });
});

describe('autoscrollReducer', () => {
  it('starts following with no unseen items', () => {
    expect(initialAutoscrollState).toEqual({ following: true, unseenCount: 0 });
  });

  it('scrolling up (not near bottom) pauses following', () => {
    const next = autoscrollReducer(initialAutoscrollState, { type: 'scrolled', nearBottom: false });
    expect(next.following).toBe(false);
  });

  it('scrolling back to the bottom resumes following and clears unseen', () => {
    const paused = autoscrollReducer(initialAutoscrollState, { type: 'scrolled', nearBottom: false });
    const withUnseen = autoscrollReducer(paused, { type: 'items-appended', count: 3 });
    const resumed = autoscrollReducer(withUnseen, { type: 'scrolled', nearBottom: true });
    expect(resumed).toEqual({ following: true, unseenCount: 0 });
  });

  it('new items while following do not increase the unseen count', () => {
    const next = autoscrollReducer(initialAutoscrollState, { type: 'items-appended', count: 5 });
    expect(next.unseenCount).toBe(0);
    expect(next.following).toBe(true);
  });

  it('new items while paused accumulate the unseen count', () => {
    let state = autoscrollReducer(initialAutoscrollState, { type: 'scrolled', nearBottom: false });
    state = autoscrollReducer(state, { type: 'items-appended', count: 2 });
    state = autoscrollReducer(state, { type: 'items-appended', count: 3 });
    expect(state.unseenCount).toBe(5);
    expect(state.following).toBe(false);
  });

  it('jump-to-latest resumes following and clears unseen', () => {
    let state = autoscrollReducer(initialAutoscrollState, { type: 'scrolled', nearBottom: false });
    state = autoscrollReducer(state, { type: 'items-appended', count: 7 });
    state = autoscrollReducer(state, { type: 'jump-to-latest' });
    expect(state).toEqual({ following: true, unseenCount: 0 });
  });
});
