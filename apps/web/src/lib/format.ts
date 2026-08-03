/** Small formatting helpers shared by the dashboard widgets. */

const compactNumberFormatter = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 1,
});

/** 1284 -> "1.3K". Returns a dash for missing values. */
export function formatViewers(viewers: number | null): string {
  if (viewers === null) return '--';
  return compactNumberFormatter.format(viewers);
}

/** Seconds -> "1h 04m 12s". */
export function formatUptime(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;

  if (hours > 0) {
    return `${hours}h ${String(minutes).padStart(2, '0')}m ${String(rest).padStart(2, '0')}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${String(rest).padStart(2, '0')}s`;
  }
  return `${rest}s`;
}
