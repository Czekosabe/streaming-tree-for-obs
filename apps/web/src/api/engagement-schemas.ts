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
  /** Always present on the wire (internal/httpapi/engagement.go's
   * connectorResponse.Provider has no `omitempty`) - "twitch" or
   * "youtube" today. */
  provider: z.string().min(1),
  enabled: z.boolean(),
  state: connectorStateSchema,
  blockerCodes: z.array(z.string()).optional(),
  missingScopes: z.array(z.string()).optional(),
  connectedAt: z.string().optional(),
  lastEventAt: z.string().optional(),
  lastKeepaliveAt: z.string().optional(),
  lastDataGapAt: z.string().optional(),
  reconnectCount: z.number(),
  /** `omitempty` on the wire - a YouTube connector (which has no
   * subscription concept) legitimately omits both at zero, so these
   * must be optional rather than defaulting to a false "always
   * present" assumption inherited from Twitch being the only provider
   * before Stage 15A. */
  activeSubscriptionCount: z.number().optional(),
  expectedSubscriptionCount: z.number().optional(),
  lastError: z.string().optional(),
  /** Stage 15A YouTube-only fields - always absent for a Twitch
   * connector's own snapshot. */
  selectedBroadcastId: z.string().optional(),
  lastPollAt: z.string().optional(),
  possibleGapCount: z.number().optional(),
  unsupportedEventCount: z.number().optional(),
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
  'youtube.membership',
  'youtube.membership_milestone',
  'youtube.super_chat',
  'youtube.super_sticker',
  'donation',
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
  /** Stage 15A real monetary value (YouTube Super Chat/Super Sticker) -
   * mirrors internal/httpapi/engagement.go's eventResponse exactly
   * (amountMicros/currency/displayAmount, never a plain float "amount"
   * field - internal/domain/engagement.Money is integer-micros only). */
  amountMicros: z.number().optional(),
  currency: z.string().optional(),
  displayAmount: z.string().optional(),
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
