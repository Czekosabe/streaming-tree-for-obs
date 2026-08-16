/**
 * Pure, backend-mirroring validation and mapping helpers for Stage 12A
 * alerts. The backend (internal/domain/alerts.Validate*,
 * internal/alerts's own capability table and placeholder engine) is
 * the real authority for every bound below - these exist only for live
 * client-side feedback before a submit.
 */

import type { AlertEventTypeCapability, AlertRule, AlertRuleAudio, AlertRuleInput } from '@/api/alerts-schemas';

/** Counts Unicode code points the same way the backend does - see
 * models/chat-automation.ts's own identical helper. */
export function codePointLength(text: string): number {
  return Array.from(text).length;
}

export const MAX_NAME_CODE_POINTS = 80;
export const MAX_TEMPLATE_CODE_POINTS = 500;

export const MIN_PRIORITY = 0;
export const MAX_PRIORITY = 100;
export const DEFAULT_PRIORITY = 50;

export const MIN_DURATION_MS = 1000;
export const MAX_DURATION_MS = 30000;
export const DEFAULT_DURATION_MS = 5000;

export const MIN_ANIMATION_DURATION_MS = 0;
export const MAX_ANIMATION_DURATION_MS = 2000;
export const DEFAULT_ANIMATION_DURATION_MS = 400;

export const DEFAULT_MAX_QUEUE_ITEMS = 100;
export const MIN_MAX_QUEUE_ITEMS = 1;
export const MAX_MAX_QUEUE_ITEMS = 500;

export const DEFAULT_MAXIMUM_QUEUE_AGE_SECONDS = 120;
export const MIN_MAXIMUM_QUEUE_AGE_SECONDS = 5;
export const MAX_MAXIMUM_QUEUE_AGE_SECONDS = 3600;

/** Stage 12B: a rule's grouping window bounds - mirrors
 * internal/domain/alerts.{Min,Max,Default}GroupWindowMS exactly. */
export const MIN_GROUP_WINDOW_MS = 1000;
export const MAX_GROUP_WINDOW_MS = 30000;
export const DEFAULT_GROUP_WINDOW_MS = 5000;

/** Stage 17B: a rule's own persistent-sound/TTS volume bounds - mirrors
 * internal/domain/alerts.{Min,Max,Default}RuleAudioVolume exactly. The
 * TTS template reuses MAX_TEMPLATE_CODE_POINTS above (the identical
 * bound the visual text template already uses). */
export const MIN_RULE_AUDIO_VOLUME = 0.0;
export const MAX_RULE_AUDIO_VOLUME = 1.0;
export const DEFAULT_RULE_AUDIO_VOLUME = 1.0;

/** The closed alert placeholder vocabulary - see
 * internal/alerts/templates.go's own KnownPlaceholders. Deliberately a
 * different set from chat automation's (models/chat-automation.ts) -
 * an alert has different data. */
export const KNOWN_ALERT_PLACEHOLDERS = [
  'username',
  'platform',
  'eventType',
  'quantity',
  'message',
  'rewardTitle',
  'groupCount',
  'amount',
  'currency',
  'membershipLevel',
] as const;
export type KnownAlertPlaceholder = (typeof KNOWN_ALERT_PLACEHOLDERS)[number];

export const ALERT_EVENT_TYPES = [
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
] as const;

export const ALERT_ROLES = ['everyone', 'subscriber', 'vip', 'moderator', 'broadcaster'] as const;

/** Stage 12B: mirrors internal/domain/alerts.InterruptMode's own closed
 * enum exactly. */
export const ALERT_INTERRUPT_MODES = ['never', 'lower_priority'] as const;

export const ALERT_ANIMATIONS = ['none', 'fade', 'slide_up', 'slide_left', 'scale'] as const;
export const ALERT_THEMES = ['minimal', 'compact', 'large'] as const;
export const ALERT_POSITIONS = ['top', 'center', 'bottom'] as const;
export const ALERT_TEXT_ALIGNS = ['left', 'center', 'right'] as const;
export const ALERT_LANGUAGES = ['en', 'pl'] as const;

export function isValidAlertName(name: string): boolean {
  const n = codePointLength(name.trim());
  return n >= 1 && n <= MAX_NAME_CODE_POINTS;
}

export function isValidAlertTemplate(template: string): boolean {
  const trimmed = template.trim();
  if (trimmed === '') return false;
  return codePointLength(template) <= MAX_TEMPLATE_CODE_POINTS;
}

export function isValidPriority(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_PRIORITY && value <= MAX_PRIORITY;
}

export function isValidDurationMs(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_DURATION_MS && value <= MAX_DURATION_MS;
}

export function isValidAnimationDurationMs(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_ANIMATION_DURATION_MS && value <= MAX_ANIMATION_DURATION_MS;
}

export function isValidMaxQueueItems(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_MAX_QUEUE_ITEMS && value <= MAX_MAX_QUEUE_ITEMS;
}

export function isValidMaximumQueueAgeSeconds(value: number): boolean {
  return (
    Number.isInteger(value) &&
    value >= MIN_MAXIMUM_QUEUE_AGE_SECONDS &&
    value <= MAX_MAXIMUM_QUEUE_AGE_SECONDS
  );
}

/** Stage 12B: bounds enforced unconditionally by the backend regardless
 * of whether grouping is currently enabled on the rule - see
 * internal/domain/alerts.ValidateRuleFields's own doc comment for why. */
export function isValidGroupWindowMs(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_GROUP_WINDOW_MS && value <= MAX_GROUP_WINDOW_MS;
}

/** Inclusive threshold bounds: both sides optional (null = unbounded),
 * and when both are set, minimum must not exceed maximum - mirrors
 * internal/domain/alerts.ValidateThresholds exactly. */
export function isValidThresholdRange(minimum: number | null, maximum: number | null): boolean {
  if (minimum !== null && (!Number.isInteger(minimum) || minimum < 0)) return false;
  if (maximum !== null && (!Number.isInteger(maximum) || maximum < 0)) return false;
  if (minimum !== null && maximum !== null && minimum > maximum) return false;
  return true;
}

/** Mirrors internal/domain/alerts.maxAmountMicros exactly - the same
 * bound the backend enforces, so a value this app will never actually
 * accept never appears "valid" client-side either. */
export const MAX_AMOUNT_MICROS = 1_000_000_000_000;

/** Inclusive amount-threshold bounds, plus the backend's own
 * currency-required-whenever-a-bound-is-set rule - mirrors
 * internal/domain/alerts.ValidateMoneyThresholds exactly (Stage 15A). */
export function isValidAmountRange(currency: string, minimum: number | null, maximum: number | null): boolean {
  if (minimum === null && maximum === null) return true;
  if (currency.trim() === '') return false;
  if (minimum !== null && (!Number.isInteger(minimum) || minimum < 0 || minimum > MAX_AMOUNT_MICROS)) return false;
  if (maximum !== null && (!Number.isInteger(maximum) || maximum < 0 || maximum > MAX_AMOUNT_MICROS)) return false;
  if (minimum !== null && maximum !== null && minimum > maximum) return false;
  return true;
}

/** Uppercases a currency code exactly like the backend's own
 * NormalizeCurrency (internal/domain/alerts/validation.go) - so a value
 * typed in lowercase compares/displays consistently before the round
 * trip to the server ever normalizes it. */
export function normalizeCurrencyCode(currency: string): string {
  return currency.toUpperCase();
}

/** Converts a user-typed decimal major-unit string (e.g. "5.00") into
 * integer micros, or null if the string is not a valid non-negative
 * decimal number - the exact inverse of formatAmountMicros below. Never
 * a float: the fractional part is parsed as a zero-padded 6-digit
 * string and summed as integers, so this can never accumulate the
 * rounding error a `Math.round(x * 1_000_000)` float computation could. */
export function parseAmountMicros(input: string): number | null {
  const trimmed = input.trim();
  if (trimmed === '') return null;
  const match = /^(\d+)(?:\.(\d{1,6}))?$/.exec(trimmed);
  if (match === null) return null;
  const major = match[1] ?? '0';
  const fraction = (match[2] ?? '').padEnd(6, '0');
  const micros = BigInt(major) * 1_000_000n + BigInt(fraction);
  if (micros > BigInt(Number.MAX_SAFE_INTEGER)) return null;
  return Number(micros);
}

/** Formats integer micros as a plain decimal major-unit string (no
 * currency symbol - mirrors internal/alerts.FormatAmountMicros exactly,
 * including its "trim trailing zeros, then pad back to at least 2
 * fraction digits" behavior). */
export function formatAmountMicros(amountMicros: number): string {
  const negative = amountMicros < 0;
  const abs = Math.abs(Math.trunc(amountMicros));
  const major = Math.floor(abs / 1_000_000);
  const fraction = abs % 1_000_000;
  let frac = String(fraction).padStart(6, '0').replace(/0+$/, '');
  if (frac.length < 2) frac = (frac + '00').slice(0, 2);
  return `${negative ? '-' : ''}${major}.${frac}`;
}

/** Parses a template's `{name}` placeholders (ignoring `{{`/`}}`
 * escapes) into the set of names referenced - a light client-side
 * mirror of the backend parser, used only to warn about an unknown/
 * unsupported name before save; the backend remains authoritative. */
export function extractPlaceholderNames(template: string): string[] {
  const names: string[] = [];
  const chars = Array.from(template);
  let i = 0;
  while (i < chars.length) {
    const c = chars[i];
    if (c === '{') {
      if (chars[i + 1] === '{') {
        i += 2;
        continue;
      }
      let j = i + 1;
      while (j < chars.length && chars[j] !== '}' && chars[j] !== '{') j++;
      if (j < chars.length && chars[j] === '}') {
        const name = chars.slice(i + 1, j).join('');
        if (name !== '') names.push(name);
        i = j + 1;
        continue;
      }
    }
    if (c === '}' && chars[i + 1] === '}') {
      i += 2;
      continue;
    }
    i++;
  }
  return names;
}

export function unknownPlaceholderNames(template: string): string[] {
  return extractPlaceholderNames(template).filter(
    (name) => !(KNOWN_ALERT_PLACEHOLDERS as readonly string[]).includes(name),
  );
}

/** Names referenced in template that are known overall but not
 * available for eventType - requires the real, backend-derived
 * capability list (fetched via useAlertEventTypesQuery), never a
 * hand-maintained duplicate. */
export function unsupportedPlaceholderNames(template: string, capability: AlertEventTypeCapability | undefined): string[] {
  if (capability === undefined) return [];
  const available = new Set(capability.availablePlaceholders);
  return extractPlaceholderNames(template).filter(
    (name) => (KNOWN_ALERT_PLACEHOLDERS as readonly string[]).includes(name) && !available.has(name),
  );
}

/** Inserts `{name}` at the given cursor position - the placeholder
 * insertion helper the rule editor's template field uses. */
export function insertPlaceholder(text: string, cursor: number, name: string): { text: string; cursor: number } {
  const token = `{${name}}`;
  const before = text.slice(0, cursor);
  const after = text.slice(cursor);
  return { text: before + token + after, cursor: cursor + token.length };
}

/** Stage 17B: a rule's own sound/TTS volume - the identical [0,1] bound
 * both halves share (internal/domain/alerts.ValidateRuleAudio). */
export function isValidRuleAudioVolume(value: number): boolean {
  return Number.isFinite(value) && value >= MIN_RULE_AUDIO_VOLUME && value <= MAX_RULE_AUDIO_VOLUME;
}

/** The safe "no rule-owned audio" zero value - mirrors
 * internal/domain/alerts.DefaultRuleAudio exactly. Shared by the rule
 * editor's own empty draft and the Alert Designer's own template-apply
 * flow (docs/alert-audio.md §10.6). */
export function defaultRuleAudio(): AlertRuleAudio {
  return {
    soundEnabled: false, soundAssetId: '', soundVolume: DEFAULT_RULE_AUDIO_VOLUME,
    ttsEnabled: false, ttsTemplate: '', ttsVolume: DEFAULT_RULE_AUDIO_VOLUME,
  };
}

/** Fills in the optional `soundAssetId`/`ttsTemplate` fields a response
 * may omit - the same `?? ''` normalization every draft-building call
 * site needs, kept in one place. */
export function normalizeRuleAudio(audio: AlertRuleAudio): AlertRuleAudio {
  return {
    soundEnabled: audio.soundEnabled, soundAssetId: audio.soundAssetId ?? '', soundVolume: audio.soundVolume,
    ttsEnabled: audio.ttsEnabled, ttsTemplate: audio.ttsTemplate ?? '', ttsVolume: audio.ttsVolume,
  };
}

/** Value equality for two rule-owned audio configurations - used by the
 * Alert Designer's own combined visual+audio dirty check (docs/alert-
 * audio.md §10.6), mirroring how `documentsEqual` compares the visual
 * document. */
export function ruleAudioEqual(a: AlertRuleAudio, b: AlertRuleAudio): boolean {
  return JSON.stringify(normalizeRuleAudio(a)) === JSON.stringify(normalizeRuleAudio(b));
}

/** The rule editor's own empty-draft starting point for a brand-new
 * rule - moved here (rather than staying a RuleManager.tsx-local
 * function) so the Alert Designer's own combined visual+audio save
 * (docs/alert-audio.md §10.6) can reuse draftFromRule below without
 * importing a component file purely for its plain data helpers. */
export function emptyRuleDraft(defaultEventType: AlertRuleInput['eventType']): AlertRuleInput {
  return {
    name: '', enabled: true, eventType: defaultEventType, priority: 50, durationMs: 5000,
    minimumQuantity: null, maximumQuantity: null, requiredRole: 'everyone',
    showPlatform: true, showUsername: true, showMessage: false, showQuantity: false,
    textTemplate: '', entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    providers: [], accounts: [],
    currency: '', minimumAmountMicros: null, maximumAmountMicros: null, showAmount: false,
    allowGrouping: false, groupWindowMs: DEFAULT_GROUP_WINDOW_MS,
    interruptMode: 'never', interruptible: true,
    audio: defaultRuleAudio(),
  };
}

/** Converts a persisted AlertRule into its own editable AlertRuleInput
 * shape - used both by the rule editor's own edit-mode draft and by
 * the Alert Designer's combined save, which needs every other field
 * carried through unchanged while only the audio half is replaced with
 * the current audio draft. */
export function draftFromRule(rule: AlertRule): AlertRuleInput {
  return {
    name: rule.name, enabled: rule.enabled, eventType: rule.eventType, priority: rule.priority,
    durationMs: rule.durationMs, minimumQuantity: rule.minimumQuantity ?? null, maximumQuantity: rule.maximumQuantity ?? null,
    requiredRole: rule.requiredRole, showPlatform: rule.showPlatform, showUsername: rule.showUsername,
    showMessage: rule.showMessage, showQuantity: rule.showQuantity, textTemplate: rule.textTemplate,
    entryAnimation: rule.entryAnimation, exitAnimation: rule.exitAnimation, animationDurationMs: rule.animationDurationMs,
    providers: rule.providers, accounts: rule.accounts,
    currency: rule.currency ?? '', minimumAmountMicros: rule.minimumAmountMicros ?? null,
    maximumAmountMicros: rule.maximumAmountMicros ?? null, showAmount: rule.showAmount,
    allowGrouping: rule.allowGrouping, groupWindowMs: rule.groupWindowMs,
    interruptMode: rule.interruptMode, interruptible: rule.interruptible,
    audio: normalizeRuleAudio(rule.audio),
  };
}

/** Stage 17B: a rule's own TTS template - the same length bound as the
 * visual text template, plus an unconditional rejection of the literal
 * `{groupCount}` placeholder (internal/httpapi/alerts.go's own
 * validateRuleTTSTemplate: grouping never restarts already-playing
 * audio, so a TTS template can never meaningfully reflect a group
 * count that may still change after the audio has already started). */
export function isValidTTSTemplate(template: string): boolean {
  const trimmed = template.trim();
  if (trimmed === '') return false;
  if (codePointLength(template) > MAX_TEMPLATE_CODE_POINTS) return false;
  return !template.includes('{groupCount}');
}
