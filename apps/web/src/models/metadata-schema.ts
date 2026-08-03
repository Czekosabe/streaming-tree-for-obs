import { z } from 'zod';

import type { PlatformDefinition, PlatformMetadata } from './platform';

/**
 * Capability-driven metadata validation.
 *
 * There is no single "stream metadata" schema, because platforms do not agree
 * on which fields exist or how long they may be. Instead a Zod schema is built
 * per platform from its capability table and field limits, so a platform that
 * does not support tags is never validated against tag rules.
 */

export type MetadataFieldName = keyof PlatformMetadata;

export type MetadataErrors = Partial<Record<MetadataFieldName, string>>;

export type MetadataValidationResult =
  | { success: true; errors: MetadataErrors }
  | { success: false; errors: MetadataErrors };

function optionValues(options: readonly { value: string }[]): string[] {
  return options.map((option) => option.value);
}

/** Builds the Zod schema describing the fields a given platform accepts. */
export function buildMetadataSchema(definition: PlatformDefinition): z.ZodType {
  const { capabilities, limits, options } = definition;
  const shape: Record<string, z.ZodType> = {};

  if (capabilities.title) {
    shape.title = z
      .string()
      .trim()
      .min(1, 'Title is required.')
      .max(limits.titleMaxLength, `Title must be at most ${limits.titleMaxLength} characters.`);
  }

  if (capabilities.description) {
    shape.description = z
      .string()
      .max(
        limits.descriptionMaxLength,
        `Description must be at most ${limits.descriptionMaxLength} characters.`,
      );
  }

  if (capabilities.category) {
    shape.category = z
      .string()
      .trim()
      .min(1, `${options.categoryLabel} is required.`)
      .max(100, `${options.categoryLabel} must be at most 100 characters.`);
  }

  if (capabilities.tags) {
    shape.tags = z
      .array(
        z
          .string()
          .trim()
          .min(2, 'A tag needs at least 2 characters.')
          .max(limits.tagMaxLength, `A tag may be at most ${limits.tagMaxLength} characters.`)
          .regex(/^[\p{L}\p{N} _-]+$/u, 'Tags may only contain letters, digits, spaces, - and _.'),
      )
      .max(limits.maxTags, `At most ${limits.maxTags} tags are allowed.`)
      .refine(
        (tags) => new Set(tags.map((tag) => tag.toLowerCase())).size === tags.length,
        'Tags must be unique.',
      );
  }

  if (capabilities.language) {
    const allowed = optionValues(options.languages);
    shape.language = z
      .string()
      .refine((value) => allowed.includes(value), 'Select a supported language.');
  }

  if (capabilities.visibility) {
    const allowed = optionValues(options.visibility);
    shape.visibility = z
      .string()
      .refine((value) => allowed.includes(value), 'Select a supported visibility option.');
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
      .refine((value) => allowed.includes(value), 'Select a supported latency mode.');
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
): MetadataValidationResult {
  const schema = buildMetadataSchema(definition);
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
