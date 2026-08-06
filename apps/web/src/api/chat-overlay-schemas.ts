import { z } from 'zod';

/**
 * Zod contracts for the Stage 10 chat-overlay management and public APIs
 * (`internal/httpapi/chatoverlay.go`).
 *
 * Mirrors operator-chat-schemas.ts's own reasoning: no field here carries a
 * token, a session id, a raw provider payload, or (for the public schemas)
 * a management id, a blocked term, or a hidden-user list - the backend
 * response shapes never carry one, so there is nothing to strip.
 */

export const CURRENT_CHAT_OVERLAY_SCHEMA_VERSION = 1;

export const chatOverlayLayoutModeSchema = z.enum(['horizontal', 'vertical']);
export type ChatOverlayLayoutMode = z.infer<typeof chatOverlayLayoutModeSchema>;

export const chatOverlayStackDirectionSchema = z.enum(['top_down', 'bottom_up']);
export type ChatOverlayStackDirection = z.infer<typeof chatOverlayStackDirectionSchema>;

export const chatOverlayHorizontalAlignmentSchema = z.enum(['left', 'center', 'right']);
export type ChatOverlayHorizontalAlignment = z.infer<typeof chatOverlayHorizontalAlignmentSchema>;

export const chatOverlayFontFamilySchema = z.enum(['sans_serif', 'serif', 'monospace', 'rounded']);
export type ChatOverlayFontFamily = z.infer<typeof chatOverlayFontFamilySchema>;

export const chatOverlayUsernameColorModeSchema = z.enum(['provider', 'fixed']);
export type ChatOverlayUsernameColorMode = z.infer<typeof chatOverlayUsernameColorModeSchema>;

export const chatOverlayAnimationSchema = z.enum(['none', 'fade', 'slide_up', 'slide_left', 'scale']);
export type ChatOverlayAnimation = z.infer<typeof chatOverlayAnimationSchema>;

export const chatOverlayLanguageSchema = z.enum(['en', 'pl']);
export type ChatOverlayLanguage = z.infer<typeof chatOverlayLanguageSchema>;

export const chatOverlayMatchModeSchema = z.enum(['contains', 'whole_word']);
export type ChatOverlayMatchMode = z.infer<typeof chatOverlayMatchModeSchema>;

/** Editable overlay settings, shared by the create request, the PUT
 * request, and the persisted profile response. */
const chatOverlayEditableFieldsSchema = z.object({
  name: z.string(),
  enabled: z.boolean(),

  layoutMode: chatOverlayLayoutModeSchema,
  stackDirection: chatOverlayStackDirectionSchema,
  horizontalAlignment: chatOverlayHorizontalAlignmentSchema,

  showPlatformIcon: z.boolean(),
  showPlatformName: z.boolean(),
  showAccountLabel: z.boolean(),
  showAvatar: z.boolean(),
  showBadges: z.boolean(),
  showTimestamp: z.boolean(),
  showActivityEvents: z.boolean(),
  showDeletedPlaceholder: z.boolean(),
  hideCommands: z.boolean(),
  hideBots: z.boolean(),

  maxVisibleItems: z.number().int(),
  messageLifetimeSeconds: z.number().int(),

  fontFamily: chatOverlayFontFamilySchema,
  fontSize: z.number().int(),
  fontWeight: z.number().int(),
  lineHeight: z.number(),
  textColor: z.string(),
  usernameColorMode: chatOverlayUsernameColorModeSchema,
  bubbleColor: z.string(),
  bubbleOpacity: z.number(),
  borderRadius: z.number().int(),
  itemSpacing: z.number().int(),
  textOutline: z.boolean(),
  textShadow: z.boolean(),

  entryAnimation: chatOverlayAnimationSchema,
  exitAnimation: chatOverlayAnimationSchema,
  animationDurationMs: z.number().int(),

  highlightBroadcaster: z.boolean(),
  highlightModerators: z.boolean(),
  highlightSubscribers: z.boolean(),
  highlightVips: z.boolean(),

  language: chatOverlayLanguageSchema,
});
export type ChatOverlayEditableFields = z.infer<typeof chatOverlayEditableFieldsSchema>;

export const chatOverlayProfileSchema = chatOverlayEditableFieldsSchema.extend({
  id: z.string().min(1),
  publicSlug: z.string().min(1),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type ChatOverlayProfile = z.infer<typeof chatOverlayProfileSchema>;

export const chatOverlayProfilesResponseSchema = z.object({
  items: z.array(chatOverlayProfileSchema),
});

export const chatOverlayAccountsResponseSchema = z.object({
  accountIds: z.array(z.string()),
});

export const chatOverlayHiddenUserSchema = z.object({
  providerId: z.string().min(1),
  connectedAccountId: z.string().min(1),
  providerUserId: z.string().min(1),
  label: z.string().optional(),
  createdAt: z.string(),
});
export type ChatOverlayHiddenUser = z.infer<typeof chatOverlayHiddenUserSchema>;

export const chatOverlayHiddenUsersResponseSchema = z.object({
  items: z.array(chatOverlayHiddenUserSchema),
});

export type AddChatOverlayHiddenUserInput = {
  providerId: string;
  connectedAccountId: string;
  providerUserId: string;
  label?: string;
};

export const chatOverlayBlockedTermSchema = z.object({
  id: z.string().min(1),
  value: z.string(),
  matchMode: chatOverlayMatchModeSchema,
  createdAt: z.string(),
});
export type ChatOverlayBlockedTerm = z.infer<typeof chatOverlayBlockedTermSchema>;

export const chatOverlayBlockedTermsResponseSchema = z.object({
  items: z.array(chatOverlayBlockedTermSchema),
});

export const chatOverlayActivityTypesResponseSchema = z.object({
  activityTypes: z.array(z.string()),
});

// --- public API --------------------------------------------------------

/** Renderer-only settings - never a management id, blocked terms, hidden
 * users, or an account id. See internal/httpapi's own doc comment on
 * `publicChatOverlayConfigResponse` for the full "why" behind the
 * subset. */
export const publicChatOverlayConfigSchema = z.object({
  schemaVersion: z.number(),

  layoutMode: chatOverlayLayoutModeSchema,
  stackDirection: chatOverlayStackDirectionSchema,
  horizontalAlignment: chatOverlayHorizontalAlignmentSchema,

  showPlatformIcon: z.boolean(),
  showPlatformName: z.boolean(),
  showTimestamp: z.boolean(),

  maxVisibleItems: z.number().int(),
  messageLifetimeSeconds: z.number().int(),

  fontFamily: chatOverlayFontFamilySchema,
  fontSize: z.number().int(),
  fontWeight: z.number().int(),
  lineHeight: z.number(),
  textColor: z.string(),
  usernameColorMode: chatOverlayUsernameColorModeSchema,
  bubbleColor: z.string(),
  bubbleOpacity: z.number(),
  borderRadius: z.number().int(),
  itemSpacing: z.number().int(),
  textOutline: z.boolean(),
  textShadow: z.boolean(),

  entryAnimation: chatOverlayAnimationSchema,
  exitAnimation: chatOverlayAnimationSchema,
  animationDurationMs: z.number().int(),

  highlightBroadcaster: z.boolean(),
  highlightModerators: z.boolean(),
  highlightSubscribers: z.boolean(),
  highlightVips: z.boolean(),

  language: chatOverlayLanguageSchema,
});
export type PublicChatOverlayConfig = z.infer<typeof publicChatOverlayConfigSchema>;

export const publicChatOverlayKindSchema = z.enum(['message', 'activity']);
export type PublicChatOverlayKind = z.infer<typeof publicChatOverlayKindSchema>;

export const publicChatOverlayBadgeSchema = z.object({
  setId: z.string(),
  id: z.string(),
  imageUrl1x: z.string().optional(),
  imageUrl2x: z.string().optional(),
  imageUrl4x: z.string().optional(),
});
export type PublicChatOverlayBadge = z.infer<typeof publicChatOverlayBadgeSchema>;

export const publicChatOverlayUserSchema = z.object({
  displayName: z.string().optional(),
  color: z.string().optional(),
  avatarUrl: z.string().optional(),
  badges: z.array(publicChatOverlayBadgeSchema).optional(),
  anonymous: z.boolean(),
  isBroadcaster: z.boolean().optional(),
  isModerator: z.boolean().optional(),
  isSubscriber: z.boolean().optional(),
  isVip: z.boolean().optional(),
});
export type PublicChatOverlayUser = z.infer<typeof publicChatOverlayUserSchema>;

export const publicChatOverlayFragmentTypeSchema = z.enum(['text', 'emote', 'mention']);
export type PublicChatOverlayFragmentType = z.infer<typeof publicChatOverlayFragmentTypeSchema>;

export const publicChatOverlayFragmentSchema = z.object({
  type: publicChatOverlayFragmentTypeSchema,
  text: z.string(),
  emoteImageUrl: z.string().optional(),
});
export type PublicChatOverlayFragment = z.infer<typeof publicChatOverlayFragmentSchema>;

export const publicChatOverlayMessageSchema = z.object({
  plainText: z.string(),
  fragments: z.array(publicChatOverlayFragmentSchema),
});
export type PublicChatOverlayMessage = z.infer<typeof publicChatOverlayMessageSchema>;

export const publicChatOverlayActivitySchema = z.object({
  activityType: z.string(),
  amount: z.number().optional(),
  currency: z.string().optional(),
  quantity: z.number().optional(),
});
export type PublicChatOverlayActivity = z.infer<typeof publicChatOverlayActivitySchema>;

export const publicChatOverlayItemSchema = z.object({
  version: z.number(),
  sequence: z.number(),
  id: z.string().min(1),
  kind: publicChatOverlayKindSchema,
  providerId: z.string().min(1),
  accountLabel: z.string().optional(),
  occurredAt: z.string(),
  user: publicChatOverlayUserSchema.optional(),
  message: publicChatOverlayMessageSchema.optional(),
  activity: publicChatOverlayActivitySchema.optional(),
  deleted: z.boolean(),
  synthetic: z.boolean(),
});
export type PublicChatOverlayItem = z.infer<typeof publicChatOverlayItemSchema>;

export const publicChatOverlayItemsResponseSchema = z.object({
  items: z.array(publicChatOverlayItemSchema),
});

export const publicChatOverlayResetPayloadSchema = z.object({
  items: z.array(publicChatOverlayItemSchema),
});

/**
 * The reason a public overlay item was removed - mirrors
 * `internal/chatoverlay.RemoveReason` exactly (six values, not the
 * corrective task's own larger example list - see that Go type's own
 * doc comment for why a settings/privacy change never reaches this
 * schema at all: it always travels as a full `reset`, not an
 * individual `remove`). Only `expired` and `capacity_evicted` are safe
 * to animate - see `isCosmeticRemoveReason` below.
 *
 * `.catch('unknown')` makes an unrecognized reason value fail safely
 * closed to the immediate-removal case rather than dropping the whole
 * event - a malformed/unknown reason must never cause an item to be
 * retained on screen for an animation it was never meant to get.
 */
export const chatOverlayRemoveReasonSchema = z
  .enum(['expired', 'capacity_evicted', 'message_deleted', 'chat_cleared', 'user_messages_cleared', 'unknown'])
  .catch('unknown');
export type ChatOverlayRemoveReason = z.infer<typeof chatOverlayRemoveReasonSchema>;

const COSMETIC_REMOVE_REASONS: ReadonlySet<ChatOverlayRemoveReason> = new Set(['expired', 'capacity_evicted']);

/** Whether `reason` is safe for the frontend to animate as a "leaving"
 * transition rather than removing immediately - mirrors
 * `internal/chatoverlay.RemoveReason.IsCosmetic` exactly. */
export function isCosmeticRemoveReason(reason: ChatOverlayRemoveReason): boolean {
  return COSMETIC_REMOVE_REASONS.has(reason);
}

export const publicChatOverlayRemovePayloadSchema = z.object({
  id: z.string().min(1),
  reason: chatOverlayRemoveReasonSchema,
});
