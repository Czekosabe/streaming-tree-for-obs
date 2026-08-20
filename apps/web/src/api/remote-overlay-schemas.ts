import { z } from 'zod';

/**
 * Zod contracts for the Stage 20D2C remote-overlay capability
 * management API (`internal/httpapi/remote_overlay_management.go`).
 * Registered on the backend only when a remote overlay origin is
 * configured - every request against these routes on any other
 * deployment returns 404, treated as "this feature does not exist
 * here" (see use-remote-overlay.ts), not an error.
 */

export const remoteOverlayDomainSchema = z.enum(['chat-overlay', 'alert-profile', 'audio', 'widget']);
export type RemoteOverlayDomain = z.infer<typeof remoteOverlayDomainSchema>;

export const remoteOverlayStatusSchema = z.object({
  version: z.number(),
  available: z.boolean(),
  enabled: z.boolean(),
  /** The current remote Browser Source URL - present only while enabled. */
  url: z.string().optional(),
});
export type RemoteOverlayStatus = z.infer<typeof remoteOverlayStatusSchema>;

export const remoteOverlayUrlResponseSchema = z.object({
  version: z.number(),
  url: z.string(),
});
export type RemoteOverlayUrlResponse = z.infer<typeof remoteOverlayUrlResponseSchema>;
