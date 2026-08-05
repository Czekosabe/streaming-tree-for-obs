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
  // YouTube only: the selected live broadcast this preview compares
  // against - absent for every other provider.
  broadcastId: z.string().optional(),
  broadcastTitle: z.string().optional(),
  fields: z.array(fieldDiffSchema),
  skipped: z.array(z.string()),
  blockers: z.array(z.string()),
  warnings: z.array(z.string()).optional(),
  allowed: z.boolean(),
});
export type PublishPreview = z.infer<typeof publishPreviewSchema>;

export const publishResultSchema = z.object({
  status: z.enum(['published', 'blocked']),
  accountId: z.string().optional(),
  broadcastId: z.string().optional(),
  publishedAt: z.string().optional(),
  fieldsChanged: z.array(z.string()).optional(),
  fieldsSkipped: z.array(z.string()).optional(),
  // YouTube only: fields a multi-call publish attempted but failed to
  // change - always empty this stage (a single-call publish path), kept so
  // a future multi-call publish can report a genuine partial result
  // without a schema change - see docs/provider-integrations/youtube.md.
  fieldsFailed: z.array(z.string()).optional(),
  warnings: z.array(z.string()).optional(),
  blockers: z.array(z.string()).optional(),
});
export type PublishResult = z.infer<typeof publishResultSchema>;

/**
 * YouTube's own OAuth attempt state machine: Authorization Code Flow with
 * PKCE and a loopback callback, distinct from Twitch's device-flow states -
 * see docs/provider-integrations/youtube.md for why these are not unified
 * into one shape.
 */
export const oauthAttemptStateSchema = z.enum([
  'creating',
  'waiting_for_browser',
  'processing_callback',
  'awaiting_channel_selection',
  'authorized',
  'denied',
  'expired',
  'cancelled',
  'error',
]);
export type OAuthAttemptState = z.infer<typeof oauthAttemptStateSchema>;

export const channelSummarySchema = z.object({
  channelId: z.string().min(1),
  title: z.string(),
  thumbnailUrl: z.string().optional(),
});
export type ChannelSummary = z.infer<typeof channelSummarySchema>;

export const oauthAttemptSnapshotSchema = z.object({
  attemptId: z.string().min(1),
  providerId: z.string().min(1),
  state: oauthAttemptStateSchema,
  // Ephemeral, security-sensitive: present only while waiting for the
  // browser. Never an authorization code, a PKCE verifier, or a state
  // value - those fields do not exist anywhere in this shape (see
  // internal/httpapi/youtube.go's oauthAttemptResponse).
  authorizationUrl: z.string().optional(),
  createdAt: z.string(),
  expiresAt: z.string().optional(),
  connectedAccountId: z.string().optional(),
  channels: z.array(channelSummarySchema).optional(),
  errorCode: z.string().optional(),
  errorMessage: z.string().optional(),
});
export type OAuthAttemptSnapshot = z.infer<typeof oauthAttemptSnapshotSchema>;

export const broadcastItemSchema = z.object({
  id: z.string().min(1),
  title: z.string(),
  lifeCycleStatus: z.string(),
  privacyStatus: z.string(),
  scheduledStartTime: z.string().optional(),
  actualStartTime: z.string().optional(),
});
export type BroadcastItem = z.infer<typeof broadcastItemSchema>;

export const broadcastListResponseSchema = z.object({
  items: z.array(broadcastItemSchema),
});

export const remoteTargetSchema = z.object({
  platformId: z.string().min(1),
  providerId: z.string().min(1),
  resourceType: z.string().min(1),
  resourceId: z.string().min(1),
  displayName: z.string(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type RemoteTarget = z.infer<typeof remoteTargetSchema>;

/** `GET /api/platforms/{id}/remote-target` answers `null` when unset. */
export const remoteTargetResponseSchema = remoteTargetSchema.nullable();

export const regionResponseSchema = z.object({
  region: z.string(),
});
