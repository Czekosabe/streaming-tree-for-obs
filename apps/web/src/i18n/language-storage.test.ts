import { afterEach, describe, expect, it, vi } from 'vitest';

import { LANGUAGE_STORAGE_KEY } from './config';
import { readStoredLanguage, resolveInitialLanguage, writeStoredLanguage } from './language-storage';

describe('language preference storage', () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  it('starts in English when nothing was stored yet', () => {
    expect(readStoredLanguage()).toBeNull();
    expect(resolveInitialLanguage()).toBe('en');
  });

  it('accepts a previously stored Polish preference', () => {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, 'pl');

    expect(readStoredLanguage()).toBe('pl');
    expect(resolveInitialLanguage()).toBe('pl');
  });

  it.each(['de', 'PL', '', 'null', '{"lang":"pl"}', 'en-US'])(
    'falls back to English for the unsupported stored value %j',
    (stored) => {
      window.localStorage.setItem(LANGUAGE_STORAGE_KEY, stored);

      expect(readStoredLanguage()).toBeNull();
      expect(resolveInitialLanguage()).toBe('en');
    },
  );

  it('writes the preference under the documented key', () => {
    writeStoredLanguage('pl');

    expect(window.localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('pl');
    expect(LANGUAGE_STORAGE_KEY).toBe('streaming-tree.language');
  });

  it('falls back to English when localStorage is unavailable', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled by policy');
    });

    expect(readStoredLanguage()).toBeNull();
    expect(resolveInitialLanguage()).toBe('en');
  });

  it('does not throw when the preference cannot be persisted', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded');
    });

    expect(() => writeStoredLanguage('pl')).not.toThrow();
  });

  it('stores nothing except the language preference', () => {
    writeStoredLanguage('pl');

    expect(Object.keys(window.localStorage)).toEqual([LANGUAGE_STORAGE_KEY]);
  });
});
