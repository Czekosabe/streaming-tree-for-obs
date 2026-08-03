import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { DEFAULT_LANGUAGE, LANGUAGE_LABELS, SUPPORTED_LANGUAGES } from './config';
import { localeForLanguage } from './document-language';
import { changeAppLanguage } from './index';
import { toSupportedLanguage, type SupportedLanguage } from './types';

export type LanguageOption = {
  value: SupportedLanguage;
  /** Endonym, e.g. "Polski". Intentionally not translated. */
  label: string;
};

const LANGUAGE_OPTIONS: readonly LanguageOption[] = SUPPORTED_LANGUAGES.map((value) => ({
  value,
  label: LANGUAGE_LABELS[value],
}));

export type UseLanguageResult = {
  language: SupportedLanguage;
  /** BCP 47 tag for `Intl` formatting. */
  locale: string;
  options: readonly LanguageOption[];
  setLanguage: (language: SupportedLanguage) => void;
};

/**
 * Single entry point for reading and changing the interface language.
 *
 * Components use this instead of touching i18next, localStorage or the document
 * element directly, so the three always stay in sync.
 */
export function useLanguage(): UseLanguageResult {
  const { i18n } = useTranslation();

  // `resolvedLanguage` accounts for fallback; `language` may still hold a
  // region variant such as "pl-PL".
  const language = toSupportedLanguage(i18n.resolvedLanguage ?? i18n.language, DEFAULT_LANGUAGE);

  const setLanguage = useCallback(
    (next: SupportedLanguage) => {
      changeAppLanguage(i18n, next);
    },
    [i18n],
  );

  return {
    language,
    locale: localeForLanguage(language),
    options: LANGUAGE_OPTIONS,
    setLanguage,
  };
}
