import { z } from 'zod';

/**
 * Zod contracts for the Stage 9 unified-operator-chat projection API
 * (`internal/httpapi/operatorchat.go`).
 *
 * Mirrors engagement-schemas.ts's own reasoning: no field here carries a
 * token, a session id, a reconnect URL, or a raw provider payload - the
 * backend response shapes never carry one, so there is nothing to strip.
 */

export const CURRENT_OPERATOR_CHAT_SCHEMA_VERSION = 1;

export const operatorChatKindSchema = z.enum(['message', 'activity', 'moderation', 'system']);
export type OperatorChatKind = z.infer<typeof operatorChatKindSchema>;

export const operatorChatBadgeSchema = z.object({
  setId: z.string(),
  id: z.string(),
  info: z.string().optional(),
  imageUrl1x: z.string().optional(),
  imageUrl2x: z.string().optional(),
  imageUrl4x: z.string().optional(),
});
export type OperatorChatBadge = z.infer<typeof operatorChatBadgeSchema>;

export const operatorChatUserSchema = z.object({
  providerUserId: z.string().optional(),
  login: z.string().optional(),
  displayName: z.string().optional(),
  avatarUrl: z.string().optional(),
  color: z.string().optional(),
  badges: z.array(operatorChatBadgeSchema).optional(),
  anonymous: z.boolean(),
});
export type OperatorChatUser = z.infer<typeof operatorChatUserSchema>;

export const operatorChatFragmentTypeSchema = z.enum([
  'text',
  'emote',
  'cheermote',
  'mention',
  'unknown',
]);
export type OperatorChatFragmentType = z.infer<typeof operatorChatFragmentTypeSchema>;

export const operatorChatFragmentSchema = z.object({
  type: operatorChatFragmentTypeSchema,
  text: z.string(),
  emoteId: z.string().optional(),
  emoteImageUrl: z.string().optional(),
  cheermotePrefix: z.string().optional(),
  cheermoteBits: z.number().optional(),
  mentionUserId: z.string().optional(),
  mentionLogin: z.string().optional(),
  mentionDisplayName: z.string().optional(),
});
export type OperatorChatFragment = z.infer<typeof operatorChatFragmentSchema>;

export const operatorChatMessageSchema = z.object({
  plainText: z.string(),
  fragments: z.array(operatorChatFragmentSchema),
});
export type OperatorChatMessage = z.infer<typeof operatorChatMessageSchema>;

export const operatorChatActivitySchema = z.object({
  activityType: z.string(),
  amount: z.number().optional(),
  currency: z.string().optional(),
  quantity: z.number().optional(),
});
export type OperatorChatActivity = z.infer<typeof operatorChatActivitySchema>;

export const operatorChatModerationSchema = z.object({
  action: z.string(),
  targetUserId: z.string().optional(),
  targetMessageRef: z.string().optional(),
});
export type OperatorChatModeration = z.infer<typeof operatorChatModerationSchema>;

export const operatorChatDeletionReasonSchema = z.enum([
  'moderator_deleted',
  'chat_cleared',
  'user_messages_cleared',
]);
export type OperatorChatDeletionReason = z.infer<typeof operatorChatDeletionReasonSchema>;

export const operatorChatLifecycleSchema = z.object({
  deleted: z.boolean(),
  deletedAt: z.string().optional(),
  deletionReason: z.union([operatorChatDeletionReasonSchema, z.literal('')]).optional(),
});
export type OperatorChatLifecycle = z.infer<typeof operatorChatLifecycleSchema>;

export const operatorChatItemSchema = z.object({
  version: z.number(),
  sequence: z.number(),
  id: z.string().min(1),
  sourceEventId: z.string().optional(),
  providerId: z.string().min(1),
  connectedAccountId: z.string().min(1),
  destinationId: z.string().optional(),
  kind: operatorChatKindSchema,
  occurredAt: z.string(),
  receivedAt: z.string(),
  user: operatorChatUserSchema.optional(),
  message: operatorChatMessageSchema.optional(),
  activity: operatorChatActivitySchema.optional(),
  moderation: operatorChatModerationSchema.optional(),
  lifecycle: operatorChatLifecycleSchema,
  synthetic: z.boolean(),
});
export type OperatorChatItem = z.infer<typeof operatorChatItemSchema>;

export const operatorChatItemsResponseSchema = z.object({
  items: z.array(operatorChatItemSchema),
  gap: z.boolean(),
});
export type OperatorChatItemsResponse = z.infer<typeof operatorChatItemsResponseSchema>;

export const operatorChatStatusSchema = z.object({
  schemaVersion: z.number(),
  bufferCapacity: z.number(),
  retainedCount: z.number(),
  oldestSequence: z.number(),
  newestSequence: z.number(),
  activeSubscribers: z.number(),
  busGap: z.boolean(),
});
export type OperatorChatStatus = z.infer<typeof operatorChatStatusSchema>;

export const operatorChatPreferencesSchema = z.object({
  showPlatformIcon: z.boolean(),
  showPlatformName: z.boolean(),
  showAccountLabel: z.boolean(),
  showBadges: z.boolean(),
  showTimestamps: z.boolean(),
  showActivityEvents: z.boolean(),
  showDeletedMessages: z.boolean(),
  hideCommandMessages: z.boolean(),
  compactMode: z.boolean(),
});
export type OperatorChatPreferences = z.infer<typeof operatorChatPreferencesSchema>;

/** Mirrors the backend's own documented defaults
 * (internal/domain/operatorchatprefs.Default) - used only until the real
 * preferences have loaded, so the page never flashes a different layout. */
export const DEFAULT_OPERATOR_CHAT_PREFERENCES: OperatorChatPreferences = {
  showPlatformIcon: true,
  showPlatformName: false,
  showAccountLabel: true,
  showBadges: true,
  showTimestamps: true,
  showActivityEvents: true,
  showDeletedMessages: true,
  hideCommandMessages: false,
  compactMode: false,
};

export const operatorChatAccountVisibilitySchema = z.object({
  accountId: z.string().min(1),
  visible: z.boolean(),
});
export type OperatorChatAccountVisibility = z.infer<typeof operatorChatAccountVisibilitySchema>;

export const operatorChatAccountVisibilityResponseSchema = z.object({
  items: z.array(operatorChatAccountVisibilitySchema),
});

export const operatorChatUserRefSchema = z.object({
  id: z.string().min(1),
  providerId: z.string().min(1),
  connectedAccountId: z.string().min(1),
  providerUserId: z.string().min(1),
  label: z.string().optional(),
  createdAt: z.string(),
});
export type OperatorChatUserRef = z.infer<typeof operatorChatUserRefSchema>;

export const operatorChatUserRefsResponseSchema = z.object({
  items: z.array(operatorChatUserRefSchema),
});

/** Payload accepted by `POST /api/operator-chat/hidden-users` and
 * `/bot-users`. */
export type AddOperatorChatUserRefInput = {
  providerId: string;
  connectedAccountId: string;
  providerUserId: string;
  label?: string;
};
