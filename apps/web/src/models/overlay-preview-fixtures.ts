import type { PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

/**
 * Deterministic, local, clearly-synthetic fixtures for the Overlays page's
 * own preview panel (Part 19) - every item has `synthetic: true`, exactly
 * like the server's own `Item.Synthetic` field, and none of this is ever
 * published to the real Event Bus, the operator chat projection, or a
 * public SSE stream. Covers every case the task names: an ordinary
 * message, badges, an emote, a mention, a long username, a long message,
 * an anonymous activity, a follow, a subscription, a gift batch, bits, a
 * deleted placeholder, and a message with no avatar.
 */

const OCCURRED_AT = '2026-08-06T20:00:00Z';

export function overlayPreviewFixtures(): PublicChatOverlayItem[] {
  return [
    {
      version: 1,
      sequence: 1,
      id: 'preview_ordinary',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'Viewer', color: '#22c55e', avatarUrl: 'https://static-cdn.jtvnw.net/preview-avatar.png' },
      message: { plainText: 'Hey, great stream today!', fragments: [{ type: 'text', text: 'Hey, great stream today!' }] },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 2,
      id: 'preview_badges',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: {
        anonymous: false,
        displayName: 'ModeratorMax',
        color: '#3b82f6',
        isModerator: true,
        isSubscriber: true,
        badges: [
          { setId: 'moderator', id: '1', imageUrl1x: 'https://static-cdn.jtvnw.net/badges/moderator.png' },
          { setId: 'subscriber', id: '12', imageUrl1x: 'https://static-cdn.jtvnw.net/badges/subscriber.png' },
        ],
      },
      message: { plainText: 'Keep it friendly, everyone.', fragments: [{ type: 'text', text: 'Keep it friendly, everyone.' }] },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 3,
      id: 'preview_emote',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'EmoteFan', color: '#f59e0b' },
      message: {
        plainText: 'PogChamp that was awesome',
        fragments: [
          { type: 'emote', text: 'PogChamp' },
          { type: 'text', text: ' that was awesome' },
        ],
      },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 4,
      id: 'preview_mention',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'ChatterBox', color: '#a78bfa' },
      message: {
        plainText: '@Streamer thanks for the shoutout!',
        fragments: [
          { type: 'mention', text: '@Streamer' },
          { type: 'text', text: ' thanks for the shoutout!' },
        ],
      },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 5,
      id: 'preview_long_username',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'ThisIsAnExceptionallyLongTwitchUsername2026', color: '#ef4444' },
      message: { plainText: 'Testing a long username.', fragments: [{ type: 'text', text: 'Testing a long username.' }] },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 6,
      id: 'preview_long_message',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'Wordy', color: '#22c55e' },
      message: {
        plainText:
          'This is a deliberately long message meant to exercise wrapping behavior in the overlay renderer, since a real chat message can run on for quite a while before the viewer stops typing and finally hits enter.',
        fragments: [
          {
            type: 'text',
            text: 'This is a deliberately long message meant to exercise wrapping behavior in the overlay renderer, since a real chat message can run on for quite a while before the viewer stops typing and finally hits enter.',
          },
        ],
      },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 7,
      id: 'preview_anonymous_activity',
      kind: 'activity',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: true },
      activity: { activityType: 'subscription_gift_batch', quantity: 5 },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 8,
      id: 'preview_follow',
      kind: 'activity',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'NewFollower' },
      activity: { activityType: 'follow' },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 9,
      id: 'preview_subscription',
      kind: 'activity',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'LoyalSub', isSubscriber: true },
      activity: { activityType: 'subscription' },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 10,
      id: 'preview_gift_batch',
      kind: 'activity',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'GenerousGifter' },
      activity: { activityType: 'subscription_gift_batch', quantity: 10 },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 11,
      id: 'preview_bits',
      kind: 'activity',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'BitsCheerer' },
      activity: { activityType: 'bits', amount: 500 },
      deleted: false,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 12,
      id: 'preview_deleted',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      user: { anonymous: false, displayName: 'ModeratedUser', color: '#64748b' },
      deleted: true,
      synthetic: true,
    },
    {
      version: 1,
      sequence: 13,
      id: 'preview_no_avatar',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: OCCURRED_AT,
      accountLabel: 'Main Channel',
      user: { anonymous: false, displayName: 'NoAvatarViewer', color: '#8b5cf6' },
      message: { plainText: 'I never set a profile picture.', fragments: [{ type: 'text', text: 'I never set a profile picture.' }] },
      deleted: false,
      synthetic: true,
    },
  ];
}
