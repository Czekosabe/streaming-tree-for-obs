import { apiDelete, apiGet, apiPut } from '@/lib/api-client';

import { visualDesignResponseSchema, type VisualDesignDocument, type VisualDesignResponse } from './visualdesign-schemas';

/**
 * Transport for the Stage 13A visual-design API
 * (`internal/httpapi/visualdesign.go`). No caching/React concerns live
 * here - see hooks/use-visual-design.ts.
 */

export async function fetchVisualDesign(ruleId: string, signal?: AbortSignal): Promise<VisualDesignResponse> {
  return apiGet(`/api/alert-rules/${ruleId}/visual-design`, visualDesignResponseSchema, { signal });
}

/** Full-replacement save, requiring the caller's own last-known
 * revision (0 for "I believe nothing is saved yet") - a 409 ApiError
 * (`code === 'visual_design_revision_conflict'`) means another writer
 * saved first; the caller must reload, never silently retry with a
 * merged document (Stage 13A task Part 41). */
export async function saveVisualDesign(
  ruleId: string,
  document: VisualDesignDocument,
  expectedRevision: number,
): Promise<VisualDesignResponse> {
  return apiPut(`/api/alert-rules/${ruleId}/visual-design`, { expectedRevision, document }, visualDesignResponseSchema);
}

/** "Reset to legacy presentation" - idempotent, never deletes the rule
 * itself. */
export async function deleteVisualDesign(ruleId: string): Promise<void> {
  return apiDelete(`/api/alert-rules/${ruleId}/visual-design`);
}
