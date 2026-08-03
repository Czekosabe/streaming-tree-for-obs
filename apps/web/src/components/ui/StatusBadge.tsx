import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/cn';
import { PLATFORM_STATUS_LABEL_KEYS, type PlatformStatus } from '@/models/platform';

/**
 * Status colours are semantic and consistent across the whole app:
 * green = live, blue = starting, red = error, grey = offline.
 * Colour is never the only signal - every badge also carries a text label.
 */
const STATUS_CLASSES: Record<PlatformStatus, string> = {
  live: 'border-status-live/40 bg-status-live/12 text-status-live',
  starting: 'border-status-starting/40 bg-status-starting/12 text-status-starting',
  error: 'border-status-error/40 bg-status-error/12 text-status-error',
  offline: 'border-status-offline/40 bg-status-offline/12 text-status-offline',
};

const DOT_CLASSES: Record<PlatformStatus, string> = {
  live: 'bg-status-live text-status-live',
  starting: 'bg-status-starting text-status-starting',
  error: 'bg-status-error text-status-error',
  offline: 'bg-status-offline text-status-offline',
};

export function StatusDot({ status, className }: { status: PlatformStatus; className?: string }) {
  return (
    <span className={cn('relative flex size-2 shrink-0', className)} aria-hidden="true">
      {status === 'live' && (
        <span
          className={cn('absolute inset-0 rounded-full animate-pulse-ring', DOT_CLASSES[status])}
        />
      )}
      <span className={cn('size-2 rounded-full', DOT_CLASSES[status])} />
    </span>
  );
}

export function StatusBadge({
  status,
  className,
  label,
}: {
  status: PlatformStatus;
  className?: string;
  /** Overrides the default status label. */
  label?: string;
}) {
  const { t } = useTranslation('platforms');

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5',
        'text-[11px] font-semibold uppercase tracking-wide',
        STATUS_CLASSES[status],
        className,
      )}
    >
      <StatusDot status={status} />
      {label ?? t(PLATFORM_STATUS_LABEL_KEYS[status])}
    </span>
  );
}
