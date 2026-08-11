import { ApiError, kindForStatus, readErrorEnvelope, resolveUrl } from '@/lib/api-client';
import type { VisualTemplate } from '@/api/visualtemplate-schemas';
import { visualTemplateSchema } from '@/api/visualtemplate-schemas';

import { visualTemplatePackagePreviewSchema, type VisualTemplatePackagePreview } from './visualpackage-schemas';

/**
 * Transport for the Stage 14B portable package import/preview/export
 * API (`internal/httpapi/visualpackage.go`). Every request/response body
 * here is raw binary (`application/octet-stream`/`application/zip`),
 * never JSON, so this file talks to `fetch` directly rather than going
 * through `lib/api-client.ts`'s JSON-only `send` helper - it reuses that
 * module's error-envelope/status-classification logic instead of
 * duplicating it, exactly like api/visualasset.ts's own upload call.
 */

const PACKAGE_TIMEOUT_MS = 30_000;

async function sendPackageBody(path: string, body: ArrayBuffer): Promise<Response> {
  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), PACKAGE_TIMEOUT_MS);
  try {
    const response = await fetch(resolveUrl(path), {
      method: 'POST',
      body,
      signal: controller.signal,
      headers: { Accept: 'application/json', 'Content-Type': 'application/octet-stream' },
    });
    if (!response.ok) {
      const envelope = await readErrorEnvelope(response);
      throw new ApiError(kindForStatus(response.status, envelope.code), `Request to ${path} failed with ${response.status}.`, {
        status: response.status,
        ...envelope,
      });
    }
    return response;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError('timeout', `Request to ${path} timed out.`);
    }
    throw new ApiError('network', `Cannot reach the backend at ${resolveUrl(path)}.`);
  } finally {
    window.clearTimeout(timeoutId);
  }
}

/** Fully validates file as a `.streaming-tree-template` package and
 * stages its verified assets under a fresh preview token - persists
 * nothing (docs/visual-template-packages.md §43). */
export async function previewVisualTemplatePackageImport(file: File): Promise<VisualTemplatePackagePreview> {
  const bytes = await file.arrayBuffer();
  const response = await sendPackageBody('/api/visual-template-packages/import/preview', bytes);
  const payload: unknown = await response.json();
  const parsed = visualTemplatePackagePreviewSchema.safeParse(payload);
  if (!parsed.success) {
    throw new ApiError('parse', 'Backend response for the package preview did not match the expected shape.');
  }
  return parsed.data;
}

/** Best-effort - removes a preview session's staged bytes early. Safe
 * to call even after the session has already expired. */
export async function cancelVisualTemplatePackagePreview(token: string): Promise<void> {
  await fetch(resolveUrl(`/api/visual-template-packages/preview/${token}`), { method: 'DELETE' }).catch(() => undefined);
}

/** Re-validates file's own bytes from scratch - never trusts a prior
 * preview call (docs/visual-template-packages.md §19 step 6). A
 * successful import creates exactly one new user template plus its
 * managed assets; it never touches any owner design (§45). */
export async function importVisualTemplatePackage(file: File): Promise<VisualTemplate> {
  const bytes = await file.arrayBuffer();
  const response = await sendPackageBody('/api/visual-template-packages/import', bytes);
  const payload: unknown = await response.json();
  const parsed = visualTemplateSchema.safeParse(payload);
  if (!parsed.success) {
    throw new ApiError('parse', 'Backend response for the package import did not match the expected shape.');
  }
  return parsed.data;
}

/** Downloads id's own package export as a Blob - the caller triggers
 * the actual browser download (see models/visualtemplate.ts's own
 * download helper pattern). Rejected with `ApiError.code ===
 * 'visual_template_requires_package_export'` is never returned here -
 * that only applies to the asset-free JSON export endpoint; a package
 * export always succeeds for any template, asset-backed or not. */
export async function exportVisualTemplatePackage(id: string): Promise<{ blob: Blob; filename: string }> {
  const path = `/api/visual-templates/${id}/export-package`;
  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), PACKAGE_TIMEOUT_MS);
  let response: Response;
  try {
    response = await fetch(resolveUrl(path), { method: 'GET', signal: controller.signal });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError('timeout', `Request to ${path} timed out.`);
    }
    throw new ApiError('network', `Cannot reach the backend at ${resolveUrl(path)}.`);
  } finally {
    window.clearTimeout(timeoutId);
  }
  if (!response.ok) {
    const envelope = await readErrorEnvelope(response);
    throw new ApiError(kindForStatus(response.status, envelope.code), `Request to ${path} failed with ${response.status}.`, {
      status: response.status,
      ...envelope,
    });
  }
  const blob = await response.blob();
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const match = /filename="([^"]+)"/.exec(disposition);
  const filename = match?.[1] ?? 'template.streaming-tree-template';
  return { blob, filename };
}
