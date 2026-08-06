import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  chatOverlayAccountsResponseSchema,
  chatOverlayActivityTypesResponseSchema,
  chatOverlayBlockedTermSchema,
  chatOverlayBlockedTermsResponseSchema,
  chatOverlayHiddenUserSchema,
  chatOverlayHiddenUsersResponseSchema,
  chatOverlayProfileSchema,
  chatOverlayProfilesResponseSchema,
  publicChatOverlayConfigSchema,
  publicChatOverlayItemsResponseSchema,
  type AddChatOverlayHiddenUserInput,
  type ChatOverlayBlockedTerm,
  type ChatOverlayEditableFields,
  type ChatOverlayHiddenUser,
  type ChatOverlayMatchMode,
  type ChatOverlayProfile,
  type PublicChatOverlayConfig,
  type PublicChatOverlayItem,
} from './chat-overlay-schemas';

/**
 * Transport for the Stage 10 chat-overlay management and public APIs. No
 * caching or React concerns live here - see hooks/use-chat-overlay.ts and
 * hooks/use-chat-overlay-stream.ts.
 */

export async function fetchChatOverlays(signal?: AbortSignal): Promise<ChatOverlayProfile[]> {
  const response = await apiGet('/api/chat-overlays', chatOverlayProfilesResponseSchema, { signal });
  return response.items;
}

export async function fetchChatOverlay(
  id: string,
  signal?: AbortSignal,
): Promise<ChatOverlayProfile> {
  return apiGet(`/api/chat-overlays/${id}`, chatOverlayProfileSchema, { signal });
}

export async function createChatOverlay(name: string): Promise<ChatOverlayProfile> {
  return apiPost('/api/chat-overlays', { name }, chatOverlayProfileSchema);
}

export async function replaceChatOverlay(
  id: string,
  input: ChatOverlayEditableFields,
): Promise<ChatOverlayProfile> {
  return apiPut(`/api/chat-overlays/${id}`, input, chatOverlayProfileSchema);
}

export async function deleteChatOverlay(id: string): Promise<void> {
  return apiDelete(`/api/chat-overlays/${id}`);
}

export async function rotateChatOverlayPublicSlug(id: string): Promise<ChatOverlayProfile> {
  return apiPost(`/api/chat-overlays/${id}/rotate-public-slug`, undefined, chatOverlayProfileSchema);
}

export async function fetchChatOverlayAccounts(
  overlayId: string,
  signal?: AbortSignal,
): Promise<string[]> {
  const response = await apiGet(
    `/api/chat-overlays/${overlayId}/accounts`,
    chatOverlayAccountsResponseSchema,
    { signal },
  );
  return response.accountIds;
}

export async function setChatOverlayAccounts(
  overlayId: string,
  accountIds: string[],
): Promise<string[]> {
  const response = await apiPut(
    `/api/chat-overlays/${overlayId}/accounts`,
    { accountIds },
    chatOverlayAccountsResponseSchema,
  );
  return response.accountIds;
}

export async function fetchChatOverlayHiddenUsers(
  overlayId: string,
  signal?: AbortSignal,
): Promise<ChatOverlayHiddenUser[]> {
  const response = await apiGet(
    `/api/chat-overlays/${overlayId}/hidden-users`,
    chatOverlayHiddenUsersResponseSchema,
    { signal },
  );
  return response.items;
}

export async function addChatOverlayHiddenUser(
  overlayId: string,
  input: AddChatOverlayHiddenUserInput,
): Promise<ChatOverlayHiddenUser> {
  return apiPost(`/api/chat-overlays/${overlayId}/hidden-users`, input, chatOverlayHiddenUserSchema);
}

export async function removeChatOverlayHiddenUser(
  overlayId: string,
  ref: { providerId: string; connectedAccountId: string; providerUserId: string },
): Promise<void> {
  const params = new URLSearchParams({
    providerId: ref.providerId,
    connectedAccountId: ref.connectedAccountId,
    providerUserId: ref.providerUserId,
  });
  return apiDelete(`/api/chat-overlays/${overlayId}/hidden-users?${params.toString()}`);
}

export async function fetchChatOverlayBlockedTerms(
  overlayId: string,
  signal?: AbortSignal,
): Promise<ChatOverlayBlockedTerm[]> {
  const response = await apiGet(
    `/api/chat-overlays/${overlayId}/blocked-terms`,
    chatOverlayBlockedTermsResponseSchema,
    { signal },
  );
  return response.items;
}

export async function addChatOverlayBlockedTerm(
  overlayId: string,
  value: string,
  matchMode: ChatOverlayMatchMode,
): Promise<ChatOverlayBlockedTerm> {
  return apiPost(
    `/api/chat-overlays/${overlayId}/blocked-terms`,
    { value, matchMode },
    chatOverlayBlockedTermSchema,
  );
}

export async function removeChatOverlayBlockedTerm(overlayId: string, termId: string): Promise<void> {
  return apiDelete(`/api/chat-overlays/${overlayId}/blocked-terms/${termId}`);
}

export async function fetchChatOverlayActivityTypes(
  overlayId: string,
  signal?: AbortSignal,
): Promise<string[]> {
  const response = await apiGet(
    `/api/chat-overlays/${overlayId}/activity-types`,
    chatOverlayActivityTypesResponseSchema,
    { signal },
  );
  return response.activityTypes;
}

export async function setChatOverlayActivityTypes(
  overlayId: string,
  activityTypes: string[],
): Promise<string[]> {
  const response = await apiPut(
    `/api/chat-overlays/${overlayId}/activity-types`,
    { activityTypes },
    chatOverlayActivityTypesResponseSchema,
  );
  return response.activityTypes;
}

// --- public API ----------------------------------------------------------

export async function fetchPublicChatOverlayConfig(
  publicSlug: string,
  signal?: AbortSignal,
): Promise<PublicChatOverlayConfig> {
  return apiGet(`/api/public/chat-overlays/${publicSlug}/config`, publicChatOverlayConfigSchema, {
    signal,
  });
}

export async function fetchPublicChatOverlayItems(
  publicSlug: string,
  signal?: AbortSignal,
): Promise<PublicChatOverlayItem[]> {
  const response = await apiGet(
    `/api/public/chat-overlays/${publicSlug}/items`,
    publicChatOverlayItemsResponseSchema,
    { signal },
  );
  return response.items;
}
