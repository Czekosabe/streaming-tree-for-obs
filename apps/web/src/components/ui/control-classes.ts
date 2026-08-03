import { cn } from '@/lib/cn';

/**
 * Shared input chrome so every form control looks, hovers and focuses the same
 * way. Kept outside the component files so React Fast Refresh keeps working.
 */
export const controlClasses = cn(
  'w-full rounded-lg border border-line bg-surface-sunken px-3 py-2 text-sm text-ink',
  'placeholder:text-ink-faint',
  'transition-colors duration-150',
  'hover:border-line-strong',
  'focus:border-accent focus:outline-none',
  'aria-[invalid=true]:border-status-error/70',
);
