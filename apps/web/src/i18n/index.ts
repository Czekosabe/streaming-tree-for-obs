import i18next, { type i18n as I18nInstance } from 'i18next';
import { initReactI18next } from 'react-i18next';

import { DEFAULT_NAMESPACE, FALLBACK_LANGUAGE, NAMESPACES, SUPPORTED_LANGUAGES } from './config';
import { applyDocumentLanguage } from './document-language';
import { resolveInitialLanguage, writeStoredLanguage } from './language-storage';
import { resources } from './resources';
import { isSupportedLanguage } from './types';

export { DEFAULT_LANGUAGE, LANGUAGE_LABELS, LANGUAGE_STORAGE_KEY, SUPPORTED_LANGUAGES } from './config';
export { applyDocumentLanguage, localeForLanguage } from './document-language';
export { readStoredLanguage, resolveInitialLanguage, writeStoredLanguage } from './language-storage';
export { isSupportedLanguage, toSupportedLanguage } from './types';
export type { AppNamespace, SupportedLanguage } from './types';

/**
 * Creates a configured i18next instance.
 *
 * Exported as a factory so tests can build isolated instances instead of
 * mutating the shared one.
 */
export function createI18n(): I18nInstance {
  const instance = i18next.createInstance();

  void instance.use(initReactI18next).init({
    resources,
    lng: resolveInitialLanguage(),
    fallbackLng: FALLBACK_LANGUAGE,
    supportedLngs: [...SUPPORTED_LANGUAGES],
    ns: [...NAMESPACES],
    defaultNS: DEFAULT_NAMESPACE,
    // Region variants ("pl-PL") resolve to the base language we ship.
    load: 'languageOnly',
    nonExplicitSupportedLngs: true,
    interpolation: {
      // React escapes interpolated values on render, so a second pass here
      // would double-escape apostrophes and ampersands.
      escapeValue: false,
    },
    returnNull: false,
    // Surfaces gaps loudly in development while production silently falls back
    // to English - a user must never be shown a raw translation key.
    saveMissing: false,
    missingKeyHandler: import.meta.env.DEV
      ? (languages, namespace, key) => {
          console.warn(
            `[i18n] missing key "${key}" in namespace "${namespace}" for ${languages.join(', ')}`,
          );
        }
      : false,
  });

  return instance;
}

/** Shared instance used by the application. */
export const i18n = createI18n();

/**
 * Applies a language across every place that has to know about it: i18next, the
 * persisted preference and the document element. Unsupported input is ignored
 * rather than allowed to break the UI.
 */
export function changeAppLanguage(instance: I18nInstance, language: string): void {
  if (!isSupportedLanguage(language)) return;

  void instance.changeLanguage(language);
  writeStoredLanguage(language);
  applyDocumentLanguage(language);
}

// The document attribute must already be correct on first paint.
applyDocumentLanguage(resolveInitialLanguage());
