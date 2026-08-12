import { describe, expect, it } from 'vitest';

import type { DonationConnectorState } from '@/api/donationsource-schemas';

import { donationConnectorStateKey, donationConnectorStateTone } from './donationsource-presentation';

const ALL_STATES: DonationConnectorState[] = [
  'disabled',
  'connecting',
  'connected',
  'reconnecting',
  'possible_gap',
  'reconnect_required',
  'error',
  'stopping',
];

describe('donationConnectorStateKey', () => {
  it('returns a distinct key for every state', () => {
    const keys = ALL_STATES.map(donationConnectorStateKey);
    expect(new Set(keys).size).toBe(ALL_STATES.length);
  });
});

describe('donationConnectorStateTone', () => {
  it('reports connected as live', () => {
    expect(donationConnectorStateTone('connected')).toBe('live');
  });

  it('reports disabled as offline', () => {
    expect(donationConnectorStateTone('disabled')).toBe('offline');
  });

  it('reports error and reconnect_required as error', () => {
    expect(donationConnectorStateTone('error')).toBe('error');
    expect(donationConnectorStateTone('reconnect_required')).toBe('error');
  });

  it('reports connecting/reconnecting/possible_gap/stopping as starting', () => {
    expect(donationConnectorStateTone('connecting')).toBe('starting');
    expect(donationConnectorStateTone('reconnecting')).toBe('starting');
    expect(donationConnectorStateTone('possible_gap')).toBe('starting');
    expect(donationConnectorStateTone('stopping')).toBe('starting');
  });

  it('is exhaustive over every state without throwing', () => {
    for (const state of ALL_STATES) {
      expect(() => donationConnectorStateTone(state)).not.toThrow();
    }
  });
});
