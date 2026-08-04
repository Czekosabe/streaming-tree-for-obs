import { z } from 'zod';

/**
 * Zod contract for the destination-credential API.
 *
 * This shape is the only thing this application ever knows about a stored
 * stream key: whether one is configured, and whether the credential store
 * could be reached. The value itself has no field here because the backend
 * never sends it - see docs/engagement-architecture.md and
 * docs/project-overview.md section 10.
 */
export const credentialStatusSchema = z.object({
  streamKey: z.object({
    configured: z.boolean(),
  }),
  store: z.object({
    available: z.boolean(),
  }),
});

export type CredentialStatus = z.infer<typeof credentialStatusSchema>;
