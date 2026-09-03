import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import type { DeviceFlowSnapshot } from '@/api/account-schemas';
import {
  authorizeEngagement,
  fetchAccountEngagement,
  fetchEngagementStatus,
  restartEngagementConnector,
  setAccountEngagement,
} from '@/api/engagement';
import type {
  AccountEngagement,
  Connector,
  EngagementStatus,
  SetEngagementInput,
} from '@/api/engagement-schemas';

/**
 * Query keys.
 *
 * None carries a secret: an account id is a non-secret identifier, matching
 * this project's existing rule (see hooks/use-accounts.ts's own precedent).
 */
export const engagementKeys = {
  status: ['engagement-status'] as const,
  account: (accountId: string) => ['engagement-account', accountId] as const,
};

/** How often the diagnostic view polls bus/connector status while mounted. */
const STATUS_POLL_INTERVAL_MS = 5_000;

export function useEngagementStatusQuery(): UseQueryResult<EngagementStatus, Error> {
  return useQuery({
    queryKey: engagementKeys.status,
    queryFn: ({ signal }) => fetchEngagementStatus(signal),
    refetchInterval: STATUS_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useAccountEngagementQuery(
  accountId: string,
): UseQueryResult<AccountEngagement, Error> {
  return useQuery({
    queryKey: engagementKeys.account(accountId),
    queryFn: ({ signal }) => fetchAccountEngagement(accountId, signal),
    // A caller with no account currently selected (e.g. OutboundChatComposer
    // before an account is picked) passes an empty string rather than
    // omitting the hook call entirely - without this guard that fired a
    // real request against the malformed `/api/connected-accounts//
    // engagement` (a missing id segment), a real browser-discovered defect.
    enabled: accountId !== '',
    refetchInterval: STATUS_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useSetEngagementMutation(): UseMutationResult<
  AccountEngagement,
  Error,
  { accountId: string; input: SetEngagementInput }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ accountId, input }) => setAccountEngagement(accountId, input),
    onSuccess: (data, variables) => {
      queryClient.setQueryData(engagementKeys.account(variables.accountId), data);
      void queryClient.invalidateQueries({ queryKey: engagementKeys.status });
    },
  });
}

export function useAuthorizeEngagementMutation(): UseMutationResult<
  DeviceFlowSnapshot,
  Error,
  string
> {
  return useMutation({
    mutationFn: (accountId: string) => authorizeEngagement(accountId),
  });
}

export function useRestartEngagementMutation(): UseMutationResult<Connector, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (accountId: string) => restartEngagementConnector(accountId),
    onSuccess: (_data, accountId) => {
      void queryClient.invalidateQueries({ queryKey: engagementKeys.account(accountId) });
      void queryClient.invalidateQueries({ queryKey: engagementKeys.status });
    },
  });
}
