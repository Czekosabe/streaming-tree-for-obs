import { z } from 'zod';

/**
 * Contract of `GET /api/about` exposed by the Go backend.
 *
 * This is the single source of truth for the application's product/creator
 * identity on the frontend: nothing in this codebase keeps a second copy of
 * the repository/creator/support URLs as a literal string - see
 * `internal/buildinfo` on the backend and `docs/product-identity-legal.md`.
 *
 * Display prose is intentionally NOT part of this contract - `versionLabel`-
 * style wording ("Development build", licence status text) is derived from
 * `isReleaseBuild`/`applicationLicenceStatus` in the UI layer via i18n, never
 * sent as English text from the backend.
 */
export const aboutResponseSchema = z.object({
  productName: z.string().min(1),
  version: z.string().min(1),
  isReleaseBuild: z.boolean(),
  /** Short VCS revision, present only when Go's build-info stamping found one. */
  commit: z.string().min(1).optional(),
  commitDirty: z.boolean().optional(),
  creatorName: z.string().min(1),
  repositoryUrl: z.string().url(),
  creatorUrl: z.string().url(),
  supportUrl: z.string().url(),
  applicationLicenceStatus: z.enum(['unselected']),
});

export type AboutResponse = z.infer<typeof aboutResponseSchema>;
