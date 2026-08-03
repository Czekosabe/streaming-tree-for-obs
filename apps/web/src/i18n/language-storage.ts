import { DEFAULT_LANGUAGE, LANGUAGE_STORAGE_KEY } from './config';
import { isSupportedLanguage, type SupportedLanguage } from './types';

/**
 * Persistence of the language preference.
 *
 * This is the only piece of state the application keeps in localStorage. Stream
 * keys, OAuth tokens, platform configuration and stream metadata must never be
 * written here - see docs/project-overview.md, "Stream key security".
 *
 * Every access is defensive: localStorage throws in private browsing modes and
 * when storage is disabled by policy, and the stored value may have been edited
 * by hand or left behind by an older build.
 */

export function readStoredLanguage(): SupportedLanguage | null {
  let raw: string | null;

  try {
    raw = window.localStorage.getItem(LANGUAGE_STORAGE_KEY);
  } catch {
    // Storage unavailable (private mode, blocked by policy). Not an error:
    // the app simply falls back to the default language.
    return null;
  }

  // An unsupported or corrupted value is discarded rather than trusted.
  return isSupportedLanguage(raw) ? raw : null;
}

export function writeStoredLanguage(language: SupportedLanguage): void {
  try {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, language);
  } catch {
    // Losing the preference is acceptable; breaking the switch is not.
  }
}

/**
 * Language the app should start in: the stored preference when it is valid,
 * English otherwise. The browser's own language is deliberately ignored, so a
 * first launch is always English.
 */
export function resolveInitialLanguage(): SupportedLanguage {
  return readStoredLanguage() ?? DEFAULT_LANGUAGE;
}
