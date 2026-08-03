import { z } from 'zod';

/**
 * Zod contracts for the platform configuration API.
 *
 * Every backend response is validated against these before it reaches a
 * component, so a contract mismatch surfaces as one readable error instead of
 * an undefined access somewhere in the render tree.
 *
 * Option values arrive as stable semantic identifiers ("public", "ultra-low").
 * They are kept as plain strings on purpose: an identifier the frontend does
 * not recognise must not fail parsing, because a newer backend adding an option
 * should degrade gracefully rather than blank the dashboard. Unknown values are
 * handled at render time - see `provider-labels.ts`.
 */

export const providerCapabilitiesSchema = z.object({
  title: z.boolean(),
  description: z.boolean(),
  category: z.boolean(),
  tags: z.boolean(),
  language: z.boolean(),
  visibility: z.boolean(),
  matureContent: z.boolean(),
  dvr: z.boolean(),
  latencyMode: z.boolean(),
});

export const providerLimitsSchema = z.object({
  titleMaxLength: z.number().int().nonnegative(),
  descriptionMaxLength: z.number().int().nonnegative(),
  maxTags: z.number().int().nonnegative(),
  tagMaxLength: z.number().int().nonnegative(),
});

export const providerDefinitionSchema = z.object({
  id: z.string().min(1),
  /** Brand name. A proper noun, never translated. */
  brandName: z.string().min(1),
  shortLabel: z.string().min(1),
  categoryFieldType: z.string().min(1),
  capabilities: providerCapabilitiesSchema,
  limits: providerLimitsSchema,
  visibilityOptions: z.array(z.string()),
  latencyOptions: z.array(z.string()),
  languageOptions: z.array(z.string()),
});

export const providerDefinitionsResponseSchema = z.object({
  definitions: z.array(providerDefinitionSchema),
});

export const platformMetadataSchema = z.object({
  title: z.string(),
  description: z.string(),
  category: z.string(),
  tags: z.array(z.string()),
  language: z.string(),
  visibility: z.string(),
  matureContent: z.boolean(),
  dvr: z.boolean(),
  latencyMode: z.string(),
  updatedAt: z.string(),
});

export const configuredPlatformSchema = z.object({
  id: z.string().min(1),
  providerId: z.string().min(1),
  displayName: z.string(),
  enabled: z.boolean(),
  sortOrder: z.number().int(),
  createdAt: z.string(),
  updatedAt: z.string(),
  /**
   * Optional so a platform referencing a provider this build does not know
   * still parses; the card then renders a clearly degraded state instead of
   * crashing.
   */
  provider: providerDefinitionSchema.optional(),
  metadata: platformMetadataSchema,
});

export const configuredPlatformsResponseSchema = z.object({
  platforms: z.array(configuredPlatformSchema),
});

export type ProviderCapabilities = z.infer<typeof providerCapabilitiesSchema>;
export type ProviderLimits = z.infer<typeof providerLimitsSchema>;
export type ProviderDefinition = z.infer<typeof providerDefinitionSchema>;
export type PlatformMetadata = z.infer<typeof platformMetadataSchema>;
export type ConfiguredPlatform = z.infer<typeof configuredPlatformSchema>;

/** Payload accepted by `POST /api/platforms`. Carries no credential field. */
export type CreatePlatformInput = {
  providerId: string;
  displayName: string;
  enabled: boolean;
};

/** Payload accepted by `PUT /api/platforms/{id}` - a full replacement. */
export type UpdatePlatformInput = {
  displayName: string;
  enabled: boolean;
  sortOrder: number;
};

/** Payload accepted by `PUT /api/platforms/{id}/metadata`. */
export type SaveMetadataInput = {
  title: string;
  description: string;
  category: string;
  tags: string[];
  language: string;
  visibility: string;
  matureContent: boolean;
  dvr: boolean;
  latencyMode: string;
};
