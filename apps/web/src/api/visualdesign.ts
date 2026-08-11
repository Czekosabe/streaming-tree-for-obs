import { apiDelete, apiGet, apiPut } from '@/lib/api-client';

import { visualDesignResponseSchema, type VisualDesignDocument, type VisualDesignResponse } from './visualdesign-schemas';

/**
 * Transport for the shared visual-design API
 * (`internal/httpapi/visualdesign.go`, `internal/httpapi/chatoverlay.go`'s
 * own visual-design routes) - genuinely shared between the Alert
 * Designer and the Chat Overlay Designer (Stage 13B task Part 16), the
 * only difference being which management-API path segment owns the
 * design. No caching/React concerns live here - see
 * hooks/use-visual-design.ts.
 */

export type VisualDesignOwnerKind = 'alert-rules' | 'chat-overlays';

function ownerPath(ownerKind: VisualDesignOwnerKind, ownerId: string): string {
  return `/api/${ownerKind}/${ownerId}/visual-design`;
}

export async function fetchVisualDesign(
  ownerKind: VisualDesignOwnerKind,
  ownerId: string,
  signal?: AbortSignal,
): Promise<VisualDesignResponse> {
  return apiGet(ownerPath(ownerKind, ownerId), visualDesignResponseSchema, { signal });
}

/** Full-replacement save, requiring the caller's own last-known
 * revision (0 for "I believe nothing is saved yet") - a 409 ApiError
 * (`code === 'visual_design_revision_conflict'`) means another writer
 * saved first; the caller must reload, never silently retry with a
 * merged document (Stage 13A task Part 41). */
export async function saveVisualDesign(
  ownerKind: VisualDesignOwnerKind,
  ownerId: string,
  document: VisualDesignDocument,
  expectedRevision: number,
): Promise<VisualDesignResponse> {
  return apiPut(ownerPath(ownerKind, ownerId), { expectedRevision, document }, visualDesignResponseSchema);
}

/** "Reset to legacy presentation" - idempotent, never deletes the
 * owning rule/overlay itself. */
export async function deleteVisualDesign(ownerKind: VisualDesignOwnerKind, ownerId: string): Promise<void> {
  return apiDelete(ownerPath(ownerKind, ownerId));
}
