import type { ParseKeys } from 'i18next';

import type { AccountStatus, DeviceFlowState } from '@/api/account-schemas';
import type { PlatformStatus } from './platform';

/**
 * Maps device-flow and connected-account state onto presentation: label and
 * tone. Pure and exhaustive, mirroring branch-presentation.ts and
 * runtime-presentation.ts, so a new state cannot be silently forgotten.
 */

export type AccountKey = ParseKeys<'accounts'>;

export function deviceFlowStateKey(state: DeviceFlowState): AccountKey {
  const keys: Record<DeviceFlowState, AccountKey> = {
    requesting_code: 'deviceFlow.state.requesting_code',
    waiting_for_user: 'deviceFlow.state.waiting_for_user',
    polling: 'deviceFlow.state.polling',
    authorized: 'deviceFlow.state.authorized',
    denied: 'deviceFlow.state.denied',
    expired: 'deviceFlow.state.expired',
    cancelled: 'deviceFlow.state.cancelled',
    error: 'deviceFlow.state.error',
  };
  return keys[state];
}

export function deviceFlowTone(state: DeviceFlowState): PlatformStatus {
  switch (state) {
    case 'authorized':
      return 'live';
    case 'requesting_code':
    case 'waiting_for_user':
    case 'polling':
      return 'starting';
    case 'denied':
    case 'expired':
    case 'error':
      return 'error';
    case 'cancelled':
      return 'offline';
  }
}

/** True once a device-flow attempt can no longer change. */
export function deviceFlowIsTerminal(state: DeviceFlowState): boolean {
  return (
    state === 'authorized' ||
    state === 'denied' ||
    state === 'expired' ||
    state === 'cancelled' ||
    state === 'error'
  );
}

export function accountStatusKey(status: AccountStatus): AccountKey {
  const keys: Record<AccountStatus, AccountKey> = {
    connected: 'account.status.connected',
    reconnect_required: 'account.status.reconnect_required',
  };
  return keys[status];
}

export function accountStatusTone(status: AccountStatus): PlatformStatus {
  return status === 'connected' ? 'live' : 'error';
}

/** Translation key for one publish/link blocker identifier, or null for one
 * this build does not recognise - the caller falls back to the identifier
 * itself, matching branch-presentation.ts's blockerKey convention. */
export function publishBlockerKey(blocker: string): AccountKey | null {
  const keys: Record<string, AccountKey> = {
    account_not_linked: 'publish.blockers.accountNotLinked',
    account_reconnect_required: 'publish.blockers.accountReconnectRequired',
    credential_store_unavailable: 'publish.blockers.credentialStoreUnavailable',
    missing_required_scope: 'publish.blockers.missingRequiredScope',
    category_not_selected: 'publish.blockers.categoryNotSelected',
    provider_unavailable: 'publish.blockers.providerUnavailable',
    rate_limited: 'publish.blockers.rateLimited',
  };
  return Object.prototype.hasOwnProperty.call(keys, blocker) ? (keys[blocker] ?? null) : null;
}
