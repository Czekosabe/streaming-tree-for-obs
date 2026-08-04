import { z } from 'zod';

/**
 * Zod contracts for the Twitch integration-config, device-flow,
 * connected-account, account-link, category-search and metadata-publish
 * APIs.
 *
 * None of these schemas has a field for an access token, a refresh token,
 * a device code, or a client secret - the backend response shapes
 * themselves never carry one (see internal/httpapi/accounts.go), so there
 * is nothing to strip here; the absence is structural, not a filtering
 * step this file performs.
 */

export const integrationConfigSchema = z.object({
  configured: z.boolean(),
  source: z.enum(['environment', 'database', 'missing']),
  clientId: z.string().optional(),
});
export type IntegrationConfig = z.infer<typeof integrationConfigSchema>;

/** Payload accepted by `PUT /api/integrations/twitch/config`. */
export type SetIntegrationConfigInput = { clientId: string };

export const deviceFlowStateSchema = z.enum([
  'requesting_code',
  'waiting_for_user',
  'polling',
  'authorized',
  'denied',
  'expired',
  'cancelled',
  'error',
]);
export type DeviceFlowState = z.infer<typeof deviceFlowStateSchema>;

export const deviceFlowSnapshotSchema = z.object({
  attemptId: z.string().min(1),
  providerId: z.string().min(1),
  state: deviceFlowStateSchema,
  userCode: z.string().optional(),
  verificationUri: z.string().optional(),
  createdAt: z.string(),
  expiresAt: z.string().optional(),
  intervalSeconds: z.number().optional(),
  connectedAccountId: z.string().optional(),
  errorCode: z.string().optional(),
  errorMessage: z.string().optional(),
});
export type DeviceFlowSnapshot = z.infer<typeof deviceFlowSnapshotSchema>;

export const accountStatusSchema = z.enum(['connected', 'reconnect_required']);
export type AccountStatus = z.infer<typeof accountStatusSchema>;

export const connectedAccountSchema = z.object({
  id: z.string().min(1),
  providerId: z.string().min(1),
  login: z.string(),
  displayName: z.string(),
  avatarUrl: z.string().optional(),
  status: accountStatusSchema,
  scopes: z.array(z.string()),
  lastValidatedAt: z.string().optional(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type ConnectedAccount = z.infer<typeof connectedAccountSchema>;

export const connectedAccountsResponseSchema = z.object({
  accounts: z.array(connectedAccountSchema),
});

export const platformAccountLinkSchema = z.object({
  platformId: z.string().min(1),
  accountId: z.string().min(1),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type PlatformAccountLink = z.infer<typeof platformAccountLinkSchema>;

/** `GET /api/platforms/{id}/connected-account` answers `null` when unlinked. */
export const platformAccountLinkResponseSchema = platformAccountLinkSchema.nullable();

export const categoryItemSchema = z.object({
  id: z.string().min(1),
  name: z.string(),
  boxArtUrl: z.string().optional(),
});
export type CategoryItem = z.infer<typeof categoryItemSchema>;

export const categorySearchResponseSchema = z.object({
  items: z.array(categoryItemSchema),
});

export const fieldDiffSchema = z.object({
  field: z.string(),
  local: z.string(),
  remote: z.string(),
  changed: z.boolean(),
});
export type FieldDiff = z.infer<typeof fieldDiffSchema>;

export const publishPreviewSchema = z.object({
  providerId: z.string(),
  accountId: z.string().optional(),
  accountLogin: z.string().optional(),
  fields: z.array(fieldDiffSchema),
  skipped: z.array(z.string()),
  blockers: z.array(z.string()),
  allowed: z.boolean(),
});
export type PublishPreview = z.infer<typeof publishPreviewSchema>;

export const publishResultSchema = z.object({
  status: z.enum(['published', 'blocked']),
  accountId: z.string().optional(),
  publishedAt: z.string().optional(),
  fieldsChanged: z.array(z.string()).optional(),
  fieldsSkipped: z.array(z.string()).optional(),
  blockers: z.array(z.string()).optional(),
});
export type PublishResult = z.infer<typeof publishResultSchema>;
