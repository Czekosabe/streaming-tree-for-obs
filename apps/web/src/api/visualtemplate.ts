import { z } from 'zod';

import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  visualTemplateFileSchema,
  visualTemplateSchema,
  type VisualTemplate,
  type VisualTemplateFile,
  type VisualTemplateTarget,
} from './visualtemplate-schemas';

/**
 * Transport for the Stage 14A visual-template library
 * (`internal/httpapi/visualtemplate.go`) - a management/editor surface
 * only, never exposed publicly. No caching/React concerns live here -
 * see hooks/use-visual-templates.ts.
 */

const visualTemplateListSchema = z.array(visualTemplateSchema);

/** ownerContext, when supplied, asks the backend to additionally
 * compute each template's own `compatibility` block against a real
 * owner instance (docs/visual-templates.md §9) - purely informational,
 * never blocking the list itself. */
export async function fetchVisualTemplates(
  ownerContext?: { target: VisualTemplateTarget; ownerId: string },
  signal?: AbortSignal,
): Promise<VisualTemplate[]> {
  const query =
    ownerContext === undefined
      ? ''
      : `?target=${encodeURIComponent(ownerContext.target)}&ownerId=${encodeURIComponent(ownerContext.ownerId)}`;
  return apiGet(`/api/visual-templates${query}`, visualTemplateListSchema, { signal });
}

export async function fetchVisualTemplate(id: string, signal?: AbortSignal): Promise<VisualTemplate> {
  return apiGet(`/api/visual-templates/${id}`, visualTemplateSchema, { signal });
}

export async function createVisualTemplate(input: {
  target: VisualTemplateTarget;
  name: string;
  description: string;
  author: string;
  license: string;
  document: VisualTemplate['document'];
}): Promise<VisualTemplate> {
  return apiPost('/api/visual-templates', input, visualTemplateSchema);
}

export async function updateVisualTemplateMetadata(
  id: string,
  input: { name: string; description: string; author: string; license: string },
): Promise<VisualTemplate> {
  return apiPut(`/api/visual-templates/${id}`, input, visualTemplateSchema);
}

/** User templates only - a built-in returns a 409 ApiError
 * (`code === 'visual_template_immutable'`). */
export async function deleteVisualTemplate(id: string): Promise<void> {
  return apiDelete(`/api/visual-templates/${id}`);
}

/** Validates file, returning a normalized representation. Never
 * persists anything (Stage 14A task Part 19). ownerContext optionally
 * asks for a compatibility block, exactly like fetchVisualTemplates. */
export async function previewVisualTemplateImport(
  file: VisualTemplateFile,
  ownerContext?: { target: VisualTemplateTarget; ownerId: string },
): Promise<VisualTemplate> {
  const query =
    ownerContext === undefined
      ? ''
      : `?target=${encodeURIComponent(ownerContext.target)}&ownerId=${encodeURIComponent(ownerContext.ownerId)}`;
  return apiPost(`/api/visual-templates/import/preview${query}`, file, visualTemplateSchema);
}

/** Re-validates file (never trusts a prior preview call - see
 * previewVisualTemplateImport's own doc comment) and persists it as a
 * new user template with a freshly generated local id. */
export async function importVisualTemplate(file: VisualTemplateFile): Promise<VisualTemplate> {
  return apiPost('/api/visual-templates/import', file, visualTemplateSchema);
}

/** Fetches the canonical portable JSON representation of id (built-in
 * or user) - the caller is responsible for triggering an actual browser
 * download (see models/visualtemplate.ts's own
 * downloadVisualTemplateFile helper), since this function only performs
 * the validated fetch. */
export async function exportVisualTemplate(id: string): Promise<VisualTemplateFile> {
  return apiGet(`/api/visual-templates/${id}/export`, visualTemplateFileSchema);
}
