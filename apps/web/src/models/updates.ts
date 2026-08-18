import { z } from 'zod';

/**
 * Contract of `GET /api/updates/status` (docs/updater.md §11/§30).
 *
 * Deliberately mirrors the backend's own safe-field exclusion list: no local
 * filesystem path, no GitHub asset id, no download URL, no SHA-256 value, no
 * machine identity ever appears here - if the backend ever adds one, this
 * schema not matching is the mechanism that would catch it (see
 * `internal/httpapi/updater_test.go`'s own regression guard on the Go side).
 */
export const updateStatusSchema = z.object({
  enabled: z.boolean(),
  releaseBuild: z.boolean(),
  currentVersion: z.string().min(1),

  autoCheck: z.boolean(),
  state: z.enum([
    'disabled',
    'idle',
    'checking',
    'up_to_date',
    'available',
    'downloading',
    'ready_to_install',
    'installing',
    'error',
    'platform_unsupported',
  ]),

  latestVersion: z.string().optional(),
  updateAvailable: z.boolean(),
  releaseNotes: z.string().optional(),
  releaseNotesTruncated: z.boolean().optional(),
  publishedAt: z.string().optional(),

  lastSuccessfulCheckAt: z.string().optional(),

  downloadedBytes: z.number().optional(),
  totalBytes: z.number().optional(),

  installBlocked: z.boolean(),
  blockerCode: z.string().optional(),

  lastErrorCode: z.string().optional(),

  postUpdateOutcome: z.string().optional(),
  postUpdateFromVersion: z.string().optional(),
  postUpdateToVersion: z.string().optional(),
});

export type UpdateStatus = z.infer<typeof updateStatusSchema>;

export type UpdateState = UpdateStatus['state'];
