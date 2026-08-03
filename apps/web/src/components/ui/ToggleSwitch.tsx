import { useId } from 'react';

import { cn } from '@/lib/cn';

type ToggleSwitchProps = {
  label: string;
  description?: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  className?: string;
};

/**
 * Accessible switch built on a native checkbox: keyboard support, focus ring
 * and screen-reader semantics come for free, the visual is purely decorative.
 */
export function ToggleSwitch({
  label,
  description,
  checked,
  onCheckedChange,
  className,
}: ToggleSwitchProps) {
  const id = useId();
  const descriptionId = `${id}-description`;

  return (
    <div className={cn('flex items-start justify-between gap-3', className)}>
      <div className="min-w-0">
        <label htmlFor={id} className="cursor-pointer text-xs font-medium text-ink">
          {label}
        </label>
        {description !== undefined && (
          <p id={descriptionId} className="mt-0.5 text-[11px] text-ink-faint">
            {description}
          </p>
        )}
      </div>

      <span className="relative inline-flex shrink-0">
        <input
          id={id}
          type="checkbox"
          role="switch"
          checked={checked}
          aria-describedby={description !== undefined ? descriptionId : undefined}
          onChange={(event) => onCheckedChange(event.target.checked)}
          className="peer size-0 opacity-0"
        />
        <span
          aria-hidden="true"
          onClick={() => onCheckedChange(!checked)}
          className={cn(
            'block h-5 w-9 cursor-pointer rounded-full border transition-colors duration-200',
            'peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent-soft',
            checked ? 'border-accent bg-accent/70' : 'border-line bg-surface-sunken',
          )}
        >
          <span
            className={cn(
              'mt-0.5 block size-4 rounded-full bg-ink transition-transform duration-200',
              checked ? 'translate-x-4' : 'translate-x-0.5',
            )}
          />
        </span>
      </span>
    </div>
  );
}
