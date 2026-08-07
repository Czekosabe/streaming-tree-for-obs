import { deviceFlowSnapshotSchema, type DeviceFlowSnapshot } from '@/api/account-schemas';
import { apiGet, apiPost } from '@/lib/api-client';

import {
  outboundChatStatusSchema,
  sendOutboundChatMessageResponseSchema,
  type OutboundChatStatus,
  type SendOutboundChatMessageInput,
  type SendOutboundChatMessageResponse,
} from './outbound-chat-schemas';

/**
 * Transport for the Stage 11A manual outbound-chat API. No caching or React
 * concerns live here - see hooks/use-outbound-chat.ts. The browser never
 * builds a Twitch request itself; every call here is a plain call to this
 * application's own backend.
 */

const NO_BODY = undefined;

export async function fetchOutboundChatStatus(
  accountId: string,
  signal?: AbortSignal,
): Promise<OutboundChatStatus> {
  return apiGet(`/api/connected-accounts/${accountId}/outbound-chat`, outboundChatStatusSchema, {
    signal,
  });
}

/** Starts the identity-bound, union-scoped Twitch outbound-chat permission
 * upgrade flow - reuses the same device-flow response shape as every other
 * permission upgrade in this application. */
export async function authorizeOutboundChat(accountId: string): Promise<DeviceFlowSnapshot> {
  return apiPost(
    `/api/connected-accounts/${accountId}/outbound-chat/authorize`,
    NO_BODY,
    deviceFlowSnapshotSchema,
  );
}

export async function sendOutboundChatMessage(
  accountId: string,
  input: SendOutboundChatMessageInput,
): Promise<SendOutboundChatMessageResponse> {
  return apiPost(
    `/api/connected-accounts/${accountId}/outbound-chat/messages`,
    input,
    sendOutboundChatMessageResponseSchema,
  );
}
