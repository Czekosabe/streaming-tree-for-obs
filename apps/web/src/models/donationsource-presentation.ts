import type { ParseKeys } from 'i18next';

import type { DonationConnectorState } from '@/api/donationsource-schemas';

import type { PlatformStatus } from './platform';

/**
 * Maps the StreamElements donation-source connector's state onto
 * presentation: label and tone. Pure and exhaustive, mirroring
 * engagement-presentation.ts's own connectorStateKey/connectorStateTone so
 * a new state cannot be silently forgotten.
 */

export type EngagementKey = ParseKeys<'engagement'>;

export function donationConnectorStateKey(state: DonationConnectorState): EngagementKey {
  const keys: Record<DonationConnectorState, EngagementKey> = {
    disabled: 'streamElementsConnector.state.disabled',
    connecting: 'streamElementsConnector.state.connecting',
    connected: 'streamElementsConnector.state.connected',
    reconnecting: 'streamElementsConnector.state.reconnecting',
    possible_gap: 'streamElementsConnector.state.possible_gap',
    reconnect_required: 'streamElementsConnector.state.reconnect_required',
    error: 'streamElementsConnector.state.error',
    stopping: 'streamElementsConnector.state.stopping',
  };
  return keys[state];
}

export function donationConnectorStateTone(state: DonationConnectorState): PlatformStatus {
  switch (state) {
    case 'connected':
      return 'live';
    case 'connecting':
    case 'reconnecting':
    case 'possible_gap':
    case 'stopping':
      return 'starting';
    case 'reconnect_required':
    case 'error':
      return 'error';
    case 'disabled':
      return 'offline';
  }
}
