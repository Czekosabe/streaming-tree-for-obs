/**
 * Pure, backend-mirroring validation and mapping helpers for Stage 11B
 * chat automation. The backend
 * (internal/domain/chatautomation.Validate*, internal/chatautomation's
 * own placeholder engine) is the real authority for every bound below -
 * these exist only for live client-side feedback before a submit.
 */

/** Counts Unicode code points the same way the backend does - see
 * models/outbound-chat.ts's own identical helper for why `Array.from`
 * (not `.length`) is used. */
export function codePointLength(text: string): number {
  return Array.from(text).length;
}

export const MAX_NAME_CODE_POINTS = 80;
export const MAX_TEMPLATE_CODE_POINTS = 500;
export const MAX_MESSAGES_PER_SCHEDULE = 20;
export const MIN_INTERVAL_SECONDS = 60;
export const MAX_INTERVAL_SECONDS = 24 * 60 * 60;
export const MAX_FIRST_DELAY_SECONDS = 24 * 60 * 60;
export const MAX_JITTER_SECONDS = 15 * 60;
export const MAX_MINIMUM_CHAT_MESSAGES = 1000;
export const MIN_MAXIMUM_SENDS_PER_HOUR = 1;
export const MAX_MAXIMUM_SENDS_PER_HOUR = 60;
export const MIN_COMMAND_NAME_LENGTH = 1;
export const MAX_COMMAND_NAME_LENGTH = 32;
export const MAX_GLOBAL_COOLDOWN_SECONDS = 3600;
export const MAX_USER_COOLDOWN_SECONDS = 24 * 60 * 60;

/** The closed, fixed placeholder set the backend ever resolves - see
 * internal/chatautomation/placeholders.go's own KnownPlaceholders. */
export const KNOWN_PLACEHOLDERS = ['channelName', 'platform', 'channelUrl', 'streamTitle', 'streamUptime'] as const;
export type KnownPlaceholder = (typeof KNOWN_PLACEHOLDERS)[number];

export const COMMAND_ROLES = ['everyone', 'subscriber', 'vip', 'moderator', 'broadcaster'] as const;
export type CommandRoleValue = (typeof COMMAND_ROLES)[number];

export function isValidScheduleName(name: string): boolean {
  const n = codePointLength(name.trim());
  return n >= 1 && n <= MAX_NAME_CODE_POINTS;
}

export function isValidTemplate(template: string): boolean {
  const trimmed = template.trim();
  if (trimmed === '') return false;
  return codePointLength(template) <= MAX_TEMPLATE_CODE_POINTS;
}

export function isValidIntervalSeconds(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_INTERVAL_SECONDS && value <= MAX_INTERVAL_SECONDS;
}

export function isValidFirstDelaySeconds(value: number): boolean {
  return Number.isInteger(value) && value >= 0 && value <= MAX_FIRST_DELAY_SECONDS;
}

export function isValidJitterSeconds(value: number): boolean {
  return Number.isInteger(value) && value >= 0 && value <= MAX_JITTER_SECONDS;
}

export function isValidMinimumChatMessages(value: number): boolean {
  return Number.isInteger(value) && value >= 0 && value <= MAX_MINIMUM_CHAT_MESSAGES;
}

export function isValidMaximumSendsPerHour(value: number): boolean {
  return (
    Number.isInteger(value) && value >= MIN_MAXIMUM_SENDS_PER_HOUR && value <= MAX_MAXIMUM_SENDS_PER_HOUR
  );
}

/** ASCII lowercase letters, digits, `_` and `-` only, 1-32 characters -
 * mirrors internal/domain/chatautomation.ValidateCommandName exactly. */
export function isValidCommandName(name: string): boolean {
  if (name.length < MIN_COMMAND_NAME_LENGTH || name.length > MAX_COMMAND_NAME_LENGTH) return false;
  return /^[a-z0-9_-]+$/.test(name);
}

export function normalizeCommandName(name: string): string {
  return name.trim().toLowerCase();
}

export function isValidCooldownSeconds(globalSeconds: number, userSeconds: number): boolean {
  if (!Number.isInteger(globalSeconds) || globalSeconds < 0 || globalSeconds > MAX_GLOBAL_COOLDOWN_SECONDS) {
    return false;
  }
  if (!Number.isInteger(userSeconds) || userSeconds < 0 || userSeconds > MAX_USER_COOLDOWN_SECONDS) {
    return false;
  }
  return true;
}

/** Parses a template's `{name}` placeholders (ignoring `{{`/`}}`
 * escapes) into the set of names referenced - a light client-side
 * mirror of the backend parser, used only to warn about an unknown
 * name before save; the backend remains authoritative. */
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
    (name) => !(KNOWN_PLACEHOLDERS as readonly string[]).includes(name),
  );
}

/** Inserts `{name}` at the given cursor position - the placeholder
 * insertion helper both editors' template fields use. */
export function insertPlaceholder(text: string, cursor: number, name: string): { text: string; cursor: number } {
  const token = `{${name}}`;
  const before = text.slice(0, cursor);
  const after = text.slice(cursor);
  return { text: before + token + after, cursor: cursor + token.length };
}
