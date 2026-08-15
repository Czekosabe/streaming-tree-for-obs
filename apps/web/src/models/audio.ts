/**
 * Pure, backend-mirroring validation/bounds helpers for Stage 17A
 * audio/TTS. internal/domain/audio's own ValidateSettings remains the
 * real authority for every bound below - these exist only for live
 * client-side feedback before a submit.
 */

/** Counts Unicode code points the same way the backend does - see
 * models/alerts.ts's own identical helper. */
export function codePointLength(text: string): number {
  return Array.from(text).length;
}

export const MIN_QUEUE_CAPACITY = 10;
export const MAX_QUEUE_CAPACITY = 500;
export const DEFAULT_QUEUE_CAPACITY = 100;

export const MIN_MAX_TEXT_LENGTH_CODE_POINTS = 50;
export const MAX_MAX_TEXT_LENGTH_CODE_POINTS = 2000;
export const DEFAULT_MAX_TEXT_LENGTH_CODE_POINTS = 500;

export const MIN_PER_USER_COOLDOWN_SECONDS = 0;
export const MAX_PER_USER_COOLDOWN_SECONDS = 3600;
export const DEFAULT_PER_USER_COOLDOWN_SECONDS = 30;

export const MIN_GLOBAL_COOLDOWN_SECONDS = 0;
export const MAX_GLOBAL_COOLDOWN_SECONDS = 300;
export const DEFAULT_GLOBAL_COOLDOWN_SECONDS = 3;

export const MIN_SPEED = 0.5;
export const MAX_SPEED = 2.0;

export const MIN_VOLUME = 0.0;
export const MAX_VOLUME = 1.0;

export const MAX_BLOCKED_WORDS = 200;
export const MAX_BLOCKED_WORD_LENGTH = 100;
export const MAX_VOICE_ID_LENGTH = 200;
export const MAX_LANGUAGE_LENGTH = 35;
export const MAX_CURRENCY_LENGTH = 8;
export const MAX_ENABLED_SOURCE_IDS = 200;
export const MAX_SOURCE_ID_LENGTH = 128;

/** Mirrors internal/domain/audio's own maxAmountMicros exactly - the
 * same bound the backend enforces. */
export const MAX_AMOUNT_MICROS = 1_000_000_000_000;

/** Every engagement provider this application currently connects to -
 * mirrors internal/domain/engagement.ProviderTwitch/ProviderYouTube/
 * ProviderStreamElements. Used to filter which providers' events TTS
 * may speak. */
export const AUDIO_PROVIDER_IDS = ['twitch', 'youtube', 'streamelements'] as const;

/** The four provider modes the domain type system recognizes -
 * mirrors internal/domain/audio.KnownProviderModes. Only a subset is
 * actually accepted on save (see the capabilities endpoint's own
 * implementedProviderModes) - this app never hardcodes that subset,
 * it comes from the backend. */
export const AUDIO_PROVIDER_MODES = ['disabled', 'system', 'local', 'cloud'] as const;

/** The closed supporter-family event type set - mirrors
 * internal/audio/capability.go's own Capabilities table exactly
 * (docs/audio-tts.md §9). */
export const AUDIO_SUPPORTER_FAMILY_EVENT_TYPES = [
  'bits',
  'subscription',
  'resubscription',
  'gifted_subscription',
  'subscription_gift_batch',
  'youtube.membership',
  'youtube.membership_milestone',
  'youtube.super_chat',
  'youtube.super_sticker',
  'donation',
] as const;

/** Every event type Stage 17A TTS may ever speak - mirrors
 * internal/audio/capability.go's own Capabilities table keys exactly
 * (chat.message plus every supporter-family type). */
export const AUDIO_SPEAKABLE_EVENT_TYPES = [
  'chat.message',
  'follow',
  ...AUDIO_SUPPORTER_FAMILY_EVENT_TYPES,
  'raid',
  'channel_point_redemption',
] as const;

export function isValidQueueCapacity(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_QUEUE_CAPACITY && value <= MAX_QUEUE_CAPACITY;
}

export function isValidMaxTextLength(value: number): boolean {
  return (
    Number.isInteger(value) &&
    value >= MIN_MAX_TEXT_LENGTH_CODE_POINTS &&
    value <= MAX_MAX_TEXT_LENGTH_CODE_POINTS
  );
}

export function isValidPerUserCooldownSeconds(value: number): boolean {
  return (
    Number.isInteger(value) &&
    value >= MIN_PER_USER_COOLDOWN_SECONDS &&
    value <= MAX_PER_USER_COOLDOWN_SECONDS
  );
}

export function isValidGlobalCooldownSeconds(value: number): boolean {
  return (
    Number.isInteger(value) &&
    value >= MIN_GLOBAL_COOLDOWN_SECONDS &&
    value <= MAX_GLOBAL_COOLDOWN_SECONDS
  );
}

export function isValidSpeed(value: number): boolean {
  return Number.isFinite(value) && value >= MIN_SPEED && value <= MAX_SPEED;
}

export function isValidVolume(value: number): boolean {
  return Number.isFinite(value) && value >= MIN_VOLUME && value <= MAX_VOLUME;
}

export function isValidBlockedWords(words: readonly string[]): boolean {
  if (words.length > MAX_BLOCKED_WORDS) return false;
  return words.every((w) => w.trim() !== '' && codePointLength(w) <= MAX_BLOCKED_WORD_LENGTH);
}

export function isValidVoiceId(value: string): boolean {
  return codePointLength(value) <= MAX_VOICE_ID_LENGTH;
}

export function isValidLanguage(value: string): boolean {
  return codePointLength(value) <= MAX_LANGUAGE_LENGTH;
}

/** Mirrors internal/domain/audio.validateMoneyThreshold exactly: a
 * threshold is never meaningful without knowing which currency it
 * bounds, and this application never compares an amount across
 * currencies. */
export function isValidThresholdAmount(currency: string, minimumAmountMicros: number | null): boolean {
  if (minimumAmountMicros === null) return true;
  if (currency.trim() === '') return false;
  if (codePointLength(currency) > MAX_CURRENCY_LENGTH) return false;
  if (!Number.isInteger(minimumAmountMicros) || minimumAmountMicros < 0 || minimumAmountMicros > MAX_AMOUNT_MICROS) {
    return false;
  }
  return true;
}

export function isValidMinimumBits(value: number | null): boolean {
  if (value === null) return true;
  return Number.isInteger(value) && value >= 0;
}

/** Uppercases a currency code exactly like the backend's own
 * NormalizeCurrency (internal/domain/audio/validation.go). */
export function normalizeCurrencyCode(currency: string): string {
  return currency.toUpperCase();
}
