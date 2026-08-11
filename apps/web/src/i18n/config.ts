/**
 * Central localization constants.
 *
 * Every language code and namespace name used by the application is declared
 * here, so no component ever hard-codes a string such as 'en' or 'pl'.
 */

/**
 * Languages the interface ships with.
 *
 * English is first because it is the canonical source language: Polish (and any
 * future language) is a translation of it, never the other way round.
 */
export const SUPPORTED_LANGUAGES = ['en', 'pl'] as const;

/** Language used on first launch and whenever a stored preference is invalid. */
export const DEFAULT_LANGUAGE = 'en';

/** Language used for any key missing from the active language. */
export const FALLBACK_LANGUAGE = 'en';

/**
 * Endonyms: each language is written in itself, which is the convention for
 * language pickers and keeps the option readable regardless of the active UI
 * language. These labels are intentionally NOT translated.
 */
export const LANGUAGE_LABELS = {
  en: 'English',
  pl: 'Polski',
} as const;

/**
 * BCP 47 tags used for `Intl` formatting (numbers, dates) and for the
 * document's `lang` attribute.
 */
export const LANGUAGE_LOCALES = {
  en: 'en-US',
  pl: 'pl-PL',
} as const;

/**
 * Translation namespaces. Splitting resources by feature area keeps each file
 * reviewable and lets a screen load only what it needs.
 */
export const NAMESPACES = [
  'common',
  'navigation',
  'dashboard',
  'platforms',
  'metadata',
  'pages',
  'errors',
  'runtime',
  'accounts',
  'engagement',
  'chat',
  'overlays',
  'automation',
  'alerts',
  'alertDesigner',
  'chatOverlayDesigner',
] as const;

/** Namespace assumed when a component calls `t()` without naming one. */
export const DEFAULT_NAMESPACE = 'common';

/**
 * localStorage key holding the language preference.
 *
 * This is the ONLY value the application stores in the browser. Stream keys,
 * tokens, platform configuration and stream metadata must never be persisted
 * client-side.
 */
export const LANGUAGE_STORAGE_KEY = 'streaming-tree.language';
