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
