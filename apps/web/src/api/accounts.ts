import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  categorySearchResponseSchema,
  connectedAccountSchema,
  connectedAccountsResponseSchema,
  deviceFlowSnapshotSchema,
  integrationConfigSchema,
  platformAccountLinkResponseSchema,
  platformAccountLinkSchema,
  publishPreviewSchema,
  publishResultSchema,
  type CategoryItem,
  type ConnectedAccount,
  type DeviceFlowSnapshot,
  type IntegrationConfig,
  type PlatformAccountLink,
  type PublishPreview,
  type PublishResult,
  type SetIntegrationConfigInput,
} from './account-schemas';

/**
 * Transport for the Twitch integration-config, device-flow,
 * connected-account, account-link, category-search and metadata-publish
 * APIs. No caching or React concerns live here - see hooks/use-accounts.ts.
 */

const NO_BODY = undefined;

export async function fetchIntegrationConfig(signal?: AbortSignal): Promise<IntegrationConfig> {
  return apiGet('/api/integrations/twitch/config', integrationConfigSchema, { signal });
}

export async function setIntegrationConfig(
  input: SetIntegrationConfigInput,
): Promise<IntegrationConfig> {
  return apiPut('/api/integrations/twitch/config', input, integrationConfigSchema);
}

export async function startDeviceFlow(): Promise<DeviceFlowSnapshot> {
  return apiPost('/api/integrations/twitch/device-flow', NO_BODY, deviceFlowSnapshotSchema);
}

export async function fetchDeviceFlow(
  attemptId: string,
  signal?: AbortSignal,
): Promise<DeviceFlowSnapshot> {
  return apiGet(
    `/api/integrations/twitch/device-flow/${encodeURIComponent(attemptId)}`,
    deviceFlowSnapshotSchema,
    { signal },
  );
}

export async function cancelDeviceFlow(attemptId: string): Promise<DeviceFlowSnapshot> {
  await apiDelete(`/api/integrations/twitch/device-flow/${encodeURIComponent(attemptId)}`);
  // DELETE here returns the final snapshot as its body in this API (unlike a
  // bare 204 delete), so re-fetch it for the caller rather than assuming a
  // shape - keeps this transport honest about what the endpoint returns.
  return fetchDeviceFlow(attemptId);
}

export async function fetchAccounts(signal?: AbortSignal): Promise<ConnectedAccount[]> {
  const response = await apiGet('/api/connected-accounts', connectedAccountsResponseSchema, {
    signal,
  });
  return response.accounts;
}

export async function fetchAccount(
  accountId: string,
  signal?: AbortSignal,
): Promise<ConnectedAccount> {
  return apiGet(
    `/api/connected-accounts/${encodeURIComponent(accountId)}`,
    connectedAccountSchema,
    { signal },
  );
}

export async function validateAccount(accountId: string): Promise<ConnectedAccount> {
  return apiPost(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/validate`,
    NO_BODY,
    connectedAccountSchema,
  );
}

export async function reconnectAccount(accountId: string): Promise<DeviceFlowSnapshot> {
  return apiPost(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/reconnect`,
    NO_BODY,
    deviceFlowSnapshotSchema,
  );
}

export async function disconnectAccount(accountId: string): Promise<void> {
  await apiDelete(`/api/connected-accounts/${encodeURIComponent(accountId)}`);
}

export async function searchTwitchCategories(
  accountId: string,
  query: string,
  signal?: AbortSignal,
): Promise<CategoryItem[]> {
  const response = await apiGet(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/twitch/categories?query=${encodeURIComponent(query)}`,
    categorySearchResponseSchema,
    { signal },
  );
  return response.items;
}

export async function fetchPlatformAccountLink(
  platformId: string,
  signal?: AbortSignal,
): Promise<PlatformAccountLink | null> {
  return apiGet(
    `/api/platforms/${encodeURIComponent(platformId)}/connected-account`,
    platformAccountLinkResponseSchema,
    { signal },
  );
}

export async function setPlatformAccountLink(
  platformId: string,
  accountId: string,
): Promise<PlatformAccountLink> {
  return apiPut(
    `/api/platforms/${encodeURIComponent(platformId)}/connected-account`,
    { accountId },
    platformAccountLinkSchema,
  );
}

export async function deletePlatformAccountLink(platformId: string): Promise<void> {
  await apiDelete(`/api/platforms/${encodeURIComponent(platformId)}/connected-account`);
}

export async function fetchPublishPreview(
  platformId: string,
  signal?: AbortSignal,
): Promise<PublishPreview> {
  return apiGet(
    `/api/platforms/${encodeURIComponent(platformId)}/metadata/publish-preview`,
    publishPreviewSchema,
    { signal },
  );
}

export async function publishMetadata(platformId: string): Promise<PublishResult> {
  return apiPost(
    `/api/platforms/${encodeURIComponent(platformId)}/metadata/publish`,
    NO_BODY,
    publishResultSchema,
  );
}
