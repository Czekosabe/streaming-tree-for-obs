import type { SelectHTMLAttributes } from 'react';

import { cn } from '@/lib/cn';
import type { SelectOption } from '@/models/platform';

import { controlClasses } from './control-classes';

type SelectInputProps = SelectHTMLAttributes<HTMLSelectElement> & {
  options: readonly SelectOption[];
};

export function SelectInput({ options, className, ...rest }: SelectInputProps) {
  return (
    <select className={cn(controlClasses, 'appearance-none pr-8', className)} {...rest}>
      {options.map((option) => (
        <option key={option.value} value={option.value} className="bg-surface text-ink">
          {option.label}
        </option>
      ))}
    </select>
  );
}
