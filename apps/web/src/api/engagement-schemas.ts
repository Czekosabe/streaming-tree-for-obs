import { z } from 'zod';

/**
 * Zod contracts for the Stage 8A Engagement Event Bus and Twitch
 * connector-management API.
 *
 * None of these schemas has a field for an access/refresh token, a
 * WebSocket session id, a reconnect URL, or a raw provider payload - the
 * backend response shapes themselves never carry one (see
 * internal/httpapi/engagement.go), so there is nothing to strip here; the
 * absence is structural, exactly like account-schemas.ts's own contract.
 */

export const CURRENT_ENGAGEMENT_SCHEMA_VERSION = 1;

export const connectorStateSchema = z.enum([
  'disabled',
  'blocked',
  'connecting',
  'waiting_for_welcome',
  'subscribing',
  'connected',
  'reconnecting',
  'stopping',
  'error',
]);
export type ConnectorState = z.infer<typeof connectorStateSchema>;

export const connectorSchema = z.object({
  accountId: z.string().min(1),
  enabled: z.boolean(),
  state: connectorStateSchema,
  blockerCodes: z.array(z.string()).optional(),
  missingScopes: z.array(z.string()).optional(),
  connectedAt: z.string().optional(),
  lastEventAt: z.string().optional(),
  lastKeepaliveAt: z.string().optional(),
  lastDataGapAt: z.string().optional(),
  reconnectCount: z.number(),
  activeSubscriptionCount: z.number(),
  expectedSubscriptionCount: z.number(),
  lastError: z.string().optional(),
});
export type Connector = z.infer<typeof connectorSchema>;

export const accountEngagementSchema = connectorSchema.extend({
  requiredScopes: z.array(z.string()),
  grantedScopes: z.array(z.string()),
  permissionUpgradeRequired: z.boolean(),
});
export type AccountEngagement = z.infer<typeof accountEngagementSchema>;

export const engagementStatusSchema = z.object({
  schemaVersion: z.number(),
  bufferCapacity: z.number(),
  retainedCount: z.number(),
  oldestSequence: z.number(),
  newestSequence: z.number(),
  activeSubscribers: z.number(),
  connectors: z.array(connectorSchema),
});
export type EngagementStatus = z.infer<typeof engagementStatusSchema>;

export const eventTypeSchema = z.enum([
  'chat.message',
  'chat.message_deleted',
  'chat.cleared',
  'moderation',
  'follow',
  'subscription',
  'resubscription',
  'gifted_subscription',
  'subscription_gift_batch',
  'bits',
  'raid',
  'channel_point_redemption',
  'stream.online',
  'stream.offline',
]);
export type EngagementEventType = z.infer<typeof eventTypeSchema>;

export const fragmentTypeSchema = z.enum(['text', 'emote', 'cheermote', 'mention', 'unknown']);
export type FragmentType = z.infer<typeof fragmentTypeSchema>;

export const fragmentSchema = z.object({
  type: fragmentTypeSchema,
  text: z.string(),
  emoteId: z.string().optional(),
  cheermotePrefix: z.string().optional(),
  cheermoteBits: z.number().optional(),
  mentionUserId: z.string().optional(),
  mentionLogin: z.string().optional(),
  mentionDisplayName: z.string().optional(),
});
export type Fragment = z.infer<typeof fragmentSchema>;

export const eventMessageSchema = z.object({
  text: z.string(),
  fragments: z.array(fragmentSchema),
});
export type EventMessage = z.infer<typeof eventMessageSchema>;

export const eventBadgeSchema = z.object({
  setId: z.string(),
  id: z.string(),
  info: z.string().optional(),
});
export type EventBadge = z.infer<typeof eventBadgeSchema>;

export const eventUserSchema = z.object({
  providerUserId: z.string().optional(),
  login: z.string().optional(),
  displayName: z.string().optional(),
  avatarUrl: z.string().optional(),
  color: z.string().optional(),
  badges: z.array(eventBadgeSchema).optional(),
  roles: z.array(z.string()).optional(),
  anonymous: z.boolean(),
});
export type EventUser = z.infer<typeof eventUserSchema>;

export const engagementEventSchema = z.object({
  schemaVersion: z.number(),
  sequence: z.number(),
  id: z.string().min(1),
  providerEventId: z.string().optional(),
  providerId: z.string().min(1),
  connectedAccountId: z.string().min(1),
  destinationId: z.string().optional(),
  type: eventTypeSchema,
  providerEventType: z.string(),
  platformTimestamp: z.string(),
  receivedAt: z.string(),
  synthetic: z.boolean(),
  user: eventUserSchema.optional(),
  message: eventMessageSchema.optional(),
  amount: z.number().optional(),
  currency: z.string().optional(),
  quantity: z.number().optional(),
  moderationRef: z.string().optional(),
  moderationAction: z.string().optional(),
  providerExtra: z.record(z.string(), z.string()).optional(),
});
export type EngagementEvent = z.infer<typeof engagementEventSchema>;

export const engagementEventsResponseSchema = z.object({
  items: z.array(engagementEventSchema),
  gap: z.boolean(),
});
export type EngagementEventsResponse = z.infer<typeof engagementEventsResponseSchema>;

/** Payload accepted by `PUT /api/connected-accounts/{id}/engagement`. */
export type SetEngagementInput = { enabled: boolean };
