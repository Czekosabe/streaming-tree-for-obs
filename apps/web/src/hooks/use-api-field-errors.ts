import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { localizeFieldErrors } from '@/lib/field-error-rules';

/**
 * Turns the backend's per-field validation details into localized messages.
 *
 * The mapping rules live in `field-error-rules.ts`; this hook only supplies the
 * translation function. The `category` messages take the field's own label as a
 * parameter, which is why the callback fills it in.
 */
export function useApiFieldErrors(): (error: unknown) => Record<string, string> {
  const { t } = useTranslation(['metadata', 'platforms']);

  return useCallback(
    (error: unknown) =>
      localizeFieldErrors(error, (key, params) =>
        // `field` is only consumed by the category messages; the others ignore
        // the extra interpolation value.
        t(key, { ...params, field: t('metadata:fields.category') }),
      ),
    [t],
  );
}
