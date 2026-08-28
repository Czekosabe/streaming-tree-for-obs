import type { ParseKeys } from 'i18next';

/**
 * Maps the stable identifiers the backend sends onto local translation keys.
 *
 * The backend never returns display text, so this is where identifiers become
 * language. Every lookup is total: an identifier this build does not recognise
 * returns `null` and the caller renders the raw identifier instead. A newer
 * backend adding an option must degrade to "shows an unfamiliar word", never to
 * a crash or a blank dashboard.
 */

export type PlatformsKey = ParseKeys<'platforms'>;

/** Category-like field: "Category" on most providers, "Topic" on TikTok. */
const CATEGORY_FIELD_KEYS: Record<string, PlatformsKey> = {
  category: 'fields.category',
  topic: 'fields.topic',
};

const VISIBILITY_KEYS: Record<string, PlatformsKey> = {
  public: 'visibility.public',
  unlisted: 'visibility.unlisted',
  private: 'visibility.private',
};

const LATENCY_KEYS: Record<string, PlatformsKey> = {
  normal: 'latency.normal',
  low: 'latency.low',
  'ultra-low': 'latency.ultraLow',
};

/**
 * Stream languages are shown as endonyms, exactly as the platforms present
 * them, so they are proper nouns rather than translation resources.
 */
const LANGUAGE_ENDONYMS: Record<string, string> = {
  en: 'English',
  pl: 'Polski',
  de: 'Deutsch',
  es: 'Espanol',
  fr: 'Francais',
};

function lookup(map: Record<string, PlatformsKey>, identifier: string): PlatformsKey | null {
  return Object.prototype.hasOwnProperty.call(map, identifier)
    ? (map[identifier] ?? null)
    : null;
}

export function categoryFieldLabelKey(identifier: string): PlatformsKey | null {
  return lookup(CATEGORY_FIELD_KEYS, identifier);
}

export function visibilityLabelKey(identifier: string): PlatformsKey | null {
  return lookup(VISIBILITY_KEYS, identifier);
}

export function latencyLabelKey(identifier: string): PlatformsKey | null {
  return lookup(LATENCY_KEYS, identifier);
}

/** Endonym for a language identifier, or the identifier itself when unknown. */
export function languageLabel(identifier: string): string {
  return LANGUAGE_ENDONYMS[identifier] ?? identifier;
}

/**
 * Category placeholders are per-provider hints. Unknown providers get no
 * placeholder rather than a misleading one from another platform.
 */
const CATEGORY_PLACEHOLDER_KEYS: Record<string, PlatformsKey> = {
  twitch: 'categoryPlaceholder.twitch',
  youtube: 'categoryPlaceholder.youtube',
  kick: 'categoryPlaceholder.kick',
  tiktok: 'categoryPlaceholder.tiktok',
};

export function categoryPlaceholderKey(providerId: string): PlatformsKey | null {
  return lookup(CATEGORY_PLACEHOLDER_KEYS, providerId);
}

/**
 * Accent classes per provider. Purely decorative and carrying no status
 * meaning; an unknown provider falls back to a neutral style.
 */
const PROVIDER_GLYPH_CLASSES: Record<string, string> = {
  twitch: 'border-violet-500/35 bg-violet-500/12 text-violet-300',
  youtube: 'border-red-500/35 bg-red-500/12 text-red-300',
  kick: 'border-emerald-500/35 bg-emerald-500/12 text-emerald-300',
  // TikTok's two brand accent colours as a soft tile background - the
  // official mark itself is never recoloured, only this backing tile.
  // See docs/provider-branding.md §3.
  tiktok: 'border-sky-400/35 bg-gradient-to-br from-[#25F4EE]/20 to-[#FE2C55]/20 text-sky-200',
  // A donation-service provider (Stage 16A), not a streaming destination -
  // a distinct, app-owned accent, never StreamElements' own logo (see
  // docs/provider-integrations/external-donations.md §22/§44).
  streamelements: 'border-amber-500/35 bg-amber-500/12 text-amber-300',
};

const NEUTRAL_GLYPH_CLASS = 'border-line bg-surface-raised text-ink-muted';

export function providerGlyphClass(providerId: string): string {
  return PROVIDER_GLYPH_CLASSES[providerId] ?? NEUTRAL_GLYPH_CLASS;
}

/**
 * Wider, atmospheric per-provider gradient for a destination card's
 * decorative header band (`PlatformCard`'s hero area) - a deliberately
 * bigger, softer wash than `providerGlyphClass`'s small-tile accent, never a
 * fake stream preview. Purely decorative, no status meaning.
 */
const PROVIDER_HERO_CLASSES: Record<string, string> = {
  twitch: 'bg-gradient-to-br from-violet-600/25 via-violet-900/10 to-transparent',
  youtube: 'bg-gradient-to-br from-red-600/25 via-red-900/10 to-transparent',
  kick: 'bg-gradient-to-br from-emerald-500/25 via-emerald-900/10 to-transparent',
  tiktok: 'bg-gradient-to-br from-[#25F4EE]/20 via-[#FE2C55]/15 to-transparent',
  streamelements: 'bg-gradient-to-br from-amber-500/25 via-amber-900/10 to-transparent',
};

const NEUTRAL_HERO_CLASS = 'bg-gradient-to-br from-surface-hover to-transparent';

export function providerHeroClass(providerId: string): string {
  return PROVIDER_HERO_CLASSES[providerId] ?? NEUTRAL_HERO_CLASS;
}
