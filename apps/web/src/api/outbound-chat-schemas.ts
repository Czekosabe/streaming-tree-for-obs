import { z } from 'zod';

/**
 * Zod contracts for the Stage 11A manual outbound-chat API
 * (`internal/httpapi/outboundchat.go`).
 *
 * No field here ever carries a token, a raw provider response, or the sent
 * message's own text - the backend response shapes never carry one, so
 * there is nothing to strip.
 */

/** Mirrors internal/httpapi's own 4-value capability label. */
export const outboundChatCapabilitySchema = z.enum([
  'unsupported',
  'permission_required',
  'ready',
  'error',
]);
export type OutboundChatCapability = z.infer<typeof outboundChatCapabilitySchema>;

export const outboundChatDispatcherStateSchema = z.enum([
  'idle',
  'queued',
  'sending',
  'rate_limited',
  'stopping',
  'error',
]);
export type OutboundChatDispatcherState = z.infer<typeof outboundChatDispatcherStateSchema>;

export const outboundChatStatusSchema = z.object({
  providerId: z.string(),
  capability: outboundChatCapabilitySchema,
  requiredScopes: z.array(z.string()).optional(),
  grantedScopes: z.array(z.string()).optional(),
  missingScopes: z.array(z.string()).optional(),
  dispatcherState: outboundChatDispatcherStateSchema,
  queueDepth: z.number(),
  queueCapacity: z.number(),
  lastAttemptAt: z.string().optional(),
  lastSuccessAt: z.string().optional(),
  lastErrorCode: z.string().optional(),
  retryAt: z.string().optional(),
  canSendNow: z.boolean(),
  sharedChatWarning: z.string(),
});
export type OutboundChatStatus = z.infer<typeof outboundChatStatusSchema>;

export const sendOutboundChatMessageResponseSchema = z.object({
  sent: z.boolean(),
  providerMessageId: z.string().optional(),
  sentAt: z.string().optional(),
});
export type SendOutboundChatMessageResponse = z.infer<typeof sendOutboundChatMessageResponseSchema>;

/** Request body for `POST /api/connected-accounts/{id}/outbound-chat/messages`. */
export type SendOutboundChatMessageInput = {
  message: string;
  replyParentMessageId?: string;
};

/** Every stable outbound_chat_* error code the backend can return, plus the
 * shared account_not_found code it reuses from the rest of the API - used
 * to map `ApiError.code` to a specific composer state. */
export const outboundChatErrorCodeSchema = z.enum([
  'account_not_found',
  'outbound_chat_unsupported',
  'outbound_chat_permission_required',
  'outbound_chat_unavailable',
  'outbound_chat_queue_full',
  'outbound_chat_rate_limited',
  'outbound_chat_forbidden',
  'outbound_chat_message_dropped',
  'outbound_chat_delivery_unknown',
  'outbound_chat_provider_failure',
  'outbound_chat_cancelled',
  'account_reconnect_required',
  'validation_failed',
]);
export type OutboundChatErrorCode = z.infer<typeof outboundChatErrorCodeSchema>;
