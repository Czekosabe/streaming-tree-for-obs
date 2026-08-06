import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  addOperatorChatBotUser,
  addOperatorChatHiddenUser,
  fetchOperatorChatAccountVisibility,
  fetchOperatorChatBotUsers,
  fetchOperatorChatHiddenUsers,
  fetchOperatorChatPreferences,
  fetchOperatorChatStatus,
  removeOperatorChatBotUser,
  removeOperatorChatHiddenUser,
  setOperatorChatAccountVisibility,
  setOperatorChatPreferences,
} from '@/api/operator-chat';
import type {
  AddOperatorChatUserRefInput,
  OperatorChatAccountVisibility,
  OperatorChatPreferences,
  OperatorChatStatus,
  OperatorChatUserRef,
} from '@/api/operator-chat-schemas';

/**
 * Query keys. None carries chat content or a secret - an account id and a
 * hidden/bot-user entry id are non-secret identifiers, matching this
 * project's existing rule (see hooks/use-engagement.ts's own precedent).
 */
export const operatorChatKeys = {
  status: ['operator-chat-status'] as const,
  preferences: ['operator-chat-preferences'] as const,
  accountVisibility: ['operator-chat-account-visibility'] as const,
  hiddenUsers: ['operator-chat-hidden-users'] as const,
  botUsers: ['operator-chat-bot-users'] as const,
};

/** How often the header polls projection status while mounted. */
const STATUS_POLL_INTERVAL_MS = 10_000;

export function useOperatorChatStatusQuery(): UseQueryResult<OperatorChatStatus, Error> {
  return useQuery({
    queryKey: operatorChatKeys.status,
    queryFn: ({ signal }) => fetchOperatorChatStatus(signal),
    refetchInterval: STATUS_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useOperatorChatPreferencesQuery(): UseQueryResult<OperatorChatPreferences, Error> {
  return useQuery({
    queryKey: operatorChatKeys.preferences,
    queryFn: ({ signal }) => fetchOperatorChatPreferences(signal),
  });
}

export function useSetOperatorChatPreferencesMutation(): UseMutationResult<
  OperatorChatPreferences,
  Error,
  OperatorChatPreferences
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: OperatorChatPreferences) => setOperatorChatPreferences(input),
    onSuccess: (data) => {
      queryClient.setQueryData(operatorChatKeys.preferences, data);
    },
  });
}

export function useOperatorChatAccountVisibilityQuery(): UseQueryResult<
  OperatorChatAccountVisibility[],
  Error
> {
  return useQuery({
    queryKey: operatorChatKeys.accountVisibility,
    queryFn: ({ signal }) => fetchOperatorChatAccountVisibility(signal),
  });
}

export function useSetOperatorChatAccountVisibilityMutation(): UseMutationResult<
  OperatorChatAccountVisibility,
  Error,
  { accountId: string; visible: boolean }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ accountId, visible }) => setOperatorChatAccountVisibility(accountId, visible),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: operatorChatKeys.accountVisibility });
    },
  });
}

export function useOperatorChatHiddenUsersQuery(): UseQueryResult<OperatorChatUserRef[], Error> {
  return useQuery({
    queryKey: operatorChatKeys.hiddenUsers,
    queryFn: ({ signal }) => fetchOperatorChatHiddenUsers(signal),
  });
}

export function useAddOperatorChatHiddenUserMutation(): UseMutationResult<
  OperatorChatUserRef,
  Error,
  AddOperatorChatUserRefInput
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AddOperatorChatUserRefInput) => addOperatorChatHiddenUser(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: operatorChatKeys.hiddenUsers });
    },
  });
}

export function useRemoveOperatorChatHiddenUserMutation(): UseMutationResult<
  void,
  Error,
  string
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => removeOperatorChatHiddenUser(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: operatorChatKeys.hiddenUsers });
    },
  });
}

export function useOperatorChatBotUsersQuery(): UseQueryResult<OperatorChatUserRef[], Error> {
  return useQuery({
    queryKey: operatorChatKeys.botUsers,
    queryFn: ({ signal }) => fetchOperatorChatBotUsers(signal),
  });
}

export function useAddOperatorChatBotUserMutation(): UseMutationResult<
  OperatorChatUserRef,
  Error,
  AddOperatorChatUserRefInput
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AddOperatorChatUserRefInput) => addOperatorChatBotUser(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: operatorChatKeys.botUsers });
    },
  });
}

export function useRemoveOperatorChatBotUserMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => removeOperatorChatBotUser(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: operatorChatKeys.botUsers });
    },
  });
}
