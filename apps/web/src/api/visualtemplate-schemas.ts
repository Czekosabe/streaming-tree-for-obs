import { z } from 'zod';

import { visualDesignDocumentSchema } from './visualdesign-schemas';

/**
 * Stage 14A's reusable visual-design template - see
 * docs/visual-templates.md for the full contract. Two distinct shapes
 * exist on purpose and are never conflated: `visualTemplateSchema` is
 * the management-API shape (local id/source/timestamps/compatibility),
 * `visualTemplateFileSchema` is the portable, asset-free JSON
 * interchange shape used for import/export - see that document's own
 * §3.
 */

export const visualTemplateTargetSchema = z.enum(['alert', 'chat']);
export type VisualTemplateTarget = z.infer<typeof visualTemplateTargetSchema>;

export const visualTemplateSourceSchema = z.enum(['builtin', 'user']);
export type VisualTemplateSource = z.infer<typeof visualTemplateSourceSchema>;

/** Stable compatibility blocker codes (docs/visual-templates.md §9) - an
 * opaque, translatable code, never parsed as prose. */
export const visualTemplateBlockerSchema = z.enum([
  'template_target_mismatch',
  'alert_binding_unavailable',
  'chat_binding_unavailable',
  'unsupported_visual_document',
  'visual_document_invalid',
]);
export type VisualTemplateBlocker = z.infer<typeof visualTemplateBlockerSchema>;

export const visualTemplateCompatibilitySchema = z.object({
  compatible: z.boolean(),
  blockers: z.array(visualTemplateBlockerSchema).optional(),
});
export type VisualTemplateCompatibility = z.infer<typeof visualTemplateCompatibilitySchema>;

export const VISUAL_TEMPLATE_FORMAT = 'streaming-tree-visual-template';
export const CURRENT_TEMPLATE_SCHEMA_VERSION = 1;

/** The management-API shape - `GET/POST/PUT /api/visual-templates(/{id})`. */
export const visualTemplateSchema = z.object({
  id: z.string(),
  target: visualTemplateTargetSchema,
  source: visualTemplateSourceSchema,
  name: z.string(),
  description: z.string(),
  author: z.string(),
  license: z.string(),
  templateSchemaVersion: z.number().int(),
  document: visualDesignDocumentSchema,
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
  compatibility: visualTemplateCompatibilitySchema.optional(),
});
export type VisualTemplate = z.infer<typeof visualTemplateSchema>;

/** The portable, asset-free JSON interchange shape - the request body
 * for import/import-preview, and the response body for export. */
export const visualTemplateFileSchema = z.object({
  format: z.string(),
  schemaVersion: z.number().int(),
  target: visualTemplateTargetSchema,
  name: z.string(),
  description: z.string(),
  author: z.string(),
  license: z.string(),
  visualDesign: visualDesignDocumentSchema,
});
export type VisualTemplateFile = z.infer<typeof visualTemplateFileSchema>;

/** Metadata bounds (docs/visual-templates.md §3/Stage 14A task Part 9) -
 * Unicode code points, mirrored from the backend's own bounds so the
 * frontend can give an immediate hint before a round trip. The backend
 * remains authoritative either way. */
export const VISUAL_TEMPLATE_NAME_MIN_LENGTH = 1;
export const VISUAL_TEMPLATE_NAME_MAX_LENGTH = 80;
export const VISUAL_TEMPLATE_DESCRIPTION_MAX_LENGTH = 400;
export const VISUAL_TEMPLATE_AUTHOR_MAX_LENGTH = 100;
export const VISUAL_TEMPLATE_LICENSE_MAX_LENGTH = 120;

/** A convenience client-side precheck for a selected import file's own
 * byte size (Stage 14A task Part 35) - mirrors the backend's own 128
 * KiB limit (visualtemplate.MaxImportBytes); the backend remains
 * authoritative and re-checks the real bytes regardless. */
export const VISUAL_TEMPLATE_IMPORT_MAX_BYTES = 128 * 1024;
