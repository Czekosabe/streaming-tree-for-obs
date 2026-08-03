import { LANGUAGE_LOCALES } from './config';
import type { SupportedLanguage } from './types';

/**
 * Keeps `<html lang>` in sync with the active language.
 *
 * Screen readers pick pronunciation from this attribute, so it must follow the
 * language switch and not just the initial render.
 */
export function applyDocumentLanguage(language: SupportedLanguage): void {
  if (typeof document === 'undefined') return;
  document.documentElement.lang = language;
}

/** BCP 47 locale used for `Intl` number and list formatting. */
export function localeForLanguage(language: SupportedLanguage): string {
  return LANGUAGE_LOCALES[language];
}
