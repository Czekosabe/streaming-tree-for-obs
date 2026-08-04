import { describe, expect, it } from 'vitest';

import type { AccountStatus, DeviceFlowState } from '@/api/account-schemas';

import {
  accountStatusKey,
  accountStatusTone,
  deviceFlowIsTerminal,
  deviceFlowStateKey,
  deviceFlowTone,
  publishBlockerKey,
} from './account-presentation';

const ALL_DEVICE_FLOW_STATES: DeviceFlowState[] = [
  'requesting_code',
  'waiting_for_user',
  'polling',
  'authorized',
  'denied',
  'expired',
  'cancelled',
  'error',
];

const ALL_ACCOUNT_STATUSES: AccountStatus[] = ['connected', 'reconnect_required'];

describe('deviceFlowStateKey', () => {
  it('maps every state to a distinct key', () => {
    const keys = ALL_DEVICE_FLOW_STATES.map(deviceFlowStateKey);
    expect(new Set(keys).size).toBe(ALL_DEVICE_FLOW_STATES.length);
  });
});

describe('deviceFlowTone', () => {
  it('is exhaustive for every state', () => {
    for (const state of ALL_DEVICE_FLOW_STATES) {
      expect(() => deviceFlowTone(state)).not.toThrow();
    }
  });

  it('maps authorized to live', () => {
    expect(deviceFlowTone('authorized')).toBe('live');
  });

  it.each(['denied', 'expired', 'error'] as const)('maps %s to error', (state) => {
    expect(deviceFlowTone(state)).toBe('error');
  });
});

describe('deviceFlowIsTerminal', () => {
  it.each(['authorized', 'denied', 'expired', 'cancelled', 'error'] as const)(
    '%s is terminal',
    (state) => {
      expect(deviceFlowIsTerminal(state)).toBe(true);
    },
  );

  it.each(['requesting_code', 'waiting_for_user', 'polling'] as const)(
    '%s is not terminal',
    (state) => {
      expect(deviceFlowIsTerminal(state)).toBe(false);
    },
  );
});

describe('accountStatusKey and accountStatusTone', () => {
  it('maps every status to a distinct key', () => {
    const keys = ALL_ACCOUNT_STATUSES.map(accountStatusKey);
    expect(new Set(keys).size).toBe(ALL_ACCOUNT_STATUSES.length);
  });

  it('connected is live tone, reconnect_required is error tone', () => {
    expect(accountStatusTone('connected')).toBe('live');
    expect(accountStatusTone('reconnect_required')).toBe('error');
  });
});

describe('publishBlockerKey', () => {
  it('maps every documented blocker to a key', () => {
    const blockers = [
      'account_not_linked',
      'account_reconnect_required',
      'credential_store_unavailable',
      'missing_required_scope',
      'category_not_selected',
      'provider_unavailable',
      'rate_limited',
    ];
    for (const blocker of blockers) {
      expect(publishBlockerKey(blocker)).not.toBeNull();
    }
  });

  it('returns null for an unrecognised blocker so the caller can fall back to the raw identifier', () => {
    expect(publishBlockerKey('a_future_blocker_this_build_does_not_know')).toBeNull();
  });
});
