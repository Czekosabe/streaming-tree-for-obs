import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import type { DeviceFlowSnapshot } from '@/api/account-schemas';
import {
  authorizeOutboundChat,
  fetchOutboundChatStatus,
  sendOutboundChatMessage,
} from '@/api/outbound-chat';
import type {
  OutboundChatStatus,
  SendOutboundChatMessageInput,
  SendOutboundChatMessageResponse,
} from '@/api/outbound-chat-schemas';

/**
 * Query keys. An account id is a non-secret identifier, matching this
 * project's existing rule (see hooks/use-engagement.ts's own precedent).
 */
export const outboundChatKeys = {
  status: (accountId: string) => ['outbound-chat-status', accountId] as const,
};

/** How often the composer polls status while mounted - matches
 * use-engagement.ts's own STATUS_POLL_INTERVAL_MS, so a permission upgrade
 * completed in another tab, or a rate-limit window elapsing, is picked up
 * without a manual refresh. */
const STATUS_POLL_INTERVAL_MS = 5_000;

export function useOutboundChatStatusQuery(
  accountId: string | undefined,
): UseQueryResult<OutboundChatStatus, Error> {
  return useQuery({
    queryKey: outboundChatKeys.status(accountId ?? ''),
    queryFn: ({ signal }) => fetchOutboundChatStatus(accountId ?? '', signal),
    enabled: accountId !== undefined,
    refetchInterval: STATUS_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useAuthorizeOutboundChatMutation(): UseMutationResult<
  DeviceFlowSnapshot,
  Error,
  string
> {
  return useMutation({
    mutationFn: (accountId: string) => authorizeOutboundChat(accountId),
  });
}

export function useSendOutboundChatMessageMutation(): UseMutationResult<
  SendOutboundChatMessageResponse,
  Error,
  { accountId: string; input: SendOutboundChatMessageInput }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ accountId, input }) => sendOutboundChatMessage(accountId, input),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: outboundChatKeys.status(variables.accountId) });
    },
  });
}
