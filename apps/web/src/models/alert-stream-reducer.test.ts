import { describe, expect, it } from 'vitest';

import type { PublicAlert } from '@/api/alerts-schemas';

import { alertStreamReducer, createAlertStreamState } from './alert-stream-reducer';

function makeAlert(overrides: Partial<PublicAlert> = {}): PublicAlert {
  return {
    schemaVersion: 1, alertId: 'alinst_1', eventType: 'follow', providerId: 'twitch',
    synthetic: false, replayed: false, renderedText: 'Ann followed!', groupCount: 1,
    durationMs: 5000, entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    renderingMode: 'legacy',
    ...overrides,
  };
}

describe('alertStreamReducer', () => {
  it('starts with no current alert and not paused', () => {
    const state = createAlertStreamState();
    expect(state.current).toBeNull();
    expect(state.paused).toBe(false);
  });

  it('show sets the current alert', () => {
    const state = alertStreamReducer(createAlertStreamState(), { type: 'show', alert: makeAlert(), paused: false });
    expect(state.current?.alertId).toBe('alinst_1');
  });

  it('hide clears the current alert', () => {
    let state = alertStreamReducer(createAlertStreamState(), { type: 'show', alert: makeAlert(), paused: false });
    state = alertStreamReducer(state, { type: 'hide', paused: false, reason: 'completed' });
    expect(state.current).toBeNull();
  });

  it('hide records its own reason (Stage 12B, e.g. "preempted")', () => {
    let state = alertStreamReducer(createAlertStreamState(), { type: 'show', alert: makeAlert(), paused: false });
    state = alertStreamReducer(state, { type: 'hide', paused: false, reason: 'preempted' });
    expect(state.lastHideReason).toBe('preempted');
  });

  it('a show after a hide clears the last hide reason', () => {
    let state = alertStreamReducer(createAlertStreamState(), { type: 'show', alert: makeAlert(), paused: false });
    state = alertStreamReducer(state, { type: 'hide', paused: false, reason: 'preempted' });
    state = alertStreamReducer(state, { type: 'show', alert: makeAlert({ alertId: 'alinst_2' }), paused: false });
    expect(state.lastHideReason).toBeNull();
  });

  it('reset replaces state wholesale, including paused', () => {
    const state = alertStreamReducer(createAlertStreamState(), {
      type: 'reset', alert: makeAlert({ alertId: 'alinst_2' }), paused: true,
    });
    expect(state.current?.alertId).toBe('alinst_2');
    expect(state.paused).toBe(true);
  });

  it('reset with a null alert clears current', () => {
    let state = alertStreamReducer(createAlertStreamState(), { type: 'show', alert: makeAlert(), paused: false });
    state = alertStreamReducer(state, { type: 'reset', alert: null, paused: false });
    expect(state.current).toBeNull();
  });

  it('paused updates only the paused flag, leaving current untouched', () => {
    let state = alertStreamReducer(createAlertStreamState(), { type: 'show', alert: makeAlert(), paused: false });
    state = alertStreamReducer(state, { type: 'paused', paused: true });
    expect(state.paused).toBe(true);
    expect(state.current?.alertId).toBe('alinst_1');
  });

  it('a new show while one alert is current replaces it (never merges)', () => {
    let state = alertStreamReducer(createAlertStreamState(), { type: 'show', alert: makeAlert(), paused: false });
    state = alertStreamReducer(state, { type: 'show', alert: makeAlert({ alertId: 'alinst_2' }), paused: false });
    expect(state.current?.alertId).toBe('alinst_2');
  });
});
