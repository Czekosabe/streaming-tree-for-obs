import { describe, expect, it } from 'vitest';

import {
  categoryFieldLabelKey,
  categoryPlaceholderKey,
  languageLabel,
  latencyLabelKey,
  providerGlyphClass,
  visibilityLabelKey,
} from './provider-labels';

/**
 * These mappings are the boundary between backend identifiers and display
 * language. The important property is that every lookup is total: an
 * unrecognised identifier degrades, never throws.
 */

describe('identifier to translation key mapping', () => {
  it('maps known category field types', () => {
    expect(categoryFieldLabelKey('category')).toBe('fields.category');
    expect(categoryFieldLabelKey('topic')).toBe('fields.topic');
  });

  it('maps known visibility identifiers', () => {
    expect(visibilityLabelKey('public')).toBe('visibility.public');
    expect(visibilityLabelKey('unlisted')).toBe('visibility.unlisted');
    expect(visibilityLabelKey('private')).toBe('visibility.private');
  });

  it('maps known latency identifiers, including the hyphenated one', () => {
    expect(latencyLabelKey('normal')).toBe('latency.normal');
    expect(latencyLabelKey('low')).toBe('latency.low');
    expect(latencyLabelKey('ultra-low')).toBe('latency.ultraLow');
  });

  it('maps known provider category placeholders', () => {
    expect(categoryPlaceholderKey('twitch')).toBe('categoryPlaceholder.twitch');
    expect(categoryPlaceholderKey('tiktok')).toBe('categoryPlaceholder.tiktok');
  });
});

describe('unknown identifiers degrade safely', () => {
  it.each([
    ['category field', categoryFieldLabelKey],
    ['visibility', visibilityLabelKey],
    ['latency', latencyLabelKey],
    ['category placeholder', categoryPlaceholderKey],
  ])('returns null for an unrecognised %s identifier', (_name, lookup) => {
    expect(lookup('something-the-backend-added-later')).toBeNull();
  });

  it('never throws on hostile input', () => {
    for (const value of ['', '__proto__', 'constructor', 'toString']) {
      expect(() => visibilityLabelKey(value)).not.toThrow();
      expect(visibilityLabelKey(value)).toBeNull();
    }
  });
});

describe('language endonyms', () => {
  it('renders known languages in their own language', () => {
    expect(languageLabel('en')).toBe('English');
    expect(languageLabel('pl')).toBe('Polski');
    expect(languageLabel('de')).toBe('Deutsch');
  });

  it('falls back to the identifier for an unknown language', () => {
    // Shown verbatim rather than blanked, so the value stays recognisable.
    expect(languageLabel('kl')).toBe('kl');
  });
});

describe('provider glyph styling', () => {
  it('gives each known provider its own accent', () => {
    const twitch = providerGlyphClass('twitch');
    const youtube = providerGlyphClass('youtube');

    expect(twitch).not.toBe('');
    expect(twitch).not.toBe(youtube);
  });

  it('falls back to a neutral style for an unknown provider', () => {
    const unknown = providerGlyphClass('some-future-platform');

    expect(unknown).toContain('border-line');
    expect(unknown).not.toBe(providerGlyphClass('twitch'));
  });
});
