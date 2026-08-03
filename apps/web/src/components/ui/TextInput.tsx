import type { InputHTMLAttributes, TextareaHTMLAttributes } from 'react';

import { cn } from '@/lib/cn';

import { controlClasses } from './control-classes';

export function TextInput({
  className,
  ...rest
}: InputHTMLAttributes<HTMLInputElement>) {
  return <input type="text" className={cn(controlClasses, className)} {...rest} />;
}

export function TextArea({
  className,
  rows = 4,
  ...rest
}: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea rows={rows} className={cn(controlClasses, 'resize-y', className)} {...rest} />;
}
