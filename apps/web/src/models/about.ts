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
 * style wording ("Development build") is derived from `isReleaseBuild` in
 * the UI layer via i18n, never sent as English text from the backend. The
 * licence fields are the one exception: a licence's own name/SPDX
 * identifier is not the kind of string that varies by UI language, so the
 * UI uses them directly rather than localizing them.
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
  applicationLicenseSpdx: z.string().min(1),
  applicationLicenseName: z.string().min(1),
});

export type AboutResponse = z.infer<typeof aboutResponseSchema>;
