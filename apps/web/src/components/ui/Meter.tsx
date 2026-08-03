import { cn } from '@/lib/cn';

type MeterProps = {
  label: string;
  /** 0-100. */
  value: number;
  detail?: string;
  className?: string;
};

/** Threshold-based colouring keeps high utilisation readable at a glance. */
function barClass(value: number): string {
  if (value >= 85) return 'bg-status-error';
  if (value >= 65) return 'bg-status-warning';
  return 'bg-accent';
}

export function Meter({ label, value, detail, className }: MeterProps) {
  const clamped = Math.min(100, Math.max(0, Math.round(value)));

  return (
    <div className={cn('space-y-1.5', className)}>
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-xs font-medium text-ink-muted">{label}</span>
        <span className="font-mono text-xs tabular-nums text-ink">{clamped}%</span>
      </div>
      <div
        role="meter"
        aria-label={label}
        aria-valuenow={clamped}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuetext={`${clamped} percent`}
        className="h-1.5 w-full overflow-hidden rounded-full bg-surface-sunken ring-1 ring-line ring-inset"
      >
        <div
          className={cn('h-full rounded-full transition-[width] duration-500', barClass(clamped))}
          style={{ width: `${clamped}%` }}
        />
      </div>
      {detail !== undefined && <p className="text-[11px] text-ink-faint">{detail}</p>}
    </div>
  );
}
