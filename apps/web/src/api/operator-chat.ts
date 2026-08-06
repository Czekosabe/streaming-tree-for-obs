import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  operatorChatAccountVisibilityResponseSchema,
  operatorChatAccountVisibilitySchema,
  operatorChatItemsResponseSchema,
  operatorChatPreferencesSchema,
  operatorChatStatusSchema,
  operatorChatUserRefSchema,
  operatorChatUserRefsResponseSchema,
  type AddOperatorChatUserRefInput,
  type OperatorChatAccountVisibility,
  type OperatorChatItemsResponse,
  type OperatorChatPreferences,
  type OperatorChatStatus,
  type OperatorChatUserRef,
} from './operator-chat-schemas';

/**
 * Transport for the Stage 9 unified-operator-chat projection API. No
 * caching or React concerns live here - see hooks/use-operator-chat.ts.
 * Live item delivery is a separate concern - see
 * hooks/use-operator-chat-stream.ts for the SSE client.
 */

export async function fetchOperatorChatStatus(signal?: AbortSignal): Promise<OperatorChatStatus> {
  return apiGet('/api/operator-chat/status', operatorChatStatusSchema, { signal });
}

export type FetchOperatorChatItemsOptions = {
  after?: number;
  limit?: number;
  accountIds?: string[];
  kinds?: string[];
  includeDeleted?: boolean;
  signal?: AbortSignal;
};

function buildItemsQuery(options: FetchOperatorChatItemsOptions): string {
  const params = new URLSearchParams();
  if (options.after !== undefined) params.set('after', String(options.after));
  if (options.limit !== undefined) params.set('limit', String(options.limit));
  for (const accountId of options.accountIds ?? []) params.append('accountId', accountId);
  if (options.kinds !== undefined && options.kinds.length > 0) {
    params.set('kinds', options.kinds.join(','));
  }
  if (options.includeDeleted !== undefined) {
    params.set('includeDeleted', String(options.includeDeleted));
  }
  return params.toString();
}

export async function fetchOperatorChatItems(
  options: FetchOperatorChatItemsOptions = {},
): Promise<OperatorChatItemsResponse> {
  const query = buildItemsQuery(options);
  const path = query ? `/api/operator-chat/items?${query}` : '/api/operator-chat/items';
  return apiGet(path, operatorChatItemsResponseSchema, { signal: options.signal });
}

export async function fetchOperatorChatPreferences(
  signal?: AbortSignal,
): Promise<OperatorChatPreferences> {
  return apiGet('/api/operator-chat/preferences', operatorChatPreferencesSchema, { signal });
}

export async function setOperatorChatPreferences(
  input: OperatorChatPreferences,
): Promise<OperatorChatPreferences> {
  return apiPut('/api/operator-chat/preferences', input, operatorChatPreferencesSchema);
}

export async function fetchOperatorChatAccountVisibility(
  signal?: AbortSignal,
): Promise<OperatorChatAccountVisibility[]> {
  const response = await apiGet(
    '/api/operator-chat/account-visibility',
    operatorChatAccountVisibilityResponseSchema,
    { signal },
  );
  return response.items;
}

export async function setOperatorChatAccountVisibility(
  accountId: string,
  visible: boolean,
): Promise<OperatorChatAccountVisibility> {
  return apiPut(
    `/api/operator-chat/account-visibility/${accountId}`,
    { visible },
    operatorChatAccountVisibilitySchema,
  );
}

export async function fetchOperatorChatHiddenUsers(
  signal?: AbortSignal,
): Promise<OperatorChatUserRef[]> {
  const response = await apiGet('/api/operator-chat/hidden-users', operatorChatUserRefsResponseSchema, {
    signal,
  });
  return response.items;
}

export async function addOperatorChatHiddenUser(
  input: AddOperatorChatUserRefInput,
): Promise<OperatorChatUserRef> {
  return apiPost('/api/operator-chat/hidden-users', input, operatorChatUserRefSchema);
}

export async function removeOperatorChatHiddenUser(id: string): Promise<void> {
  return apiDelete(`/api/operator-chat/hidden-users/${id}`);
}

export async function fetchOperatorChatBotUsers(
  signal?: AbortSignal,
): Promise<OperatorChatUserRef[]> {
  const response = await apiGet('/api/operator-chat/bot-users', operatorChatUserRefsResponseSchema, {
    signal,
  });
  return response.items;
}

export async function addOperatorChatBotUser(
  input: AddOperatorChatUserRefInput,
): Promise<OperatorChatUserRef> {
  return apiPost('/api/operator-chat/bot-users', input, operatorChatUserRefSchema);
}

export async function removeOperatorChatBotUser(id: string): Promise<void> {
  return apiDelete(`/api/operator-chat/bot-users/${id}`);
}
