import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  broadcastListResponseSchema,
  categorySearchResponseSchema,
  connectedAccountSchema,
  connectedAccountsResponseSchema,
  deviceFlowSnapshotSchema,
  integrationConfigSchema,
  oauthAttemptSnapshotSchema,
  platformAccountLinkResponseSchema,
  platformAccountLinkSchema,
  publishPreviewSchema,
  publishResultSchema,
  regionResponseSchema,
  remoteTargetResponseSchema,
  remoteTargetSchema,
  type BroadcastItem,
  type CategoryItem,
  type ConnectedAccount,
  type DeviceFlowSnapshot,
  type IntegrationConfig,
  type OAuthAttemptSnapshot,
  type PlatformAccountLink,
  type PublishPreview,
  type PublishResult,
  type RemoteTarget,
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

/**
 * Transport for the YouTube integration-config, OAuth-attempt, broadcast,
 * category, region and remote-target APIs. Reconnecting a YouTube account
 * reuses the shared `/reconnect` endpoint but parses an OAuthAttemptSnapshot
 * rather than a DeviceFlowSnapshot - the caller already knows the account's
 * provider before choosing which of these two functions to call.
 */

export async function fetchYouTubeIntegrationConfig(signal?: AbortSignal): Promise<IntegrationConfig> {
  return apiGet('/api/integrations/youtube/config', integrationConfigSchema, { signal });
}

export async function setYouTubeIntegrationConfig(
  input: SetIntegrationConfigInput,
): Promise<IntegrationConfig> {
  return apiPut('/api/integrations/youtube/config', input, integrationConfigSchema);
}

export async function startYouTubeOAuthAttempt(): Promise<OAuthAttemptSnapshot> {
  return apiPost('/api/integrations/youtube/oauth-attempts', NO_BODY, oauthAttemptSnapshotSchema);
}

export async function reconnectYouTubeAccount(accountId: string): Promise<OAuthAttemptSnapshot> {
  return apiPost(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/reconnect`,
    NO_BODY,
    oauthAttemptSnapshotSchema,
  );
}

export async function fetchYouTubeOAuthAttempt(
  attemptId: string,
  signal?: AbortSignal,
): Promise<OAuthAttemptSnapshot> {
  return apiGet(
    `/api/integrations/youtube/oauth-attempts/${encodeURIComponent(attemptId)}`,
    oauthAttemptSnapshotSchema,
    { signal },
  );
}

export async function cancelYouTubeOAuthAttempt(attemptId: string): Promise<OAuthAttemptSnapshot> {
  await apiDelete(`/api/integrations/youtube/oauth-attempts/${encodeURIComponent(attemptId)}`);
  return fetchYouTubeOAuthAttempt(attemptId);
}

export async function selectYouTubeChannel(
  attemptId: string,
  channelId: string,
): Promise<OAuthAttemptSnapshot> {
  return apiPost(
    `/api/integrations/youtube/oauth-attempts/${encodeURIComponent(attemptId)}/channel`,
    { channelId },
    oauthAttemptSnapshotSchema,
  );
}

export async function fetchYouTubeBroadcasts(
  accountId: string,
  signal?: AbortSignal,
): Promise<BroadcastItem[]> {
  const response = await apiGet(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/youtube/broadcasts`,
    broadcastListResponseSchema,
    { signal },
  );
  return response.items;
}

export async function fetchYouTubeCategories(
  accountId: string,
  signal?: AbortSignal,
): Promise<CategoryItem[]> {
  const response = await apiGet(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/youtube/categories`,
    categorySearchResponseSchema,
    { signal },
  );
  return response.items;
}

export async function fetchYouTubeRegion(accountId: string, signal?: AbortSignal): Promise<string> {
  const response = await apiGet(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/youtube/region`,
    regionResponseSchema,
    { signal },
  );
  return response.region;
}

export async function setYouTubeRegion(accountId: string, region: string): Promise<string> {
  const response = await apiPut(
    `/api/connected-accounts/${encodeURIComponent(accountId)}/youtube/region`,
    { region },
    regionResponseSchema,
  );
  return response.region;
}

export async function fetchRemoteTarget(
  platformId: string,
  signal?: AbortSignal,
): Promise<RemoteTarget | null> {
  return apiGet(
    `/api/platforms/${encodeURIComponent(platformId)}/remote-target`,
    remoteTargetResponseSchema,
    { signal },
  );
}

export async function setRemoteTarget(platformId: string, resourceId: string): Promise<RemoteTarget> {
  return apiPut(
    `/api/platforms/${encodeURIComponent(platformId)}/remote-target`,
    { resourceId },
    remoteTargetSchema,
  );
}

export async function deleteRemoteTarget(platformId: string): Promise<void> {
  await apiDelete(`/api/platforms/${encodeURIComponent(platformId)}/remote-target`);
}
