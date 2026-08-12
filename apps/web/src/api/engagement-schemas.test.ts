import { describe, expect, it } from 'vitest';

import {
  accountEngagementSchema,
  connectorSchema,
  connectorStateSchema,
  engagementEventSchema,
  engagementEventsResponseSchema,
  engagementStatusSchema,
  eventTypeSchema,
  fragmentSchema,
} from './engagement-schemas';

describe('connectorStateSchema', () => {
  it.each([
    'disabled',
    'blocked',
    'connecting',
    'waiting_for_welcome',
    'subscribing',
    'connected',
    'reconnecting',
    'stopping',
    'error',
  ])('accepts %s', (state) => {
    expect(connectorStateSchema.safeParse(state).success).toBe(true);
  });

  it('rejects an unrecognised state', () => {
    expect(connectorStateSchema.safeParse('somewhere-else').success).toBe(false);
  });
});

describe('connectorSchema', () => {
  it('accepts a minimal disabled connector', () => {
    const result = connectorSchema.safeParse({
      accountId: 'acct_1',
      provider: 'twitch',
      enabled: false,
      state: 'disabled',
      reconnectCount: 0,
      activeSubscriptionCount: 0,
      expectedSubscriptionCount: 0,
    });
    expect(result.success).toBe(true);
  });

  it('never carries a session id, reconnect URL, or token-shaped field', () => {
    const result = connectorSchema.safeParse({
      accountId: 'acct_1',
      provider: 'twitch',
      enabled: true,
      state: 'connected',
      reconnectCount: 0,
      activeSubscriptionCount: 13,
      expectedSubscriptionCount: 13,
      sessionId: 'should-be-rejected',
    });
    // Zod's default behaviour strips unknown keys rather than rejecting -
    // the structural guarantee is that the schema has no field to parse
    // such a value INTO, verified by the parsed result never exposing it.
    expect(result.success).toBe(true);
    if (result.success) {
      expect('sessionId' in result.data).toBe(false);
    }
  });

  it('accepts a YouTube connector with its own additive fields and no subscription counts', () => {
    const result = connectorSchema.safeParse({
      accountId: 'acct_1',
      provider: 'youtube',
      enabled: true,
      state: 'connected',
      reconnectCount: 0,
      selectedBroadcastId: 'broadcast_1',
      lastPollAt: '2026-08-12T00:00:00Z',
      possibleGapCount: 0,
      unsupportedEventCount: 2,
    });
    expect(result.success).toBe(true);
  });
});

describe('accountEngagementSchema', () => {
  it('accepts a blocked account with a capability assessment', () => {
    const result = accountEngagementSchema.safeParse({
      accountId: 'acct_1',
      provider: 'twitch',
      enabled: true,
      state: 'blocked',
      blockerCodes: ['engagement_scope_upgrade_required'],
      missingScopes: ['user:read:chat'],
      reconnectCount: 0,
      activeSubscriptionCount: 0,
      expectedSubscriptionCount: 0,
      requiredScopes: ['user:read:chat', 'moderator:read:followers'],
      grantedScopes: ['channel:manage:broadcast'],
      permissionUpgradeRequired: true,
    });
    expect(result.success).toBe(true);
  });

  it('accepts a YouTube account (never a scope-upgrade capability assessment)', () => {
    const result = accountEngagementSchema.safeParse({
      accountId: 'acct_1',
      provider: 'youtube',
      enabled: true,
      state: 'connected',
      reconnectCount: 0,
      requiredScopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
      grantedScopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
      permissionUpgradeRequired: false,
    });
    expect(result.success).toBe(true);
  });
});

describe('engagementStatusSchema', () => {
  it('accepts a status with no connectors yet', () => {
    const result = engagementStatusSchema.safeParse({
      schemaVersion: 1,
      bufferCapacity: 1000,
      retainedCount: 0,
      oldestSequence: 0,
      newestSequence: 0,
      activeSubscribers: 0,
      connectors: [],
    });
    expect(result.success).toBe(true);
  });
});

describe('eventTypeSchema', () => {
  it.each([
    'chat.message',
    'chat.message_deleted',
    'chat.cleared',
    'moderation',
    'follow',
    'subscription',
    'resubscription',
    'gifted_subscription',
    'subscription_gift_batch',
    'bits',
    'raid',
    'channel_point_redemption',
    'stream.online',
    'stream.offline',
    'youtube.membership',
    'youtube.membership_milestone',
    'youtube.super_chat',
    'youtube.super_sticker',
  ])('accepts %s', (type) => {
    expect(eventTypeSchema.safeParse(type).success).toBe(true);
  });
});

describe('fragmentSchema', () => {
  it('accepts an unknown fragment type (forward compatibility with future Twitch types)', () => {
    const result = fragmentSchema.safeParse({ type: 'unknown', text: '???' });
    expect(result.success).toBe(true);
  });

  it('rejects a fragment type outside the known set', () => {
    const result = fragmentSchema.safeParse({ type: 'not-a-real-type', text: 'x' });
    expect(result.success).toBe(false);
  });
});

describe('engagementEventSchema', () => {
  it('accepts a minimal follow event', () => {
    const result = engagementEventSchema.safeParse({
      schemaVersion: 1,
      sequence: 1,
      id: 'evt_1',
      providerId: 'twitch',
      connectedAccountId: 'acct_1',
      type: 'follow',
      providerEventType: 'channel.follow',
      platformTimestamp: '2026-08-05T12:00:00Z',
      receivedAt: '2026-08-05T12:00:00Z',
      synthetic: false,
      user: { providerUserId: 'u1', anonymous: false },
    });
    expect(result.success).toBe(true);
  });

  it('accepts a chat message with ordered fragments', () => {
    const result = engagementEventSchema.safeParse({
      schemaVersion: 1,
      sequence: 2,
      id: 'evt_2',
      providerId: 'twitch',
      connectedAccountId: 'acct_1',
      type: 'chat.message',
      providerEventType: 'channel.chat.message',
      platformTimestamp: '2026-08-05T12:00:00Z',
      receivedAt: '2026-08-05T12:00:00Z',
      synthetic: false,
      user: { providerUserId: 'u1', login: 'viewer', anonymous: false },
      message: {
        text: 'hello Kappa',
        fragments: [
          { type: 'text', text: 'hello ' },
          { type: 'emote', text: 'Kappa', emoteId: '25' },
        ],
      },
    });
    expect(result.success).toBe(true);
  });

  it('rejects an event missing required fields', () => {
    const result = engagementEventSchema.safeParse({ type: 'follow' });
    expect(result.success).toBe(false);
  });

  it('accepts an anonymous gift-batch event with no fabricated identity', () => {
    const result = engagementEventSchema.safeParse({
      schemaVersion: 1,
      sequence: 3,
      id: 'evt_3',
      providerId: 'twitch',
      connectedAccountId: 'acct_1',
      type: 'subscription_gift_batch',
      providerEventType: 'channel.subscription.gift',
      platformTimestamp: '2026-08-05T12:00:00Z',
      receivedAt: '2026-08-05T12:00:00Z',
      synthetic: false,
      quantity: 5,
      user: { anonymous: true },
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.user?.providerUserId).toBeUndefined();
    }
  });

  it('accepts a YouTube Super Chat event with integer-micros amount fields', () => {
    const result = engagementEventSchema.safeParse({
      schemaVersion: 1,
      sequence: 4,
      id: 'evt_4',
      providerId: 'youtube',
      connectedAccountId: 'acct_1',
      type: 'youtube.super_chat',
      providerEventType: 'superChatEvent',
      platformTimestamp: '2026-08-05T12:00:00Z',
      receivedAt: '2026-08-05T12:00:00Z',
      synthetic: false,
      user: { providerUserId: 'u1', anonymous: false },
      message: { text: 'thanks!', fragments: [] },
      amountMicros: 5_000_000,
      currency: 'USD',
      displayAmount: '$5.00',
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.amountMicros).toBe(5_000_000);
      expect(result.data.displayAmount).toBe('$5.00');
    }
  });
});

describe('engagementEventsResponseSchema', () => {
  it('accepts an empty items list with gap=true', () => {
    const result = engagementEventsResponseSchema.safeParse({ items: [], gap: true });
    expect(result.success).toBe(true);
  });
});
