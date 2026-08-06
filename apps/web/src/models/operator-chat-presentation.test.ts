import { describe, expect, it } from 'vitest';

import {
  activityTypeKey,
  deletionReasonKey,
  isCommandMessage,
  isDeletedMessage,
  isSafeTwitchAssetUrl,
  kindLabelKey,
  moderationActionKey,
  userDisplayLabel,
} from './operator-chat-presentation';

describe('kindLabelKey', () => {
  it('is exhaustive over every kind', () => {
    expect(kindLabelKey('message')).toBe('kind.message');
    expect(kindLabelKey('activity')).toBe('kind.activity');
    expect(kindLabelKey('moderation')).toBe('kind.moderation');
    expect(kindLabelKey('system')).toBe('kind.system');
  });
});

describe('activityTypeKey', () => {
  it('maps every known activity type', () => {
    expect(activityTypeKey('bits')).toBe('activityType.bits');
    expect(activityTypeKey('subscription_gift_batch')).toBe('activityType.subscription_gift_batch');
    expect(activityTypeKey('gifted_subscription')).toBe('activityType.gifted_subscription');
    expect(activityTypeKey('stream.online')).toBe('activityType.stream_online');
  });

  it('never labels bits as a donation - returns the bits key, not a made-up one', () => {
    expect(activityTypeKey('bits')).not.toBeNull();
    expect(activityTypeKey('bits')).toBe('activityType.bits');
  });

  it('returns null for an unrecognized activity type instead of crashing', () => {
    expect(activityTypeKey('some.future.type')).toBeNull();
  });
});

describe('moderationActionKey', () => {
  it('maps known actions', () => {
    expect(moderationActionKey('chat_cleared')).toBe('moderationAction.chatCleared');
    expect(moderationActionKey('user_messages_cleared')).toBe('moderationAction.userMessagesCleared');
  });

  it('returns null for an unknown action', () => {
    expect(moderationActionKey('unknown_action')).toBeNull();
  });
});

describe('deletionReasonKey', () => {
  it('maps known reasons', () => {
    expect(deletionReasonKey('moderator_deleted')).toBe('deletionReason.moderatorDeleted');
  });

  it('returns null for an unknown reason', () => {
    expect(deletionReasonKey('bogus')).toBeNull();
  });
});

describe('isCommandMessage', () => {
  it('detects a message starting with the default prefix', () => {
    expect(isCommandMessage('!uptime')).toBe(true);
  });

  it('ignores leading whitespace before the prefix', () => {
    expect(isCommandMessage('   !uptime')).toBe(true);
  });

  it('does not treat an embedded ! as a command', () => {
    expect(isCommandMessage('hello! world')).toBe(false);
  });

  it('is false for plain text', () => {
    expect(isCommandMessage('hello chat')).toBe(false);
  });

  it('supports a custom prefix', () => {
    expect(isCommandMessage('~uptime', '~')).toBe(true);
    expect(isCommandMessage('!uptime', '~')).toBe(false);
  });
});

describe('isSafeTwitchAssetUrl', () => {
  it('accepts a real Twitch CDN https URL', () => {
    expect(isSafeTwitchAssetUrl('https://static-cdn.jtvnw.net/emoticons/v2/1/static/dark/2.0')).toBe(
      true,
    );
  });

  it('rejects http (non-https)', () => {
    expect(isSafeTwitchAssetUrl('http://static-cdn.jtvnw.net/emoticons/v2/1/static/dark/2.0')).toBe(
      false,
    );
  });

  it('rejects a data: URL', () => {
    expect(isSafeTwitchAssetUrl('data:image/png;base64,AAAA')).toBe(false);
  });

  it('rejects a javascript: URL', () => {
    expect(isSafeTwitchAssetUrl('javascript:alert(1)')).toBe(false);
  });

  it('rejects an untrusted host', () => {
    expect(isSafeTwitchAssetUrl('https://evil.example.com/image.png')).toBe(false);
  });

  it('rejects a URL carrying userinfo', () => {
    expect(isSafeTwitchAssetUrl('https://user:pass@static-cdn.jtvnw.net/x')).toBe(false);
  });

  it('rejects an unparseable string', () => {
    expect(isSafeTwitchAssetUrl('not a url')).toBe(false);
  });
});

describe('userDisplayLabel', () => {
  it('prefers displayName over login', () => {
    expect(userDisplayLabel({ displayName: 'Streamer', login: 'streamer', anonymous: false })).toBe(
      'Streamer',
    );
  });

  it('falls back to login when displayName is absent', () => {
    expect(userDisplayLabel({ login: 'streamer', anonymous: false })).toBe('streamer');
  });

  it('returns empty for an anonymous user - never a fabricated identity', () => {
    expect(userDisplayLabel({ displayName: 'Should not show', anonymous: true })).toBe('');
  });

  it('returns empty for an undefined user', () => {
    expect(userDisplayLabel(undefined)).toBe('');
  });
});

describe('isDeletedMessage', () => {
  it('is true only for a deleted message item', () => {
    expect(
      isDeletedMessage({
        version: 1,
        sequence: 1,
        id: 'a',
        providerId: 'twitch',
        connectedAccountId: 'acct',
        kind: 'message',
        occurredAt: '2026-08-06T00:00:00Z',
        receivedAt: '2026-08-06T00:00:00Z',
        lifecycle: { deleted: true },
        synthetic: false,
      }),
    ).toBe(true);
  });

  it('is false for an activity item even if somehow marked deleted', () => {
    expect(
      isDeletedMessage({
        version: 1,
        sequence: 1,
        id: 'a',
        providerId: 'twitch',
        connectedAccountId: 'acct',
        kind: 'activity',
        occurredAt: '2026-08-06T00:00:00Z',
        receivedAt: '2026-08-06T00:00:00Z',
        lifecycle: { deleted: true },
        synthetic: false,
      }),
    ).toBe(false);
  });
});
