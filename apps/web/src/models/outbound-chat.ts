import type { OperatorChatItem } from '@/api/operator-chat-schemas';

/** Stage 11A's own backend-authoritative message limit - mirrored here only
 * for the live counter and the disabled-send check; the backend
 * (internal/outboundchat.ValidateMessage) is the real authority. */
export const MAX_MESSAGE_CODE_POINTS = 500;

/** Counts Unicode code points the same way the backend does (Go's
 * `for _, r := range message` is a code-point iteration) - `Array.from`
 * splits a string into an array of code points, correctly counting a
 * surrogate-pair astral character (most emoji) as one, not two. */
export function codePointLength(text: string): number {
  return Array.from(text).length;
}

/** Whether text is currently sendable: non-empty after trimming ordinary
 * whitespace, and at or under the code-point limit. A pure, client-side
 * pre-check only - the backend re-validates independently and is the real
 * authority (see the stage's own "backend is authoritative" requirement). */
export function isMessageSendable(text: string): boolean {
  if (text.trim() === '') return false;
  return codePointLength(text) <= MAX_MESSAGE_CODE_POINTS;
}

export type ReplyTarget = {
  accountId: string;
  providerMessageId: string;
  authorDisplayName: string;
  /** A short, truncated snippet for the reply preview - never the full
   * message body is required for this, and this is never sent to the
   * backend (only providerMessageId is). */
  preview: string;
};

const REPLY_PREVIEW_MAX_LENGTH = 80;

/** Whether item is eligible for the Chat page's Reply action: a real,
 * non-deleted Twitch chat message with a known provider message id - never
 * an activity, moderation row, deleted placeholder, or non-Twitch item.
 * Returns the reply target to use if so, otherwise null. */
export function replyTargetFor(item: OperatorChatItem): ReplyTarget | null {
  if (item.kind !== 'message') return null;
  if (item.providerId !== 'twitch') return null;
  if (item.lifecycle.deleted) return null;
  if (item.providerMessageId === undefined || item.providerMessageId === '') return null;

  const text = item.message?.plainText ?? '';
  const preview =
    text.length > REPLY_PREVIEW_MAX_LENGTH ? text.slice(0, REPLY_PREVIEW_MAX_LENGTH) + '…' : text;
  const authorDisplayName = item.user?.anonymous === false ? (item.user.displayName ?? '') : '';

  return { accountId: item.connectedAccountId, providerMessageId: item.providerMessageId, authorDisplayName, preview };
}
