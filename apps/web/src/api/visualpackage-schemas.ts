import { z } from 'zod';

import { visualDesignDocumentSchema } from './visualdesign-schemas';
import { visualAssetKindSchema } from './visualasset-schemas';
import { visualTemplateTargetSchema } from './visualtemplate-schemas';

/**
 * Zod contracts for the Stage 14B portable package import/preview/
 * export API (`internal/httpapi/visualpackage.go`). See
 * docs/visual-template-packages.md §19/§43 for the full contract.
 */

export const visualTemplatePackagePreviewAssetSchema = z.object({
  packageAssetId: z.string(),
  kind: visualAssetKindSchema,
  mediaType: z.string(),
  sizeBytes: z.number(),
  displayName: z.string(),
  author: z.string(),
  license: z.string(),
  notice: z.string(),
  url: z.string(),
});
export type VisualTemplatePackagePreviewAsset = z.infer<typeof visualTemplatePackagePreviewAssetSchema>;

/** Stage 17B: describes a v2 package's own optional alertAudio preset
 * for display purposes only (docs/alert-audio.md §12: "package preview
 * identifies audio") - absent when the package carries none. Preview
 * never stages or plays the sound bytes themselves, so this carries no
 * asset id/volume, only what a human needs to see: whether sound/TTS
 * is configured, the sound's own display name/duration, and the TTS
 * template text. */
export const visualTemplatePackagePreviewAudioSchema = z.object({
  soundEnabled: z.boolean(),
  soundDisplayName: z.string().optional(),
  soundDurationMs: z.number().optional(),
  ttsEnabled: z.boolean(),
  ttsTemplate: z.string().optional(),
});
export type VisualTemplatePackagePreviewAudio = z.infer<typeof visualTemplatePackagePreviewAudioSchema>;

/** The document here still carries package-local (`pkgasset_...`)
 * asset references, resolved against `assets[].packageAssetId`/`url` -
 * never a real local managed-asset id (docs/visual-template-
 * packages.md §43). Nothing here has been persisted. */
export const visualTemplatePackagePreviewSchema = z.object({
  token: z.string(),
  target: visualTemplateTargetSchema,
  name: z.string(),
  description: z.string(),
  author: z.string(),
  license: z.string(),
  document: visualDesignDocumentSchema,
  assets: z.array(visualTemplatePackagePreviewAssetSchema),
  alertAudio: visualTemplatePackagePreviewAudioSchema.optional(),
  expiresAt: z.string(),
});
export type VisualTemplatePackagePreview = z.infer<typeof visualTemplatePackagePreviewSchema>;
