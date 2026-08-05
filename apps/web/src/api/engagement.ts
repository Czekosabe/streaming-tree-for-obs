import { deviceFlowSnapshotSchema, type DeviceFlowSnapshot } from '@/api/account-schemas';
import { apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  accountEngagementSchema,
  connectorSchema,
  engagementEventsResponseSchema,
  engagementStatusSchema,
  type AccountEngagement,
  type Connector,
  type EngagementEventsResponse,
  type EngagementStatus,
  type SetEngagementInput,
} from './engagement-schemas';

/**
 * Transport for the Stage 8A Engagement Event Bus and Twitch
 * connector-management API. No caching or React concerns live here - see
 * hooks/use-engagement.ts. Live event delivery is a separate concern too -
 * see hooks/use-engagement-stream.ts for the Server-Sent Events client.
 */

const NO_BODY = undefined;

export async function fetchEngagementStatus(signal?: AbortSignal): Promise<EngagementStatus> {
  return apiGet('/api/engagement/status', engagementStatusSchema, { signal });
}

export async function fetchEngagementEvents(
  options: { after?: number; limit?: number; signal?: AbortSignal } = {},
): Promise<EngagementEventsResponse> {
  const params = new URLSearchParams();
  if (options.after !== undefined) params.set('after', String(options.after));
  if (options.limit !== undefined) params.set('limit', String(options.limit));
  const query = params.toString();
  const path = query ? `/api/engagement/events?${query}` : '/api/engagement/events';
  return apiGet(path, engagementEventsResponseSchema, { signal: options.signal });
}

export async function fetchAccountEngagement(
  accountId: string,
  signal?: AbortSignal,
): Promise<AccountEngagement> {
  return apiGet(`/api/connected-accounts/${accountId}/engagement`, accountEngagementSchema, {
    signal,
  });
}

export async function setAccountEngagement(
  accountId: string,
  input: SetEngagementInput,
): Promise<AccountEngagement> {
  return apiPut(`/api/connected-accounts/${accountId}/engagement`, input, accountEngagementSchema);
}

/** Starts the identity-bound, union-scoped Twitch permission-upgrade flow. */
export async function authorizeEngagement(accountId: string): Promise<DeviceFlowSnapshot> {
  return apiPost(
    `/api/connected-accounts/${accountId}/engagement/authorize`,
    NO_BODY,
    deviceFlowSnapshotSchema,
  );
}

export async function restartEngagementConnector(accountId: string): Promise<Connector> {
  return apiPost(`/api/connected-accounts/${accountId}/engagement/restart`, NO_BODY, connectorSchema);
}
