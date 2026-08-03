import { z } from 'zod';

import type { ProviderCapabilities, ProviderLimits, SaveMetadataInput } from '@/api/platform-schemas';

/**
 * Client-side metadata validation.
 *
 * This mirrors the backend rules to give immediate feedback and to avoid a
 * round trip for obvious mistakes. **The backend remains the authority**: every
 * save is validated again server-side and a rejection there is shown per field.
 *
 * The rules are driven entirely by the provider definition the backend sent, so
 * there is no second, independently authored capability table.
 *
 * Messages are injected rather than hard-coded, keeping this module free of
 * display language.
 */

export type MetadataFieldName = keyof SaveMetadataInput;

export type MetadataErrors = Partial<Record<MetadataFieldName, string>>;

export type MetadataValidationResult =
  | { success: true; errors: MetadataErrors }
  | { success: false; errors: MetadataErrors };

/** Already-translated validation messages. */
export type MetadataValidationMessages = {
  titleRequired: string;
  titleMaxLength: (max: number) => string;
  descriptionMaxLength: (max: number) => string;
  categoryRequired: (field: string) => string;
  categoryMaxLength: (field: string, max: number) => string;
  tagMinLength: (min: number) => string;
  tagMaxLength: (max: number) => string;
  tagPattern: string;
  tagsMaxCount: (max: number) => string;
  tagsUnique: string;
  languageUnsupported: string;
  visibilityUnsupported: string;
  latencyModeUnsupported: string;
};

/** The provider facts validation needs, taken straight from the API response. */
export type MetadataValidationContext = {
  capabilities: ProviderCapabilities;
  limits: ProviderLimits;
  /** Already-translated label of the category-like field. */
  categoryLabel: string;
  visibilityOptions: readonly string[];
  latencyOptions: readonly string[];
  languageOptions: readonly string[];
};

/** Shortest accepted tag; matches the backend's rule. */
const MIN_TAG_LENGTH = 2;

/** Longest category value; matches the backend's rule. */
const MAX_CATEGORY_LENGTH = 100;

const TAG_PATTERN = /^[\p{L}\p{N} _-]+$/u;

/** Builds the Zod schema describing the fields the provider accepts. */
export function buildMetadataSchema(
  context: MetadataValidationContext,
  messages: MetadataValidationMessages,
): z.ZodType {
  const { capabilities, limits, categoryLabel } = context;
  const shape: Record<string, z.ZodType> = {};

  if (capabilities.title) {
    shape.title = z
      .string()
      .trim()
      .min(1, messages.titleRequired)
      .max(limits.titleMaxLength, messages.titleMaxLength(limits.titleMaxLength));
  }

  if (capabilities.description) {
    shape.description = z
      .string()
      .max(limits.descriptionMaxLength, messages.descriptionMaxLength(limits.descriptionMaxLength));
  }

  if (capabilities.category) {
    shape.category = z
      .string()
      .trim()
      .min(1, messages.categoryRequired(categoryLabel))
      .max(MAX_CATEGORY_LENGTH, messages.categoryMaxLength(categoryLabel, MAX_CATEGORY_LENGTH));
  }

  if (capabilities.tags) {
    shape.tags = z
      .array(
        z
          .string()
          .trim()
          .min(MIN_TAG_LENGTH, messages.tagMinLength(MIN_TAG_LENGTH))
          .max(limits.tagMaxLength, messages.tagMaxLength(limits.tagMaxLength))
          .regex(TAG_PATTERN, messages.tagPattern),
      )
      .max(limits.maxTags, messages.tagsMaxCount(limits.maxTags))
      .refine(
        (tags) => new Set(tags.map((tag) => tag.toLowerCase())).size === tags.length,
        messages.tagsUnique,
      );
  }

  if (capabilities.language) {
    const allowed = context.languageOptions;
    shape.language = z
      .string()
      .refine((value) => value === '' || allowed.includes(value), messages.languageUnsupported);
  }

  if (capabilities.visibility) {
    const allowed = context.visibilityOptions;
    shape.visibility = z
      .string()
      .refine((value) => value === '' || allowed.includes(value), messages.visibilityUnsupported);
  }

  if (capabilities.matureContent) {
    shape.matureContent = z.boolean();
  }

  if (capabilities.dvr) {
    shape.dvr = z.boolean();
  }

  if (capabilities.latencyMode) {
    const allowed = context.latencyOptions;
    shape.latencyMode = z
      .string()
      .refine((value) => value === '' || allowed.includes(value), messages.latencyModeUnsupported);
  }

  return z.object(shape);
}

/** Narrows a metadata draft to the fields the provider supports. */
export function pickSupportedFields(
  capabilities: ProviderCapabilities,
  metadata: SaveMetadataInput,
): Record<string, unknown> {
  const picked: Record<string, unknown> = {};

  if (capabilities.title) picked.title = metadata.title;
  if (capabilities.description) picked.description = metadata.description;
  if (capabilities.category) picked.category = metadata.category;
  if (capabilities.tags) picked.tags = metadata.tags;
  if (capabilities.language) picked.language = metadata.language;
  if (capabilities.visibility) picked.visibility = metadata.visibility;
  if (capabilities.matureContent) picked.matureContent = metadata.matureContent;
  if (capabilities.dvr) picked.dvr = metadata.dvr;
  if (capabilities.latencyMode) picked.latencyMode = metadata.latencyMode;

  return picked;
}

/**
 * Validates a draft and flattens Zod issues into one message per top-level
 * field, which is what the form renders.
 */
export function validateMetadata(
  context: MetadataValidationContext,
  metadata: SaveMetadataInput,
  messages: MetadataValidationMessages,
): MetadataValidationResult {
  const schema = buildMetadataSchema(context, messages);
  const result = schema.safeParse(pickSupportedFields(context.capabilities, metadata));

  if (result.success) {
    return { success: true, errors: {} };
  }

  const errors: MetadataErrors = {};
  for (const issue of result.error.issues) {
    const field = issue.path[0];
    if (typeof field !== 'string') continue;
    const key = field as MetadataFieldName;
    // Keep the first message per field: forms show one error line per input.
    if (errors[key] === undefined) {
      errors[key] = issue.message;
    }
  }

  return { success: false, errors };
}
