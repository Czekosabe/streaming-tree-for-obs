import { describe, expect, it } from 'vitest';

import {
  alertAnimationSchema,
  alertErrorCodeSchema,
  alertEventTypeCapabilitySchema,
  alertEventTypeSchema,
  alertHideReasonSchema,
  alertInterruptModeSchema,
  alertProfileSchema,
  alertQueueStatusSchema,
  alertRoleSchema,
  alertRuleSchema,
  alertSummarySchema,
  listAlertRulesResponseSchema,
  publicAlertHideRevisionPayloadSchema,
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
    'youtube_membership',
    'youtube_membership_milestone',
    'youtube_super_chat',
    'youtube_super_sticker',
    'donation',
  ])('accepts %s', (value) => {
    expect(alertEventTypeSchema.parse(value)).toBe(value);
  });

  it('rejects streamelements_donation - not the real Stage 16A event type name', () => {
    expect(alertEventTypeSchema.safeParse('streamelements_donation').success).toBe(false);
  });
});

describe('alertEventTypeCapabilitySchema', () => {
  it('parses hasAmount/hasMembershipLevel alongside every other capability flag', () => {
    const parsed = alertEventTypeCapabilitySchema.parse({
      eventType: 'youtube_super_chat', hasUser: true, hasMessage: true, hasQuantity: false,
      hasAnonymity: false, hasRewardTitle: false, hasRoles: false, hasAmount: true, hasMembershipLevel: false,
      availablePlaceholders: ['amount', 'currency'], groupable: false, groupingRequiresHiddenMessage: false,
    });
    expect(parsed.hasAmount).toBe(true);
    expect(parsed.hasMembershipLevel).toBe(false);
  });
});

describe('alertErrorCodeSchema', () => {
  it('accepts alert_rule_amount_invalid', () => {
    expect(alertErrorCodeSchema.parse('alert_rule_amount_invalid')).toBe('alert_rule_amount_invalid');
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

describe('alertInterruptModeSchema', () => {
  it.each(['never', 'lower_priority'])('accepts %s', (value) => {
    expect(alertInterruptModeSchema.parse(value)).toBe(value);
  });
  it('rejects an unrecognized mode', () => {
    expect(alertInterruptModeSchema.safeParse('sometimes').success).toBe(false);
  });
});

describe('alertHideReasonSchema', () => {
  it.each(['completed', 'skipped', 'preempted', 'profile_disabled', 'reset'])('accepts %s', (value) => {
    expect(alertHideReasonSchema.parse(value)).toBe(value);
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
      showAmount: false,
      allowGrouping: false, groupWindowMs: 5000, interruptMode: 'never', interruptible: true,
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
    });
    expect(parsed.minimumQuantity).toBeNull();
  });

  it('parses a YouTube Super Chat rule with money fields', () => {
    const parsed = alertRuleSchema.parse({
      id: 'alrule_2', profileId: 'alprof_1', name: 'Super Chat', enabled: true,
      eventType: 'youtube_super_chat', priority: 50, durationMs: 5000,
      requiredRole: 'everyone', showPlatform: true, showUsername: true, showMessage: true, showQuantity: false,
      textTemplate: '{username} sent {amount} {currency}', entryAnimation: 'fade', exitAnimation: 'fade',
      animationDurationMs: 400, providers: ['youtube'], accounts: [],
      currency: 'USD', minimumAmountMicros: 1_000_000, maximumAmountMicros: null, showAmount: true,
      allowGrouping: false, groupWindowMs: 5000, interruptMode: 'never', interruptible: true,
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
    });
    expect(parsed.currency).toBe('USD');
    expect(parsed.minimumAmountMicros).toBe(1_000_000);
    expect(parsed.showAmount).toBe(true);
  });

  it('parses a rule with real quantity bounds and grouping/interruption enabled', () => {
    const parsed = alertRuleSchema.parse({
      id: 'alrule_1', profileId: 'alprof_1', name: 'Bits tier', enabled: true,
      eventType: 'bits', priority: 50, durationMs: 5000,
      minimumQuantity: 100, maximumQuantity: 999, requiredRole: 'everyone',
      showPlatform: true, showUsername: true, showMessage: false, showQuantity: true,
      textTemplate: '{username} cheered {quantity}', entryAnimation: 'fade', exitAnimation: 'fade',
      animationDurationMs: 400, providers: ['twitch'], accounts: ['acct_1'],
      showAmount: false,
      allowGrouping: true, groupWindowMs: 8000, interruptMode: 'lower_priority', interruptible: false,
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
    });
    expect(parsed.minimumQuantity).toBe(100);
    expect(parsed.accounts).toEqual(['acct_1']);
    expect(parsed.allowGrouping).toBe(true);
    expect(parsed.interruptMode).toBe('lower_priority');
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
      synthetic: true, replayed: false, groupCount: 1, interruptible: true,
    });
    expect(parsed.synthetic).toBe(true);
  });

  it('parses a grouped summary', () => {
    const parsed = alertSummarySchema.parse({
      alertId: 'alinst_1', ruleId: 'alrule_1', eventType: 'bits',
      queuedAt: '2026-01-01T00:00:00Z', priority: 50, renderedText: 'Ann cheered 150 bits (x2)',
      quantity: 150, synthetic: false, replayed: false, groupCount: 2, interruptible: false,
    });
    expect(parsed.groupCount).toBe(2);
    expect(parsed.interruptible).toBe(false);
  });
});

describe('alertQueueStatusSchema', () => {
  it('parses an empty queue status', () => {
    const parsed = alertQueueStatusSchema.parse({
      profileId: 'alprof_1', enabled: true, paused: false,
      queuedCount: 0, queueCapacity: 100, nextQueued: [],
      totalEnqueued: 0, totalPlayed: 0, totalExpired: 0, totalCapacityDropped: 0,
      totalManuallySkipped: 0, totalSynthetic: 0,
      totalGroupedMembers: 0, totalGroupsCreated: 0, totalPreempted: 0,
      replayAvailable: false, activeSubscribers: 0, inputGap: false,
    });
    expect(parsed.current).toBeUndefined();
    expect(parsed.totalPreempted).toBe(0);
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
      synthetic: false, replayed: false, renderedText: 'Ann followed!', groupCount: 1,
      durationMs: 5000, entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    });
    expect(parsed.username).toBeUndefined();
  });

  it('parses an anonymous alert (null username)', () => {
    const parsed = publicAlertSchema.parse({
      schemaVersion: 1, alertId: 'alinst_1', eventType: 'bits', providerId: 'twitch',
      synthetic: false, replayed: false, username: null, quantity: 500, groupCount: 1,
      renderedText: 'An anonymous cheerer gave 500 bits!',
      durationMs: 5000, entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    });
    expect(parsed.username).toBeNull();
    expect(parsed.quantity).toBe(500);
  });

  it('parses a grouped alert (groupCount > 1)', () => {
    const parsed = publicAlertSchema.parse({
      schemaVersion: 1, alertId: 'alinst_1', eventType: 'bits', providerId: 'twitch',
      synthetic: false, replayed: false, quantity: 150, groupCount: 3,
      renderedText: 'Ann cheered 150 bits (x3)',
      durationMs: 5000, entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    });
    expect(parsed.groupCount).toBe(3);
  });
});

describe('publicAlertRevisionPayloadSchema', () => {
  it('parses a reset payload with a null alert', () => {
    const parsed = publicAlertRevisionPayloadSchema.parse({ paused: false, alert: null });
    expect(parsed.alert).toBeNull();
  });
});

describe('publicAlertHideRevisionPayloadSchema', () => {
  it('parses a hide payload (id and reason, never rendered content)', () => {
    const parsed = publicAlertHideRevisionPayloadSchema.parse({
      paused: false, alertId: 'alinst_1', reason: 'preempted',
    });
    expect(parsed.alertId).toBe('alinst_1');
    expect(parsed.reason).toBe('preempted');
  });

  it('rejects a payload shaped like the general revision schema instead ({alert}, not {alertId, reason})', () => {
    expect(publicAlertHideRevisionPayloadSchema.safeParse({ paused: false, alert: null }).success).toBe(false);
  });
});
