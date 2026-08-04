/**
 * Small formatting helpers shared by the dashboard widgets.
 *
 * Numbers are locale-aware: the caller passes the BCP 47 tag of the active
 * interface language, so 1284 renders as "1.3K" in English and "1,3 tys." in
 * Polish. Formatters are cached per locale because constructing an
 * `Intl.NumberFormat` is comparatively expensive.
 */

const compactNumberFormatters = new Map<string, Intl.NumberFormat>();

function compactNumberFormatter(locale: string): Intl.NumberFormat {
  const cached = compactNumberFormatters.get(locale);
  if (cached !== undefined) return cached;

  const formatter = new Intl.NumberFormat(locale, {
    notation: 'compact',
    maximumFractionDigits: 1,
  });
  compactNumberFormatters.set(locale, formatter);
  return formatter;
}

/** 1284 -> "1.3K". Returns a dash for missing values. */
export function formatViewers(viewers: number | null, locale: string): string {
  if (viewers === null) return '--';
  return compactNumberFormatter(locale).format(viewers);
}

const byteCountFormatters = new Map<string, Intl.NumberFormat>();

function byteCountFormatter(locale: string): Intl.NumberFormat {
  const cached = byteCountFormatters.get(locale);
  if (cached !== undefined) return cached;

  const formatter = new Intl.NumberFormat(locale, { maximumFractionDigits: 1 });
  byteCountFormatters.set(locale, formatter);
  return formatter;
}

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

/** 6_291_456 -> "6 MB" (locale-aware number, fixed unit label). */
export function formatBytes(bytes: number, locale: string): string {
  if (bytes <= 0) return `0 ${BYTE_UNITS[0]}`;

  const exponent = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    BYTE_UNITS.length - 1,
  );
  const value = bytes / 1024 ** exponent;
  return `${byteCountFormatter(locale).format(value)} ${BYTE_UNITS[exponent]}`;
}

/** 1.02 -> "1.02x" with a locale-aware number. */
export function formatSpeed(speed: number, locale: string): string {
  return `${byteCountFormatter(locale).format(speed)}x`;
}

/** Split seconds into the parts a duration label needs. */
export type DurationParts = {
  hours: number;
  minutes: string;
  seconds: string;
  /** Which translation entry the caller should use. */
  unit: 'hoursMinutesSeconds' | 'minutesSeconds' | 'seconds';
};

/**
 * Breaks a duration into parts instead of formatting it directly: the unit
 * names differ per language, so the final string is assembled from a single
 * complete translation entry rather than concatenated fragments.
 */
export function toDurationParts(totalSeconds: number): DurationParts {
  const seconds = Math.max(0, Math.floor(totalSeconds));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;

  const paddedMinutes = String(minutes).padStart(2, '0');
  const paddedSeconds = String(rest).padStart(2, '0');

  if (hours > 0) {
    return {
      hours,
      minutes: paddedMinutes,
      seconds: paddedSeconds,
      unit: 'hoursMinutesSeconds',
    };
  }
  if (minutes > 0) {
    return { hours: 0, minutes: String(minutes), seconds: paddedSeconds, unit: 'minutesSeconds' };
  }
  return { hours: 0, minutes: '0', seconds: String(rest), unit: 'seconds' };
}
