import { cn } from '@/lib/cn';

/**
 * Platform marker.
 *
 * Deliberately a coloured text badge rather than a brand logo: the project does
 * not ship third-party trademarks. The short label comes from the backend's
 * provider definition; the colour is passed in by the caller and carries no
 * status meaning.
 */
export function PlatformGlyph({ label, className }: { label: string; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'flex size-9 shrink-0 items-center justify-center rounded-lg border',
        'text-xs font-bold tracking-wide',
        className,
      )}
    >
      {label}
    </span>
  );
}
