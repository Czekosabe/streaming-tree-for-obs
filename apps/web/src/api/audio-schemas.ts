/**
 * Zod contracts for the Stage 17A TTS/audio API
 * (internal/httpapi/audio.go). Response shapes only - request bodies
 * are hand-written TypeScript types (never independently validated
 * client-side), mirroring api/alerts-schemas.ts's own convention.
 */

import { z } from 'zod';

/** internal/domain/audio.ProviderMode's own known values - see
 * models/audio.ts's own AUDIO_PROVIDER_MODES for the same list kept
 * next to the other client-side bounds. */
export const audioProviderModeSchema = z.enum(['disabled', 'system', 'local', 'cloud']);
export type AudioProviderMode = z.infer<typeof audioProviderModeSchema>;

export const audioSettingsSchema = z.object({
  enabled: z.boolean(),
  providerMode: audioProviderModeSchema,
  enabledEventTypes: z.array(z.string()),
  enabledProviderIds: z.array(z.string()),
  enabledSourceIds: z.array(z.string()),
  supporterOnlyMode: z.boolean(),
  thresholdCurrency: z.string().optional(),
  thresholdMinimumAmountMicros: z.number().nullable().optional(),
  minimumBits: z.number().nullable().optional(),
  maxTextLengthCodePoints: z.number(),
  perUserCooldownSeconds: z.number(),
  globalCooldownSeconds: z.number(),
  blockedWords: z.array(z.string()),
  removeUrls: z.boolean(),
  normalizeRepeatedChars: z.boolean(),
  suppressCommands: z.boolean(),
  queueCapacity: z.number(),
  manualApproval: z.boolean(),
  voiceId: z.string(),
  language: z.string(),
  speed: z.number(),
  volume: z.number(),
  publicSlug: z.string(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type AudioSettings = z.infer<typeof audioSettingsSchema>;

/** Request body for PUT /api/audio/settings - deliberately omits
 * publicSlug/createdAt/updatedAt: none of them is ever client-settable
 * (the public slug changes only via POST rotate-slug), and the
 * backend's own strict unknown-field rejection would reject them
 * anyway if this type allowed them through. */
export type AudioSettingsInput = {
  enabled: boolean;
  providerMode: AudioProviderMode;
  enabledEventTypes: string[];
  enabledProviderIds: string[];
  enabledSourceIds: string[];
  supporterOnlyMode: boolean;
  thresholdCurrency: string;
  thresholdMinimumAmountMicros?: number | null;
  minimumBits?: number | null;
  maxTextLengthCodePoints: number;
  perUserCooldownSeconds: number;
  globalCooldownSeconds: number;
  blockedWords: string[];
  removeUrls: boolean;
  normalizeRepeatedChars: boolean;
  suppressCommands: boolean;
  queueCapacity: number;
  manualApproval: boolean;
  voiceId: string;
  language: string;
  speed: number;
  volume: number;
};

export const audioCapabilitiesSchema = z.object({
  knownProviderModes: z.array(z.string()),
  implementedProviderModes: z.array(z.string()),
  systemProviderAvailable: z.boolean(),
  systemProviderReason: z.string().optional(),
});
export type AudioCapabilities = z.infer<typeof audioCapabilitiesSchema>;

export const audioVoiceSchema = z.object({
  id: z.string(),
  name: z.string().optional(),
  language: z.string().optional(),
  gender: z.string().optional(),
  isDefault: z.boolean(),
});
export type AudioVoice = z.infer<typeof audioVoiceSchema>;

export const audioStatusSchema = z.object({
  enabled: z.boolean(),
  providerMode: audioProviderModeSchema,
  providerAvailable: z.boolean(),
  rendererConnected: z.boolean(),
  hasCurrentItem: z.boolean(),
  currentSynthetic: z.boolean(),
  pendingApprovalCount: z.number(),
  readyQueueCount: z.number(),
  capacity: z.number(),
  totalEnqueued: z.number(),
  totalCapacityDropped: z.number(),
  totalExpired: z.number(),
  totalRejected: z.number(),
  totalManuallySkipped: z.number(),
  totalSynthetic: z.number(),
  totalPlayed: z.number(),
  totalPlaybackFailed: z.number(),
  totalSynthesisFailed: z.number(),
  totalInterrupted: z.number(),
  inputGap: z.boolean(),
  subscribed: z.boolean(),
});
export type AudioStatus = z.infer<typeof audioStatusSchema>;

export const audioPendingItemSchema = z.object({
  id: z.string(),
  text: z.string(),
  enqueuedAt: z.string(),
  expiresAt: z.string().optional(),
});
export type AudioPendingItem = z.infer<typeof audioPendingItemSchema>;

/** Payload of the public `audio.reset` SSE event - carries the
 * ephemeral renderer session token this browser tab must echo back on
 * every POST .../ack call. Never logged, never persisted (kept only in
 * this hook's own React state). */
export const publicAudioResetPayloadSchema = z.object({
  rendererToken: z.string(),
});
export type PublicAudioResetPayload = z.infer<typeof publicAudioResetPayloadSchema>;

/** Payload of the public `audio.current` SSE event - the safe,
 * bounded summary docs/audio-tts.md §19 describes: no source event, no
 * account/user id, no message text beyond what the audio itself
 * already contains. */
export const publicAudioCurrentPayloadSchema = z.object({
  itemId: z.string(),
  bytesUrl: z.string(),
  contentType: z.string(),
  volume: z.number(),
});
export type PublicAudioCurrentPayload = z.infer<typeof publicAudioCurrentPayloadSchema>;

export const publicAudioGapPayloadSchema = z.object({
  reason: z.string(),
});
export type PublicAudioGapPayload = z.infer<typeof publicAudioGapPayloadSchema>;

/** Stable error codes internal/httpapi/audio.go's own writeAudioError
 * funnel returns. */
export const audioErrorCodeSchema = z.enum([
  'audio_disabled',
  'audio_text_empty',
  'audio_queue_full',
  'audio_voice_not_found',
  'audio_provider_unavailable',
  'audio_settings_invalid',
  'audio_item_not_found',
  'audio_not_available',
  'audio_ack_kind_invalid',
  'audio_ack_rejected',
]);
export type AudioErrorCode = z.infer<typeof audioErrorCodeSchema>;
