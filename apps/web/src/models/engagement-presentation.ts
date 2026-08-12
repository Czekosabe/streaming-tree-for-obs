import type { ParseKeys } from 'i18next';

import type { ConnectorState, EngagementEventType } from '@/api/engagement-schemas';

import type { PlatformStatus } from './platform';

/**
 * Maps the Twitch engagement connector's state and normalized event types
 * onto presentation: label and tone. Pure and exhaustive, mirroring
 * account-presentation.ts's own device-flow/OAuth-attempt mappings, so a
 * new state or type cannot be silently forgotten.
 */

export type EngagementKey = ParseKeys<'engagement'>;

export function connectorStateKey(state: ConnectorState): EngagementKey {
  const keys: Record<ConnectorState, EngagementKey> = {
    disabled: 'connector.state.disabled',
    blocked: 'connector.state.blocked',
    connecting: 'connector.state.connecting',
    waiting_for_welcome: 'connector.state.waiting_for_welcome',
    subscribing: 'connector.state.subscribing',
    connected: 'connector.state.connected',
    reconnecting: 'connector.state.reconnecting',
    stopping: 'connector.state.stopping',
    error: 'connector.state.error',
  };
  return keys[state];
}

export function connectorStateTone(state: ConnectorState): PlatformStatus {
  switch (state) {
    case 'connected':
      return 'live';
    case 'connecting':
    case 'waiting_for_welcome':
    case 'subscribing':
    case 'reconnecting':
    case 'stopping':
      return 'starting';
    case 'blocked':
    case 'error':
      return 'error';
    case 'disabled':
      return 'offline';
  }
}

/** Translation key for one connector blocker code, or null for one this
 * build does not recognise (rendered as its raw code, not silently
 * dropped). */
export function connectorBlockerKey(blocker: string): EngagementKey | null {
  const keys: Record<string, EngagementKey> = {
    engagement_scope_upgrade_required: 'connector.blockers.scopeUpgradeRequired',
    engagement_not_configured: 'connector.blockers.notConfigured',
    engagement_account_unhealthy: 'connector.blockers.accountUnhealthy',
    credential_store_unavailable: 'connector.blockers.credentialStoreUnavailable',
  };
  return Object.prototype.hasOwnProperty.call(keys, blocker) ? (keys[blocker] ?? null) : null;
}

export function eventTypeKey(type: EngagementEventType): EngagementKey {
  const keys: Record<EngagementEventType, EngagementKey> = {
    'chat.message': 'eventType.chat_message',
    'chat.message_deleted': 'eventType.chat_message_deleted',
    'chat.cleared': 'eventType.chat_cleared',
    moderation: 'eventType.moderation',
    follow: 'eventType.follow',
    subscription: 'eventType.subscription',
    resubscription: 'eventType.resubscription',
    gifted_subscription: 'eventType.gifted_subscription',
    subscription_gift_batch: 'eventType.subscription_gift_batch',
    bits: 'eventType.bits',
    raid: 'eventType.raid',
    channel_point_redemption: 'eventType.channel_point_redemption',
    'stream.online': 'eventType.stream_online',
    'stream.offline': 'eventType.stream_offline',
    'youtube.membership': 'eventType.youtube_membership',
    'youtube.membership_milestone': 'eventType.youtube_membership_milestone',
    'youtube.super_chat': 'eventType.youtube_super_chat',
    'youtube.super_sticker': 'eventType.youtube_super_sticker',
    donation: 'eventType.donation',
  };
  return keys[type];
}

/**
 * A short, safe plain-text summary of one event for the diagnostic feed -
 * never HTML, never a full chat-overlay rendering (that belongs to a later
 * stage). Falls back to the display name/login when a chat message has no
 * text (e.g. a follow/sub event carries no message at all).
 */
export function eventSummary(event: {
  type: EngagementEventType;
  user?: { displayName?: string | undefined; login?: string | undefined; anonymous: boolean } | undefined;
  message?: { text: string } | undefined;
  quantity?: number | undefined;
  /** Stage 15A - the display-ready form of a real Money value (never
   * derived from amountMicros here; this diagnostic feed never formats
   * currency itself, it only ever shows what the provider/backend
   * already rendered - see internal/domain/engagement.Money's own
   * DisplayAmount field). */
  displayAmount?: string | undefined;
}): { actor: string; detail: string } {
  const actor = event.user?.anonymous
    ? ''
    : (event.user?.displayName ?? event.user?.login ?? '');

  switch (event.type) {
    case 'chat.message':
      return { actor, detail: event.message?.text ?? '' };
    case 'chat.message_deleted':
      return { actor, detail: '' };
    case 'chat.cleared':
      return { actor: '', detail: '' };
    case 'moderation':
      return { actor, detail: '' };
    case 'follow':
      return { actor, detail: '' };
    case 'subscription':
      return { actor, detail: '' };
    case 'resubscription':
      return { actor, detail: event.message?.text ?? '' };
    case 'gifted_subscription':
      return { actor, detail: '' };
    case 'subscription_gift_batch':
      return { actor, detail: event.quantity !== undefined ? String(event.quantity) : '' };
    case 'bits':
      return { actor, detail: event.quantity !== undefined ? String(event.quantity) : '' };
    case 'raid':
      return { actor, detail: event.quantity !== undefined ? String(event.quantity) : '' };
    case 'channel_point_redemption':
      return { actor, detail: event.message?.text ?? '' };
    case 'stream.online':
    case 'stream.offline':
      return { actor: '', detail: '' };
    case 'youtube.membership':
      return { actor, detail: '' };
    case 'youtube.membership_milestone':
      return { actor, detail: event.message?.text ?? '' };
    case 'youtube.super_chat':
      return { actor, detail: event.displayAmount ?? event.message?.text ?? '' };
    case 'youtube.super_sticker':
      return { actor, detail: event.displayAmount ?? '' };
    case 'donation':
      return { actor, detail: event.displayAmount ?? event.message?.text ?? '' };
  }
}
