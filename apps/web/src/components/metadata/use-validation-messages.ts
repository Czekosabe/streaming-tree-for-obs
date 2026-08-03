import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import type { MetadataValidationMessages } from '@/models/metadata-schema';

/**
 * Bridges the `metadata` translation namespace to the schema's message object.
 *
 * Keeping the mapping in one hook means the schema module stays language-free
 * and every message key is checked once, here.
 */
export function useValidationMessages(): MetadataValidationMessages {
  const { t } = useTranslation('metadata');

  return useMemo<MetadataValidationMessages>(
    () => ({
      titleRequired: t('validation.titleRequired'),
      titleMaxLength: (max) => t('validation.titleMaxLength', { max }),
      descriptionMaxLength: (max) => t('validation.descriptionMaxLength', { max }),
      categoryRequired: (field) => t('validation.categoryRequired', { field }),
      categoryMaxLength: (field, max) => t('validation.categoryMaxLength', { field, max }),
      tagMinLength: (min) => t('validation.tagMinLength', { min }),
      tagMaxLength: (max) => t('validation.tagMaxLength', { max }),
      tagPattern: t('validation.tagPattern'),
      tagsMaxCount: (max) => t('validation.tagsMaxCount', { max }),
      tagsUnique: t('validation.tagsUnique'),
      languageUnsupported: t('validation.languageUnsupported'),
      visibilityUnsupported: t('validation.visibilityUnsupported'),
      latencyModeUnsupported: t('validation.latencyModeUnsupported'),
    }),
    [t],
  );
}
