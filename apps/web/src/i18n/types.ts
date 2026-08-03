import { SUPPORTED_LANGUAGES, type NAMESPACES } from './config';

/** A language code the application actually ships translations for. */
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];

/** A translation namespace name. */
export type AppNamespace = (typeof NAMESPACES)[number];

/**
 * Type guard used at every boundary where a language code arrives as plain
 * text: localStorage, query parameters, or the i18next instance itself.
 */
export function isSupportedLanguage(value: unknown): value is SupportedLanguage {
  return (
    typeof value === 'string' && (SUPPORTED_LANGUAGES as readonly string[]).includes(value)
  );
}

/**
 * Narrows an arbitrary value to a supported language, falling back to the
 * provided default. Never throws - callers use it on untrusted input.
 */
export function toSupportedLanguage(
  value: unknown,
  fallback: SupportedLanguage,
): SupportedLanguage {
  return isSupportedLanguage(value) ? value : fallback;
}
