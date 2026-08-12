import { z } from 'zod';

/**
 * Zod contracts for the Stage 16A external-donation-source management API
 * (`internal/httpapi/donationsources.go`).
 *
 * No field here ever carries a credential value - the backend response
 * shapes never carry one (CredentialConfigured is a boolean, never the
 * JWT itself), so there is nothing to strip.
 */

export const donationSourceProviderSchema = z.enum(['streamelements']);
export type DonationSourceProvider = z.infer<typeof donationSourceProviderSchema>;

export const donationSourceSchema = z.object({
  id: z.string().min(1),
  providerId: z.string().min(1),
  label: z.string(),
  enabled: z.boolean(),
  remoteChannelId: z.string(),
  credentialConfigured: z.boolean(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type DonationSource = z.infer<typeof donationSourceSchema>;

export const donationSourcesResponseSchema = z.object({
  items: z.array(donationSourceSchema),
});

export const donationConnectorStateSchema = z.enum([
  'disabled',
  'connecting',
  'connected',
  'reconnecting',
  'possible_gap',
  'reconnect_required',
  'error',
  'stopping',
]);
export type DonationConnectorState = z.infer<typeof donationConnectorStateSchema>;

export const donationConnectorSchema = z.object({
  sourceId: z.string().min(1),
  enabled: z.boolean(),
  state: donationConnectorStateSchema,
  connectedAt: z.string().optional(),
  lastEventAt: z.string().optional(),
  lastDataGapAt: z.string().optional(),
  reconnectCount: z.number(),
  possibleGapCount: z.number(),
  lastError: z.string().optional(),
});
export type DonationConnector = z.infer<typeof donationConnectorSchema>;

export const donationCredentialStatusSchema = z.object({
  configured: z.boolean(),
});

/** Payload accepted by `POST /api/donation-sources`. Token is the sensitive
 * credential (a StreamElements personal JWT, pasted verbatim) - never
 * retained anywhere in frontend state once the request completes. */
export type CreateDonationSourceInput = {
  providerId: DonationSourceProvider;
  label: string;
  remoteChannelId: string;
  token: string;
};

/** Payload accepted by `PUT /api/donation-sources/{id}` - safe metadata
 * only, never the credential (see ReplaceDonationSourceCredentialInput). */
export type UpdateDonationSourceInput = {
  label: string;
  remoteChannelId: string;
};

/** Payload accepted by `PUT /api/donation-sources/{id}/credential`. */
export type ReplaceDonationSourceCredentialInput = {
  token: string;
};

/** Payload accepted by `PUT /api/donation-sources/{id}/engagement`. */
export type SetDonationEngagementInput = {
  enabled: boolean;
};
