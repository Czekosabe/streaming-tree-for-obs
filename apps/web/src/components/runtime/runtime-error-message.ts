import type { TFunction } from 'i18next';

import type { RuntimeError } from '@/api/runtime-schemas';
import { runtimeErrorKey } from '@/models/runtime-presentation';

/**
 * Localizes a runtime error, falling back to the backend's English message.
 *
 * A code this build has no mapping for still produces a sentence, never a raw
 * identifier - the backend always sends a readable message alongside the code.
 */
export function runtimeErrorMessage(
  t: TFunction<'runtime'>,
  error: RuntimeError | null | undefined,
): string | null {
  if (error === null || error === undefined) return null;

  const key = runtimeErrorKey(error.code);
  if (key === null) {
    return error.message;
  }
  return t(key);
}
