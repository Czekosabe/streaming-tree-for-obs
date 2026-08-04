import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  cancelDeviceFlow,
  disconnectAccount,
  deletePlatformAccountLink,
  fetchAccount,
  fetchAccounts,
  fetchDeviceFlow,
  fetchIntegrationConfig,
  fetchPlatformAccountLink,
  fetchPublishPreview,
  publishMetadata,
  reconnectAccount,
  searchTwitchCategories,
  setIntegrationConfig,
  setPlatformAccountLink,
  startDeviceFlow,
  validateAccount,
} from '@/api/accounts';
import type {
  CategoryItem,
  ConnectedAccount,
  DeviceFlowSnapshot,
  IntegrationConfig,
  PlatformAccountLink,
  PublishPreview,
  PublishResult,
  SetIntegrationConfigInput,
} from '@/api/account-schemas';

/**
 * Query keys.
 *
 * None carries a secret: an account id, a platform id and a search query are
 * all non-secret identifiers, matching this project's existing rule that no
 * secret ever appears in a TanStack Query key or cache (see hooks/use-
 * credentials.ts for the precedent this follows).
 */
export const accountKeys = {
  integrationConfig: ['twitch-integration-config'] as const,
  deviceFlow: (attemptId: string) => ['device-flow', attemptId] as const,
  accounts: ['connected-accounts'] as const,
  account: (accountId: string) => ['connected-accounts', accountId] as const,
  categorySearch: (accountId: string, query: string) =>
    ['twitch-categories', accountId, query] as const,
  platformLink: (platformId: string) => ['platform-account-link', platformId] as const,
  publishPreview: (platformId: string) => ['metadata-publish-preview', platformId] as const,
};

const TERMINAL_DEVICE_FLOW_STATES: readonly DeviceFlowSnapshot['state'][] = [
  'authorized',
  'denied',
  'expired',
  'cancelled',
  'error',
];

export function useIntegrationConfigQuery(): UseQueryResult<IntegrationConfig, Error> {
  return useQuery({
    queryKey: accountKeys.integrationConfig,
    queryFn: ({ signal }) => fetchIntegrationConfig(signal),
  });
}

export function useSetIntegrationConfigMutation(): UseMutationResult<
  IntegrationConfig,
  Error,
  SetIntegrationConfigInput
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: setIntegrationConfig,
    onSuccess: (data) => {
      queryClient.setQueryData(accountKeys.integrationConfig, data);
    },
  });
}

export function useStartDeviceFlowMutation(): UseMutationResult<DeviceFlowSnapshot, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: startDeviceFlow,
    onSuccess: (data) => {
      queryClient.setQueryData(accountKeys.deviceFlow(data.attemptId), data);
    },
  });
}

export function useReconnectAccountMutation(): UseMutationResult<
  DeviceFlowSnapshot,
  Error,
  string
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: reconnectAccount,
    onSuccess: (data) => {
      queryClient.setQueryData(accountKeys.deviceFlow(data.attemptId), data);
    },
  });
}

/**
 * Polls one device-flow attempt at a fixed, fast interval while it is
 * active. This is the backend's own attempt, not Twitch: the interval here
 * is independent of the `intervalSeconds` Twitch asked for, since that
 * governs how often *this backend* may poll Twitch, not how often the
 * browser may poll this backend's own in-memory snapshot.
 */
export function useDeviceFlowQuery(
  attemptId: string | null,
): UseQueryResult<DeviceFlowSnapshot, Error> {
  return useQuery({
    queryKey: accountKeys.deviceFlow(attemptId ?? ''),
    queryFn: ({ signal }) => fetchDeviceFlow(attemptId ?? '', signal),
    enabled: attemptId !== null,
    refetchInterval: (query) => {
      const state = query.state.data?.state;
      if (state !== undefined && TERMINAL_DEVICE_FLOW_STATES.includes(state)) return false;
      return 1_000;
    },
    refetchIntervalInBackground: false,
    staleTime: 0,
  });
}

export function useCancelDeviceFlowMutation(): UseMutationResult<
  DeviceFlowSnapshot,
  Error,
  string
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: cancelDeviceFlow,
    onSuccess: (data) => {
      queryClient.setQueryData(accountKeys.deviceFlow(data.attemptId), data);
    },
  });
}

export function useAccountsQuery(): UseQueryResult<ConnectedAccount[], Error> {
  return useQuery({
    queryKey: accountKeys.accounts,
    queryFn: ({ signal }) => fetchAccounts(signal),
  });
}

export function useAccountQuery(accountId: string): UseQueryResult<ConnectedAccount, Error> {
  return useQuery({
    queryKey: accountKeys.account(accountId),
    queryFn: ({ signal }) => fetchAccount(accountId, signal),
  });
}

function useAccountsCommand<TData, TVariables>(
  mutationFn: (variables: TVariables) => Promise<TData>,
): UseMutationResult<TData, Error, TVariables> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: accountKeys.accounts });
    },
  });
}

export function useValidateAccountMutation(): UseMutationResult<
  ConnectedAccount,
  Error,
  string
> {
  return useAccountsCommand(validateAccount);
}

export function useDisconnectAccountMutation(): UseMutationResult<void, Error, string> {
  return useAccountsCommand(disconnectAccount);
}

export function usePlatformAccountLinkQuery(
  platformId: string,
): UseQueryResult<PlatformAccountLink | null, Error> {
  return useQuery({
    queryKey: accountKeys.platformLink(platformId),
    queryFn: ({ signal }) => fetchPlatformAccountLink(platformId, signal),
  });
}

export function useSetPlatformAccountLinkMutation(): UseMutationResult<
  PlatformAccountLink,
  Error,
  { platformId: string; accountId: string }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ platformId, accountId }) => setPlatformAccountLink(platformId, accountId),
    onSuccess: (data, variables) => {
      queryClient.setQueryData(accountKeys.platformLink(variables.platformId), data);
      void queryClient.invalidateQueries({ queryKey: accountKeys.publishPreview(variables.platformId) });
    },
  });
}

export function useDeletePlatformAccountLinkMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deletePlatformAccountLink,
    onSuccess: (_data, platformId) => {
      queryClient.setQueryData(accountKeys.platformLink(platformId), null);
      void queryClient.invalidateQueries({ queryKey: accountKeys.publishPreview(platformId) });
    },
  });
}

/**
 * Category search: enabled only with a real account id and a query long
 * enough to be meaningful, matching the backend's own 2-character minimum -
 * so a one-character keystroke never fires a request.
 */
export function useCategorySearchQuery(
  accountId: string | null,
  query: string,
): UseQueryResult<CategoryItem[], Error> {
  const trimmed = query.trim();
  return useQuery({
    queryKey: accountKeys.categorySearch(accountId ?? '', trimmed),
    queryFn: ({ signal }) => searchTwitchCategories(accountId ?? '', trimmed, signal),
    enabled: accountId !== null && trimmed.length >= 2,
    staleTime: 30_000,
  });
}

export function usePublishPreviewQuery(
  platformId: string,
  enabled: boolean,
): UseQueryResult<PublishPreview, Error> {
  return useQuery({
    queryKey: accountKeys.publishPreview(platformId),
    queryFn: ({ signal }) => fetchPublishPreview(platformId, signal),
    enabled,
    staleTime: 0,
  });
}

export function usePublishMetadataMutation(): UseMutationResult<PublishResult, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: publishMetadata,
    onSuccess: (_data, platformId) => {
      void queryClient.invalidateQueries({ queryKey: accountKeys.publishPreview(platformId) });
    },
  });
}
