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
});

describe('accountEngagementSchema', () => {
  it('accepts a blocked account with a capability assessment', () => {
    const result = accountEngagementSchema.safeParse({
      accountId: 'acct_1',
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
});

describe('engagementEventsResponseSchema', () => {
  it('accepts an empty items list with gap=true', () => {
    const result = engagementEventsResponseSchema.safeParse({ items: [], gap: true });
    expect(result.success).toBe(true);
  });
});
