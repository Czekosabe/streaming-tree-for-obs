import type { ParseKeys } from 'i18next';

import type { AccountStatus, DeviceFlowState, OAuthAttemptState } from '@/api/account-schemas';
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

export function oauthAttemptStateKey(state: OAuthAttemptState): AccountKey {
  const keys: Record<OAuthAttemptState, AccountKey> = {
    creating: 'oauthAttempt.state.creating',
    waiting_for_browser: 'oauthAttempt.state.waiting_for_browser',
    processing_callback: 'oauthAttempt.state.processing_callback',
    awaiting_channel_selection: 'oauthAttempt.state.awaiting_channel_selection',
    authorized: 'oauthAttempt.state.authorized',
    denied: 'oauthAttempt.state.denied',
    expired: 'oauthAttempt.state.expired',
    cancelled: 'oauthAttempt.state.cancelled',
    error: 'oauthAttempt.state.error',
  };
  return keys[state];
}

export function oauthAttemptTone(state: OAuthAttemptState): PlatformStatus {
  switch (state) {
    case 'authorized':
      return 'live';
    case 'creating':
    case 'waiting_for_browser':
    case 'processing_callback':
    case 'awaiting_channel_selection':
      return 'starting';
    case 'denied':
    case 'expired':
    case 'error':
      return 'error';
    case 'cancelled':
      return 'offline';
  }
}

/** True once a YouTube OAuth attempt can no longer change. */
export function oauthAttemptIsTerminal(state: OAuthAttemptState): boolean {
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
    youtube_broadcast_not_selected: 'publish.blockers.youtubeBroadcastNotSelected',
    youtube_broadcast_not_found: 'publish.blockers.youtubeBroadcastNotFound',
    youtube_live_streaming_not_enabled: 'publish.blockers.youtubeLiveStreamingNotEnabled',
    youtube_region_required: 'publish.blockers.youtubeRegionRequired',
    youtube_category_required: 'publish.blockers.youtubeCategoryRequired',
    youtube_quota_exceeded: 'publish.blockers.youtubeQuotaExceeded',
    youtube_unavailable: 'publish.blockers.youtubeUnavailable',
  };
  return Object.prototype.hasOwnProperty.call(keys, blocker) ? (keys[blocker] ?? null) : null;
}

/** Translation key for one publish warning identifier, or null for one this
 * build does not recognise. */
export function publishWarningKey(warning: string): AccountKey | null {
  const keys: Record<string, AccountKey> = {
    testing_mode_seven_day_token: 'publish.warnings.testingModeSevenDayToken',
    not_verified_stream_key_binding: 'publish.warnings.notVerifiedStreamKeyBinding',
  };
  return Object.prototype.hasOwnProperty.call(keys, warning) ? (keys[warning] ?? null) : null;
}
