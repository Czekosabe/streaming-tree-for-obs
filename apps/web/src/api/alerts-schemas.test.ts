import { describe, expect, it } from 'vitest';

import {
  alertAnimationSchema,
  alertEventTypeSchema,
  alertProfileSchema,
  alertQueueStatusSchema,
  alertRoleSchema,
  alertRuleSchema,
  alertSummarySchema,
  listAlertRulesResponseSchema,
  publicAlertProfileConfigSchema,
  publicAlertRevisionPayloadSchema,
  publicAlertSchema,
} from './alerts-schemas';

describe('alertEventTypeSchema', () => {
  it.each([
    'follow',
    'subscription',
    'resubscription',
    'gifted_subscription',
    'subscription_gift_batch',
    'bits',
    'raid',
    'channel_point_redemption',
  ])('accepts %s', (value) => {
    expect(alertEventTypeSchema.parse(value)).toBe(value);
  });

  it('rejects donation - not a real Stage 12A event type', () => {
    expect(alertEventTypeSchema.safeParse('donation').success).toBe(false);
  });
});

describe('alertRoleSchema', () => {
  it.each(['everyone', 'subscriber', 'vip', 'moderator', 'broadcaster'])('accepts %s', (value) => {
    expect(alertRoleSchema.parse(value)).toBe(value);
  });
});

describe('alertAnimationSchema', () => {
  it.each(['none', 'fade', 'slide_up', 'slide_left', 'scale'])('accepts %s', (value) => {
    expect(alertAnimationSchema.parse(value)).toBe(value);
  });
});

describe('alertProfileSchema', () => {
  it('parses a full profile', () => {
    const parsed = alertProfileSchema.parse({
      id: 'alprof_1', publicSlug: 'abc123', name: 'Main', enabled: true,
      language: 'en', theme: 'minimal', position: 'bottom', textAlign: 'center',
      maxQueueItems: 100, maximumQueueAgeSeconds: 120,
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
    });
    expect(parsed.publicSlug).toBe('abc123');
  });

  it('rejects a profile missing required fields', () => {
    expect(alertProfileSchema.safeParse({ id: 'alprof_1' }).success).toBe(false);
  });
});

describe('alertRuleSchema', () => {
  it('parses a full rule with null thresholds', () => {
    const parsed = alertRuleSchema.parse({
      id: 'alrule_1', profileId: 'alprof_1', name: 'Follow', enabled: true,
      eventType: 'follow', priority: 50, durationMs: 5000,
      minimumQuantity: null, maximumQuantity: null, requiredRole: 'everyone',
      showPlatform: true, showUsername: true, showMessage: false, showQuantity: false,
      textTemplate: '{username} followed!', entryAnimation: 'fade', exitAnimation: 'fade',
      animationDurationMs: 400, providers: [], accounts: [],
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
    });
    expect(parsed.minimumQuantity).toBeNull();
  });

  it('parses a rule with real quantity bounds', () => {
    const parsed = alertRuleSchema.parse({
      id: 'alrule_1', profileId: 'alprof_1', name: 'Bits tier', enabled: true,
      eventType: 'bits', priority: 50, durationMs: 5000,
      minimumQuantity: 100, maximumQuantity: 999, requiredRole: 'everyone',
      showPlatform: true, showUsername: true, showMessage: false, showQuantity: true,
      textTemplate: '{username} cheered {quantity}', entryAnimation: 'fade', exitAnimation: 'fade',
      animationDurationMs: 400, providers: ['twitch'], accounts: ['acct_1'],
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
    });
    expect(parsed.minimumQuantity).toBe(100);
    expect(parsed.accounts).toEqual(['acct_1']);
  });
});

describe('listAlertRulesResponseSchema', () => {
  it('parses rules with an overlap warning', () => {
    const parsed = listAlertRulesResponseSchema.parse({
      rules: [],
      overlapWarnings: [{ ruleId: 'a', otherRuleId: 'b', eventType: 'bits' }],
    });
    expect(parsed.overlapWarnings).toHaveLength(1);
  });
});

describe('alertSummarySchema', () => {
  it('parses a synthetic summary', () => {
    const parsed = alertSummarySchema.parse({
      alertId: 'alinst_1', ruleId: 'alrule_1', eventType: 'follow',
      queuedAt: '2026-01-01T00:00:00Z', priority: 50, renderedText: 'Ann followed!',
      synthetic: true, replayed: false,
    });
    expect(parsed.synthetic).toBe(true);
  });
});

describe('alertQueueStatusSchema', () => {
  it('parses an empty queue status', () => {
    const parsed = alertQueueStatusSchema.parse({
      profileId: 'alprof_1', enabled: true, paused: false,
      queuedCount: 0, queueCapacity: 100, nextQueued: [],
      totalEnqueued: 0, totalPlayed: 0, totalExpired: 0, totalCapacityDropped: 0,
      totalManuallySkipped: 0, totalSynthetic: 0, replayAvailable: false,
      activeSubscribers: 0, inputGap: false,
    });
    expect(parsed.current).toBeUndefined();
  });
});

describe('publicAlertProfileConfigSchema', () => {
  it('parses the fixed presentation fields only', () => {
    const parsed = publicAlertProfileConfigSchema.parse({
      schemaVersion: 1, theme: 'minimal', position: 'bottom', textAlign: 'center', language: 'en',
    });
    expect(parsed.theme).toBe('minimal');
  });

  it('rejects a management-only field being required', () => {
    // Extra fields are ignored by Zod's default (non-strict) parsing,
    // but the type this schema produces still exposes only these four
    // presentation fields plus schemaVersion - see PublicAlertProfileConfig.
    const parsed = publicAlertProfileConfigSchema.parse({
      schemaVersion: 1, theme: 'minimal', position: 'bottom', textAlign: 'center', language: 'en',
      id: 'alprof_1', maxQueueItems: 100,
    });
    expect('id' in parsed).toBe(false);
  });
});

describe('publicAlertSchema', () => {
  it('parses a minimal alert with no user/message/quantity', () => {
    const parsed = publicAlertSchema.parse({
      schemaVersion: 1, alertId: 'alinst_1', eventType: 'follow', providerId: 'twitch',
      synthetic: false, replayed: false, renderedText: 'Ann followed!',
      durationMs: 5000, entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    });
    expect(parsed.username).toBeUndefined();
  });

  it('parses an anonymous alert (null username)', () => {
    const parsed = publicAlertSchema.parse({
      schemaVersion: 1, alertId: 'alinst_1', eventType: 'bits', providerId: 'twitch',
      synthetic: false, replayed: false, username: null, quantity: 500,
      renderedText: 'An anonymous cheerer gave 500 bits!',
      durationMs: 5000, entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    });
    expect(parsed.username).toBeNull();
    expect(parsed.quantity).toBe(500);
  });
});

describe('publicAlertRevisionPayloadSchema', () => {
  it('parses a hide payload (null alert)', () => {
    const parsed = publicAlertRevisionPayloadSchema.parse({ paused: false, alert: null });
    expect(parsed.alert).toBeNull();
  });
});
