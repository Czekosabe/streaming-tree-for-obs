import { afterEach, describe, expect, it } from 'vitest';

import { LANGUAGE_STORAGE_KEY } from './config';
import { applyDocumentLanguage, localeForLanguage } from './document-language';
import { changeAppLanguage, createI18n } from './index';
import { isSupportedLanguage, toSupportedLanguage } from './types';

describe('language guards', () => {
  it('accepts only shipped languages', () => {
    expect(isSupportedLanguage('en')).toBe(true);
    expect(isSupportedLanguage('pl')).toBe(true);
    expect(isSupportedLanguage('de')).toBe(false);
    expect(isSupportedLanguage(42)).toBe(false);
    expect(isSupportedLanguage(null)).toBe(false);
  });

  it('narrows unknown values to the given fallback', () => {
    expect(toSupportedLanguage('pl', 'en')).toBe('pl');
    expect(toSupportedLanguage('klingon', 'en')).toBe('en');
  });
});

describe('translation lookup', () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  it('resolves English resources by default', () => {
    const i18n = createI18n();

    expect(i18n.language).toBe('en');
    expect(i18n.t('dashboard:backend.unavailable')).toBe('Backend unavailable');
  });

  it('resolves Polish resources after switching', async () => {
    const i18n = createI18n();
    await i18n.changeLanguage('pl');

    expect(i18n.t('dashboard:backend.unavailable')).toBe('Backend niedostępny');
    expect(i18n.t('platforms:card.enabled')).toBe('Włączona');
  });

  it('applies Polish plural categories', async () => {
    const i18n = createI18n();
    await i18n.changeLanguage('pl');

    // Polish needs one/few/many, unlike English's one/other.
    expect(i18n.t('dashboard:systemStatus.enabled', { count: 1 })).toBe('1 włączony cel');
    expect(i18n.t('dashboard:systemStatus.enabled', { count: 3 })).toBe('3 włączone cele');
    expect(i18n.t('dashboard:systemStatus.enabled', { count: 7 })).toBe('7 włączonych celów');
  });

  it('starts in the stored language', () => {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, 'pl');

    expect(createI18n().language).toBe('pl');
  });
});

describe('missing translation fallback', () => {
  /**
   * Parity is enforced by `npm run i18n:check`, so the shipped bundles have no
   * gaps. A Polish namespace is removed at runtime to simulate one and prove
   * the configured fallback: the user sees English, never a raw key.
   */
  it('falls back to English instead of showing the key', async () => {
    const i18n = createI18n();
    await i18n.changeLanguage('pl');

    // Sanity check: Polish resolves before the bundle is removed.
    expect(i18n.t('dashboard:backend.unavailable')).toBe('Backend niedostępny');

    i18n.removeResourceBundle('pl', 'dashboard');

    expect(i18n.t('dashboard:backend.unavailable')).toBe('Backend unavailable');
    expect(i18n.t('dashboard:backend.unavailable')).not.toBe('backend.unavailable');
    // Untouched namespaces still resolve to Polish.
    expect(i18n.t('platforms:card.enabled')).toBe('Włączona');
  });
});

describe('changeAppLanguage', () => {
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.lang = '';
  });

  it('updates the instance, the stored preference and the document language', async () => {
    const i18n = createI18n();
    expect(i18n.language).toBe('en');

    changeAppLanguage(i18n, 'pl');
    // i18next resolves resources synchronously here, but awaiting keeps the
    // assertion independent of that implementation detail.
    await i18n.changeLanguage('pl');

    expect(i18n.language).toBe('pl');
    expect(window.localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('pl');
    expect(document.documentElement.lang).toBe('pl');
  });

  it('ignores unsupported languages without touching any state', () => {
    const i18n = createI18n();
    applyDocumentLanguage('en');

    changeAppLanguage(i18n, 'de');

    expect(i18n.language).toBe('en');
    expect(window.localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBeNull();
    expect(document.documentElement.lang).toBe('en');
  });

  it('keeps the document language attribute in sync', () => {
    applyDocumentLanguage('pl');
    expect(document.documentElement.lang).toBe('pl');

    applyDocumentLanguage('en');
    expect(document.documentElement.lang).toBe('en');
  });

  it('maps languages to formatting locales', () => {
    expect(localeForLanguage('en')).toBe('en-US');
    expect(localeForLanguage('pl')).toBe('pl-PL');
  });
});
