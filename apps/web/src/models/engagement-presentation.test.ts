import { describe, expect, it } from 'vitest';

import type { ConnectorState, EngagementEventType } from '@/api/engagement-schemas';

import {
  connectorBlockerKey,
  connectorStateKey,
  connectorStateTone,
  eventSummary,
  eventTypeKey,
} from './engagement-presentation';

const ALL_STATES: ConnectorState[] = [
  'disabled',
  'blocked',
  'connecting',
  'waiting_for_welcome',
  'subscribing',
  'connected',
  'reconnecting',
  'stopping',
  'error',
];

const ALL_EVENT_TYPES: EngagementEventType[] = [
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
];

describe('connectorStateKey', () => {
  it('maps every connector state to a key, exhaustively', () => {
    for (const state of ALL_STATES) {
      expect(connectorStateKey(state)).toMatch(/^connector\.state\./);
    }
  });
});

describe('connectorStateTone', () => {
  it('maps connected to live', () => {
    expect(connectorStateTone('connected')).toBe('live');
  });

  it('maps blocked and error to error tone', () => {
    expect(connectorStateTone('blocked')).toBe('error');
    expect(connectorStateTone('error')).toBe('error');
  });

  it('maps disabled to offline', () => {
    expect(connectorStateTone('disabled')).toBe('offline');
  });

  it('maps every in-progress state to starting', () => {
    for (const state of ['connecting', 'waiting_for_welcome', 'subscribing', 'reconnecting', 'stopping'] as const) {
      expect(connectorStateTone(state)).toBe('starting');
    }
  });

  it('produces a tone for every known state without throwing', () => {
    for (const state of ALL_STATES) {
      expect(() => connectorStateTone(state)).not.toThrow();
    }
  });
});

describe('eventTypeKey', () => {
  it('maps every normalized event type to a key, exhaustively', () => {
    for (const type of ALL_EVENT_TYPES) {
      expect(eventTypeKey(type)).toMatch(/^eventType\./);
    }
  });

  it('gives gifted_subscription and subscription_gift_batch distinct keys', () => {
    expect(eventTypeKey('gifted_subscription')).not.toBe(eventTypeKey('subscription_gift_batch'));
  });
});

describe('connectorBlockerKey', () => {
  it('maps a known blocker code', () => {
    expect(connectorBlockerKey('engagement_scope_upgrade_required')).toBe(
      'connector.blockers.scopeUpgradeRequired',
    );
  });

  it('returns null for an unrecognised blocker code rather than guessing', () => {
    expect(connectorBlockerKey('some_future_blocker')).toBeNull();
  });
});

describe('eventSummary', () => {
  it('shows the display name for a non-anonymous follow', () => {
    const summary = eventSummary({
      type: 'follow',
      user: { displayName: 'Viewer', anonymous: false },
    });
    expect(summary.actor).toBe('Viewer');
  });

  it('never surfaces an identity for an anonymous user', () => {
    const summary = eventSummary({
      type: 'bits',
      user: { displayName: 'ShouldNeverAppear', anonymous: true },
      quantity: 100,
    });
    expect(summary.actor).toBe('');
  });

  it('falls back to login when no display name is present', () => {
    const summary = eventSummary({
      type: 'follow',
      user: { login: 'viewer_login', anonymous: false },
    });
    expect(summary.actor).toBe('viewer_login');
  });

  it('surfaces chat message text as the detail', () => {
    const summary = eventSummary({
      type: 'chat.message',
      user: { displayName: 'Viewer', anonymous: false },
      message: { text: 'hello there' },
    });
    expect(summary.detail).toBe('hello there');
  });

  it('handles an event with no user at all (e.g. stream.online)', () => {
    const summary = eventSummary({ type: 'stream.online' });
    expect(summary.actor).toBe('');
    expect(summary.detail).toBe('');
  });

  it('shows the provider-rendered display amount for a Super Chat, never the raw message when both are absent', () => {
    const summary = eventSummary({
      type: 'youtube.super_chat',
      user: { displayName: 'Viewer', anonymous: false },
      displayAmount: '$5.00',
    });
    expect(summary.detail).toBe('$5.00');
  });

  it('falls back to the message when a Super Chat has no displayAmount', () => {
    const summary = eventSummary({
      type: 'youtube.super_chat',
      user: { displayName: 'Viewer', anonymous: false },
      message: { text: 'thanks for the stream!' },
    });
    expect(summary.detail).toBe('thanks for the stream!');
  });

  it('shows the display amount for a Super Sticker (no message capability)', () => {
    const summary = eventSummary({
      type: 'youtube.super_sticker',
      user: { displayName: 'Viewer', anonymous: false },
      displayAmount: '€2.00',
    });
    expect(summary.detail).toBe('€2.00');
  });

  it('shows no detail for a plain membership event', () => {
    const summary = eventSummary({
      type: 'youtube.membership',
      user: { displayName: 'Viewer', anonymous: false },
    });
    expect(summary.detail).toBe('');
  });

  it('shows the milestone message as detail', () => {
    const summary = eventSummary({
      type: 'youtube.membership_milestone',
      user: { displayName: 'Viewer', anonymous: false },
      message: { text: '6 months and counting!' },
    });
    expect(summary.detail).toBe('6 months and counting!');
  });
});
