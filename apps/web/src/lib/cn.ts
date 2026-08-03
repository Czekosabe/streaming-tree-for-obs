import clsx, { type ClassValue } from 'clsx';

/** Conditional className helper used across the component library. */
export function cn(...inputs: ClassValue[]): string {
  return clsx(inputs);
}
