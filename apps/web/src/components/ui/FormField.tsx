import { useId, type ReactNode } from 'react';

import { cn } from '@/lib/cn';

type FormFieldProps = {
  label: string;
  hint?: string;
  error?: string | undefined;
  /** Right-aligned counter such as "42 / 140". */
  counter?: string;
  className?: string;
  /** Receives the ids that must be wired to the control for accessibility. */
  children: (ids: { inputId: string; describedBy: string | undefined }) => ReactNode;
};

/**
 * Label + control + hint/error wrapper.
 *
 * Errors are announced via `role="alert"` and linked through `aria-describedby`
 * so screen readers pick them up without the user having to hunt for them.
 */
export function FormField({
  label,
  hint,
  error,
  counter,
  className,
  children,
}: FormFieldProps) {
  const inputId = useId();
  const hintId = `${inputId}-hint`;
  const errorId = `${inputId}-error`;

  const describedByParts: string[] = [];
  if (hint !== undefined) describedByParts.push(hintId);
  if (error !== undefined) describedByParts.push(errorId);
  const describedBy = describedByParts.length > 0 ? describedByParts.join(' ') : undefined;

  return (
    <div className={cn('space-y-1.5', className)}>
      <div className="flex items-baseline justify-between gap-2">
        <label htmlFor={inputId} className="text-xs font-medium text-ink-muted">
          {label}
        </label>
        {counter !== undefined && (
          <span className="font-mono text-[11px] tabular-nums text-ink-faint">{counter}</span>
        )}
      </div>

      {children({ inputId, describedBy })}

      {hint !== undefined && error === undefined && (
        <p id={hintId} className="text-[11px] text-ink-faint">
          {hint}
        </p>
      )}
      {error !== undefined && (
        <p id={errorId} role="alert" className="text-[11px] font-medium text-status-error">
          {error}
        </p>
      )}
    </div>
  );
}
