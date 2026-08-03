import { z } from 'zod';

import type { PlatformDefinition, PlatformMetadata } from './platform';

/**
 * Capability-driven metadata validation.
 *
 * There is no single "stream metadata" schema, because platforms do not agree
 * on which fields exist or how long they may be. Instead a Zod schema is built
 * per platform from its capability table and field limits, so a platform that
 * does not support tags is never validated against tag rules.
 *
 * Messages are injected rather than hard-coded: this module must stay free of
 * display language so the same schema can produce English or Polish errors.
 */

export type MetadataFieldName = keyof PlatformMetadata;

export type MetadataErrors = Partial<Record<MetadataFieldName, string>>;

export type MetadataValidationResult =
  | { success: true; errors: MetadataErrors }
  | { success: false; errors: MetadataErrors };

/**
 * Already-translated validation messages.
 *
 * Declared as an explicit object (rather than a generic translate callback) so
 * a missing or misspelled message is a compile error, and so this module never
 * has to know about i18next.
 */
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

export type MetadataValidationContext = {
  messages: MetadataValidationMessages;
  /** Translated label of the category-like field ("Category", "Temat", ...). */
  categoryLabel: string;
};

/** Shortest tag accepted by the editor. */
const MIN_TAG_LENGTH = 2;

/** Longest value accepted in the category-like field. */
const MAX_CATEGORY_LENGTH = 100;

function optionValues(options: readonly { value: string }[]): string[] {
  return options.map((option) => option.value);
}

/** Builds the Zod schema describing the fields a given platform accepts. */
export function buildMetadataSchema(
  definition: PlatformDefinition,
  context: MetadataValidationContext,
): z.ZodType {
  const { capabilities, limits, options } = definition;
  const { messages, categoryLabel } = context;
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
      .max(
        limits.descriptionMaxLength,
        messages.descriptionMaxLength(limits.descriptionMaxLength),
      );
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
          .regex(/^[\p{L}\p{N} _-]+$/u, messages.tagPattern),
      )
      .max(limits.maxTags, messages.tagsMaxCount(limits.maxTags))
      .refine(
        (tags) => new Set(tags.map((tag) => tag.toLowerCase())).size === tags.length,
        messages.tagsUnique,
      );
  }

  if (capabilities.language) {
    const allowed = optionValues(options.languages);
    shape.language = z
      .string()
      .refine((value) => allowed.includes(value), messages.languageUnsupported);
  }

  if (capabilities.visibility) {
    const allowed = optionValues(options.visibility);
    shape.visibility = z
      .string()
      .refine((value) => allowed.includes(value), messages.visibilityUnsupported);
  }

  if (capabilities.matureContent) {
    shape.matureContent = z.boolean();
  }

  if (capabilities.dvr) {
    shape.dvr = z.boolean();
  }

  if (capabilities.latencyMode) {
    const allowed = optionValues(options.latencyModes);
    shape.latencyMode = z
      .string()
      .refine((value) => allowed.includes(value), messages.latencyModeUnsupported);
  }

  return z.object(shape);
}

/** Narrows a full metadata document to the fields the platform supports. */
export function pickSupportedFields(
  definition: PlatformDefinition,
  metadata: PlatformMetadata,
): Record<string, unknown> {
  const { capabilities } = definition;
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
 * Validates a metadata document against its platform schema and flattens Zod
 * issues into one message per top-level field, which is what the form renders.
 */
export function validateMetadata(
  definition: PlatformDefinition,
  metadata: PlatformMetadata,
  context: MetadataValidationContext,
): MetadataValidationResult {
  const schema = buildMetadataSchema(definition, context);
  const result = schema.safeParse(pickSupportedFields(definition, metadata));

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
