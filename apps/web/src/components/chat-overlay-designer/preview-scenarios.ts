import type { PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

/**
 * Deterministic, local, synthetic preview fixtures for the Chat Overlay
 * Designer (Stage 13B task Part 27) - frontend-local, never touches the
 * Event Bus, the real queue, real chat, or a real Twitch account.
 * Fixtures are ordinary `PublicChatOverlayItem` values so
 * `chat-item-data-context.ts`'s own `chatItemDataContext` - the exact
 * same mapping the real public overlay route uses - can build each
 * scenario's `VisualDesignDataContext` without a second, duplicated
 * mapping.
 */
export const CHAT_PREVIEW_SCENARIOS = [
  'message',
  'long_message',
  'very_long_username',
  'missing_avatar',
  'no_badges',
  'subscriber_badge',
  'moderator_badge',
  'vip_broadcaster',
  'message_with_emote',
  'message_with_mention',
  'multiple_badges',
  'account_label_present',
  'account_label_absent',
  'follow',
  'subscription',
  'gifted_subscription',
  'subscription_gift_batch',
  'bits',
  'raid',
  'channel_point_redemption',
  'deleted_placeholder',
] as const;
export type ChatPreviewScenario = (typeof CHAT_PREVIEW_SCENARIOS)[number];

const OCCURRED_AT = '2026-08-06T12:00:00Z';
const AVATAR_URL = 'https://static-cdn.jtvnw.net/preview-avatar.png';
const VERY_LONG_USERNAME = 'VeryLongTestViewerName'.repeat(6);
const LONG_MESSAGE = 'This is a very long test chat message used to check text wrapping and clipping. '.repeat(4);

function messageItem(overrides: Partial<PublicChatOverlayItem> = {}): PublicChatOverlayItem {
  return {
    version: 1,
    sequence: 1,
    id: 'preview_message',
    kind: 'message',
    providerId: 'twitch',
    occurredAt: OCCURRED_AT,
    user: { displayName: 'TestViewer', color: '#9146FF', avatarUrl: AVATAR_URL, badges: [], anonymous: false },
    message: { plainText: 'Hello chat!', fragments: [{ type: 'text', text: 'Hello chat!' }] },
    deleted: false,
    synthetic: true,
    ...overrides,
  };
}

function activityItem(activityType: string, overrides: Partial<PublicChatOverlayItem> = {}): PublicChatOverlayItem {
  return {
    version: 1,
    sequence: 1,
    id: `preview_${activityType}`,
    kind: 'activity',
    providerId: 'twitch',
    occurredAt: OCCURRED_AT,
    user: { displayName: 'TestViewer', color: '#9146FF', avatarUrl: AVATAR_URL, badges: [], anonymous: false },
    activity: { activityType },
    deleted: false,
    synthetic: true,
    ...overrides,
  };
}

/** Builds the scenario's own deterministic synthetic item. */
export function chatPreviewScenarioItem(scenario: ChatPreviewScenario): PublicChatOverlayItem {
  switch (scenario) {
    case 'message':
      return messageItem();
    case 'long_message':
      return messageItem({ message: { plainText: LONG_MESSAGE, fragments: [{ type: 'text', text: LONG_MESSAGE }] } });
    case 'very_long_username':
      return messageItem({ user: { ...messageItem().user!, displayName: VERY_LONG_USERNAME } });
    case 'missing_avatar':
      return messageItem({ user: { ...messageItem().user!, avatarUrl: undefined } });
    case 'no_badges':
      return messageItem({ user: { ...messageItem().user!, badges: [] } });
    case 'subscriber_badge':
      return messageItem({ user: { ...messageItem().user!, badges: [{ setId: 'subscriber', id: '12', imageUrl1x: 'https://static-cdn.jtvnw.net/badge-sub.png' }], isSubscriber: true } });
    case 'moderator_badge':
      return messageItem({ user: { ...messageItem().user!, badges: [{ setId: 'moderator', id: '1', imageUrl1x: 'https://static-cdn.jtvnw.net/badge-mod.png' }], isModerator: true } });
    case 'vip_broadcaster':
      return messageItem({
        user: {
          ...messageItem().user!,
          badges: [
            { setId: 'vip', id: '1', imageUrl1x: 'https://static-cdn.jtvnw.net/badge-vip.png' },
            { setId: 'broadcaster', id: '1', imageUrl1x: 'https://static-cdn.jtvnw.net/badge-broadcaster.png' },
          ],
          isVip: true,
          isBroadcaster: true,
        },
      });
    case 'message_with_emote':
      return messageItem({
        message: {
          plainText: 'Hello Kappa!',
          fragments: [
            { type: 'text', text: 'Hello ' },
            { type: 'emote', text: 'Kappa', emoteImageUrl: 'https://static-cdn.jtvnw.net/emoticons/v2/25/default/dark/1.0' },
            { type: 'text', text: '!' },
          ],
        },
      });
    case 'message_with_mention':
      return messageItem({
        message: {
          plainText: '@OtherViewer hi!',
          fragments: [
            { type: 'mention', text: '@OtherViewer' },
            { type: 'text', text: ' hi!' },
          ],
        },
      });
    case 'multiple_badges':
      return messageItem({
        user: {
          ...messageItem().user!,
          badges: [
            { setId: 'moderator', id: '1', imageUrl1x: 'https://static-cdn.jtvnw.net/badge-mod.png' },
            { setId: 'subscriber', id: '12', imageUrl1x: 'https://static-cdn.jtvnw.net/badge-sub.png' },
            { setId: 'vip', id: '1', imageUrl1x: 'https://static-cdn.jtvnw.net/badge-vip.png' },
          ],
        },
      });
    case 'account_label_present':
      return messageItem({ accountLabel: 'Main Channel' });
    case 'account_label_absent':
      return messageItem({ accountLabel: undefined });
    case 'follow':
      return activityItem('follow');
    case 'subscription':
      return activityItem('subscription');
    case 'gifted_subscription':
      return activityItem('gifted_subscription');
    case 'subscription_gift_batch':
      return activityItem('subscription_gift_batch', { activity: { activityType: 'subscription_gift_batch', quantity: 5 } });
    case 'bits':
      return activityItem('bits', { activity: { activityType: 'bits', quantity: 250 } });
    case 'raid':
      return activityItem('raid', { activity: { activityType: 'raid', quantity: 42 } });
    case 'channel_point_redemption':
      return activityItem('channel_point_redemption');
    case 'deleted_placeholder':
      return messageItem({ deleted: true, message: undefined });
  }
}
