/**
 * Pure, backend-mirroring validation and mapping helpers for Stage 12A
 * alerts. The backend (internal/domain/alerts.Validate*,
 * internal/alerts's own capability table and placeholder engine) is
 * the real authority for every bound below - these exist only for live
 * client-side feedback before a submit.
 */

import type { AlertEventTypeCapability } from '@/api/alerts-schemas';

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
] as const;

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

/** Inclusive threshold bounds: both sides optional (null = unbounded),
 * and when both are set, minimum must not exceed maximum - mirrors
 * internal/domain/alerts.ValidateThresholds exactly. */
export function isValidThresholdRange(minimum: number | null, maximum: number | null): boolean {
  if (minimum !== null && (!Number.isInteger(minimum) || minimum < 0)) return false;
  if (maximum !== null && (!Number.isInteger(maximum) || maximum < 0)) return false;
  if (minimum !== null && maximum !== null && minimum > maximum) return false;
  return true;
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
