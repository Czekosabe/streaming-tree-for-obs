import type { ButtonHTMLAttributes, ReactNode } from 'react';

import { cn } from '@/lib/cn';

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'success';
type ButtonSize = 'sm' | 'md';

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Decorative icon rendered before the label. */
  icon?: ReactNode;
};

const VARIANT_CLASSES: Record<ButtonVariant, string> = {
  primary:
    'bg-accent text-white hover:bg-accent-soft active:bg-accent-deep border border-accent/60 shadow-panel',
  secondary:
    'bg-surface-raised text-ink hover:bg-surface-hover border border-line hover:border-line-strong',
  ghost: 'bg-transparent text-ink-muted hover:text-ink hover:bg-surface-hover border border-transparent',
  danger: 'bg-status-error/15 text-status-error hover:bg-status-error/25 border border-status-error/40',
  success: 'bg-status-live/15 text-status-live hover:bg-status-live/25 border border-status-live/40',
};

const SIZE_CLASSES: Record<ButtonSize, string> = {
  sm: 'h-8 px-2.5 text-xs gap-1.5',
  md: 'h-9 px-3.5 text-sm gap-2',
};

export function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  className,
  children,
  type = 'button',
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      className={cn(
        'inline-flex items-center justify-center rounded-lg font-medium whitespace-nowrap',
        'transition-colors duration-150',
        'disabled:cursor-not-allowed disabled:opacity-50',
        VARIANT_CLASSES[variant],
        SIZE_CLASSES[size],
        className,
      )}
      {...rest}
    >
      {icon !== undefined && (
        <span aria-hidden="true" className="shrink-0">
          {icon}
        </span>
      )}
      {children}
    </button>
  );
}

/** Square button used for icon-only actions. Requires an accessible label. */
export function IconButton({
  label,
  icon,
  className,
  variant = 'secondary',
  ...rest
}: Omit<ButtonProps, 'children' | 'size'> & { label: string; icon: ReactNode }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn(
        'inline-flex size-8 items-center justify-center rounded-lg transition-colors duration-150',
        'disabled:cursor-not-allowed disabled:opacity-50',
        VARIANT_CLASSES[variant],
        className,
      )}
      {...rest}
    >
      <span aria-hidden="true">{icon}</span>
    </button>
  );
}
