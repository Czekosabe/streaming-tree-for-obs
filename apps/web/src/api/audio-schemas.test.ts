import { describe, expect, it } from 'vitest';

import {
  audioCapabilitiesSchema,
  audioPendingItemSchema,
  audioProviderModeSchema,
  audioSettingsSchema,
  audioStatusSchema,
  audioVoiceSchema,
  publicAudioCurrentPayloadSchema,
  publicAudioGapPayloadSchema,
  publicAudioResetPayloadSchema,
} from './audio-schemas';

describe('audioProviderModeSchema', () => {
  it.each(['disabled', 'system', 'local', 'cloud'])('accepts %s', (value) => {
    expect(audioProviderModeSchema.parse(value)).toBe(value);
  });

  it('rejects an unknown provider mode', () => {
    expect(audioProviderModeSchema.safeParse('not-a-real-mode').success).toBe(false);
  });
});

function fullAudioSettings() {
  return {
    enabled: true,
    providerMode: 'system',
    enabledEventTypes: ['chat.message'],
    enabledProviderIds: ['twitch'],
    enabledSourceIds: [],
    supporterOnlyMode: false,
    thresholdCurrency: 'USD',
    thresholdMinimumAmountMicros: 5_000_000,
    minimumBits: 100,
    maxTextLengthCodePoints: 500,
    perUserCooldownSeconds: 30,
    globalCooldownSeconds: 3,
    blockedWords: ['spam'],
    removeUrls: true,
    normalizeRepeatedChars: true,
    suppressCommands: true,
    queueCapacity: 100,
    manualApproval: false,
    voiceId: 'voice-1',
    language: 'en-US',
    speed: 1.0,
    volume: 1.0,
    publicSlug: 'abc123',
    createdAt: '2026-08-13T00:00:00Z',
    updatedAt: '2026-08-13T00:00:00Z',
  };
}

describe('audioSettingsSchema', () => {
  it('parses a full settings object', () => {
    const parsed = audioSettingsSchema.parse(fullAudioSettings());
    expect(parsed.publicSlug).toBe('abc123');
    expect(parsed.thresholdMinimumAmountMicros).toBe(5_000_000);
  });

  it('accepts null threshold/bits (no threshold configured)', () => {
    const settings = { ...fullAudioSettings(), thresholdMinimumAmountMicros: null, minimumBits: null };
    const parsed = audioSettingsSchema.parse(settings);
    expect(parsed.thresholdMinimumAmountMicros).toBeNull();
    expect(parsed.minimumBits).toBeNull();
  });

  it('accepts a response omitting the optional threshold fields entirely', () => {
    const { thresholdCurrency, thresholdMinimumAmountMicros, minimumBits, ...rest } = fullAudioSettings();
    void thresholdCurrency;
    void thresholdMinimumAmountMicros;
    void minimumBits;
    expect(audioSettingsSchema.safeParse(rest).success).toBe(true);
  });

  it('rejects a settings object missing required fields', () => {
    expect(audioSettingsSchema.safeParse({ enabled: true }).success).toBe(false);
  });
});

describe('audioCapabilitiesSchema', () => {
  it('parses a capabilities response', () => {
    const parsed = audioCapabilitiesSchema.parse({
      knownProviderModes: ['disabled', 'system', 'local', 'cloud'],
      implementedProviderModes: ['disabled', 'system'],
      systemProviderAvailable: true,
    });
    expect(parsed.systemProviderAvailable).toBe(true);
  });

  it('parses an unavailable provider with a reason', () => {
    const parsed = audioCapabilitiesSchema.parse({
      knownProviderModes: [],
      implementedProviderModes: [],
      systemProviderAvailable: false,
      systemProviderReason: 'not on Windows',
    });
    expect(parsed.systemProviderReason).toBe('not on Windows');
  });
});

describe('audioVoiceSchema', () => {
  it('parses a voice with every field', () => {
    const parsed = audioVoiceSchema.parse({
      id: 'voice-1', name: 'Test Voice', language: 'en-US', gender: 'Female', isDefault: true,
    });
    expect(parsed.id).toBe('voice-1');
  });

  it('parses a voice with only id and isDefault', () => {
    expect(audioVoiceSchema.safeParse({ id: 'voice-1', isDefault: false }).success).toBe(true);
  });
});

describe('audioStatusSchema', () => {
  it('parses a full status response', () => {
    const parsed = audioStatusSchema.parse({
      enabled: false, providerMode: 'disabled', providerAvailable: true,
      rendererConnected: false, hasCurrentItem: false, currentSynthetic: false,
      pendingApprovalCount: 0, readyQueueCount: 0, capacity: 100,
      totalEnqueued: 0, totalCapacityDropped: 0, totalExpired: 0, totalRejected: 0,
      totalManuallySkipped: 0, totalSynthetic: 0, totalPlayed: 0, totalPlaybackFailed: 0,
      totalSynthesisFailed: 0, totalInterrupted: 0, inputGap: false, subscribed: true,
    });
    expect(parsed.capacity).toBe(100);
  });
});

describe('audioPendingItemSchema', () => {
  it('parses a pending item with an expiry', () => {
    const parsed = audioPendingItemSchema.parse({
      id: 'auditem_1', text: 'hello', enqueuedAt: '2026-08-13T00:00:00Z', expiresAt: '2026-08-13T00:05:00Z',
    });
    expect(parsed.id).toBe('auditem_1');
  });

  it('parses a pending item without an expiry', () => {
    expect(
      audioPendingItemSchema.safeParse({ id: 'auditem_1', text: 'hello', enqueuedAt: '2026-08-13T00:00:00Z' })
        .success,
    ).toBe(true);
  });
});

describe('public audio SSE payload schemas', () => {
  it('parses a reset payload', () => {
    expect(publicAudioResetPayloadSchema.parse({ rendererToken: 'tok' }).rendererToken).toBe('tok');
  });

  it('parses a current payload', () => {
    const parsed = publicAudioCurrentPayloadSchema.parse({
      itemId: 'auditem_1', bytesUrl: '/api/public/audio/slug/bytes/tok', contentType: 'audio/wav', volume: 1,
    });
    expect(parsed.contentType).toBe('audio/wav');
  });

  it('parses a gap payload', () => {
    expect(publicAudioGapPayloadSchema.parse({ reason: 'unknown_slug' }).reason).toBe('unknown_slug');
  });

  it('rejects a current payload leaking an unexpected shape', () => {
    expect(publicAudioCurrentPayloadSchema.safeParse({ itemId: 'x' }).success).toBe(false);
  });
});
