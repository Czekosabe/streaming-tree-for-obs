import type { ParseKeys } from 'i18next';

import type {
  OperatorChatDeletionReason,
  OperatorChatItem,
  OperatorChatKind,
} from '@/api/operator-chat-schemas';

/**
 * Maps the operator-chat item model onto presentation: labels, command/bot
 * detection, and asset-URL safety. Pure and total, mirroring
 * engagement-presentation.ts's own reasoning - an unrecognized identifier
 * degrades to a safe fallback rather than crashing the page.
 */

export type ChatKey = ParseKeys<'chat'>;

export function kindLabelKey(kind: OperatorChatKind): ChatKey {
  const keys: Record<OperatorChatKind, ChatKey> = {
    message: 'kind.message',
    activity: 'kind.activity',
    moderation: 'kind.moderation',
    system: 'kind.system',
  };
  return keys[kind];
}

const ACTIVITY_TYPE_KEYS: Record<string, ChatKey> = {
  follow: 'activityType.follow',
  subscription: 'activityType.subscription',
  resubscription: 'activityType.resubscription',
  gifted_subscription: 'activityType.gifted_subscription',
  subscription_gift_batch: 'activityType.subscription_gift_batch',
  bits: 'activityType.bits',
  raid: 'activityType.raid',
  channel_point_redemption: 'activityType.channel_point_redemption',
  'stream.online': 'activityType.stream_online',
  'stream.offline': 'activityType.stream_offline',
  donation: 'activityType.donation',
};

/** Translation key for an activity type, or null for one this build does
 * not recognise (rendered from its raw identifier, never dropped). */
export function activityTypeKey(activityType: string): ChatKey | null {
  return Object.prototype.hasOwnProperty.call(ACTIVITY_TYPE_KEYS, activityType)
    ? (ACTIVITY_TYPE_KEYS[activityType] ?? null)
    : null;
}

const MODERATION_ACTION_KEYS: Record<string, ChatKey> = {
  message_deleted_not_retained: 'moderationAction.messageDeletedNotRetained',
  chat_cleared: 'moderationAction.chatCleared',
  user_messages_cleared: 'moderationAction.userMessagesCleared',
};

export function moderationActionKey(action: string): ChatKey | null {
  return Object.prototype.hasOwnProperty.call(MODERATION_ACTION_KEYS, action)
    ? (MODERATION_ACTION_KEYS[action] ?? null)
    : null;
}

const DELETION_REASON_KEYS: Record<OperatorChatDeletionReason, ChatKey> = {
  moderator_deleted: 'deletionReason.moderatorDeleted',
  chat_cleared: 'deletionReason.chatCleared',
  user_messages_cleared: 'deletionReason.userMessagesCleared',
};

export function deletionReasonKey(reason: string): ChatKey | null {
  return Object.prototype.hasOwnProperty.call(DELETION_REASON_KEYS, reason)
    ? (DELETION_REASON_KEYS[reason as OperatorChatDeletionReason] ?? null)
    : null;
}

/** The default command prefix - see the Stage 9 task's Part 12: a fixed
 * prefix this stage, never an arbitrary substring match. */
export const DEFAULT_COMMAND_PREFIX = '!';

/** Whether a chat message's plain text is a command: the trimmed text
 * begins with the command prefix. Never interprets arbitrary substrings. */
export function isCommandMessage(plainText: string, prefix: string = DEFAULT_COMMAND_PREFIX): boolean {
  return plainText.trimStart().startsWith(prefix);
}

/**
 * Whether an emote/badge image URL is safe to render: https, no userinfo
 * component, and hosted on Twitch's own documented CDN host - see the
 * Stage 9 task's Part 22. A caller that gets `false` falls back to text
 * rather than rendering the URL.
 */
const ALLOWED_ASSET_HOSTS = ['static-cdn.jtvnw.net'];

export function isSafeTwitchAssetUrl(rawUrl: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(rawUrl);
  } catch {
    return false;
  }
  if (parsed.protocol !== 'https:') return false;
  if (parsed.username !== '' || parsed.password !== '') return false;
  return ALLOWED_ASSET_HOSTS.includes(parsed.hostname);
}

/** Display-name fallback chain: display name, then login, then a safe
 * anonymous marker - never a fabricated identity. */
export function userDisplayLabel(user: OperatorChatItem['user']): string {
  if (user === undefined || user.anonymous) return '';
  return user.displayName ?? user.login ?? '';
}

/** A safe, non-color-alone way to tell a deleted message apart: callers
 * pair this with a visual style, never color alone (Part 25). */
export function isDeletedMessage(item: OperatorChatItem): boolean {
  return item.kind === 'message' && item.lifecycle.deleted;
}
