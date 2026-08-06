import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  addChatOverlayBlockedTerm,
  addChatOverlayHiddenUser,
  createChatOverlay,
  deleteChatOverlay,
  fetchChatOverlay,
  fetchChatOverlayAccounts,
  fetchChatOverlayActivityTypes,
  fetchChatOverlayBlockedTerms,
  fetchChatOverlayHiddenUsers,
  fetchChatOverlays,
  fetchPublicChatOverlayConfig,
  removeChatOverlayBlockedTerm,
  removeChatOverlayHiddenUser,
  replaceChatOverlay,
  rotateChatOverlayPublicSlug,
  setChatOverlayAccounts,
  setChatOverlayActivityTypes,
} from '@/api/chat-overlay';
import type {
  AddChatOverlayHiddenUserInput,
  ChatOverlayBlockedTerm,
  ChatOverlayEditableFields,
  ChatOverlayHiddenUser,
  ChatOverlayMatchMode,
  ChatOverlayProfile,
  PublicChatOverlayConfig,
} from '@/api/chat-overlay-schemas';

/** Query keys. None carries chat content or a secret. */
export const chatOverlayKeys = {
  list: ['chat-overlays'] as const,
  detail: (id: string) => ['chat-overlays', id] as const,
  accounts: (id: string) => ['chat-overlays', id, 'accounts'] as const,
  hiddenUsers: (id: string) => ['chat-overlays', id, 'hidden-users'] as const,
  blockedTerms: (id: string) => ['chat-overlays', id, 'blocked-terms'] as const,
  activityTypes: (id: string) => ['chat-overlays', id, 'activity-types'] as const,
};

export function useChatOverlaysQuery(): UseQueryResult<ChatOverlayProfile[], Error> {
  return useQuery({
    queryKey: chatOverlayKeys.list,
    queryFn: ({ signal }) => fetchChatOverlays(signal),
  });
}

export function useChatOverlayQuery(id: string | undefined): UseQueryResult<ChatOverlayProfile, Error> {
  return useQuery({
    queryKey: chatOverlayKeys.detail(id ?? ''),
    queryFn: ({ signal }) => fetchChatOverlay(id!, signal),
    enabled: id !== undefined,
  });
}

export function useCreateChatOverlayMutation(): UseMutationResult<ChatOverlayProfile, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => createChatOverlay(name),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.list });
    },
  });
}

export function useReplaceChatOverlayMutation(
  id: string,
): UseMutationResult<ChatOverlayProfile, Error, ChatOverlayEditableFields> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ChatOverlayEditableFields) => replaceChatOverlay(id, input),
    onSuccess: (data) => {
      queryClient.setQueryData(chatOverlayKeys.detail(id), data);
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.list });
    },
  });
}

export function useDeleteChatOverlayMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteChatOverlay(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.list });
    },
  });
}

export function useRotateChatOverlayPublicSlugMutation(): UseMutationResult<
  ChatOverlayProfile,
  Error,
  string
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rotateChatOverlayPublicSlug(id),
    onSuccess: (data) => {
      queryClient.setQueryData(chatOverlayKeys.detail(data.id), data);
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.list });
    },
  });
}

export function useChatOverlayAccountsQuery(id: string): UseQueryResult<string[], Error> {
  return useQuery({
    queryKey: chatOverlayKeys.accounts(id),
    queryFn: ({ signal }) => fetchChatOverlayAccounts(id, signal),
  });
}

export function useSetChatOverlayAccountsMutation(
  id: string,
): UseMutationResult<string[], Error, string[]> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (accountIds: string[]) => setChatOverlayAccounts(id, accountIds),
    onSuccess: (data) => {
      queryClient.setQueryData(chatOverlayKeys.accounts(id), data);
    },
  });
}

export function useChatOverlayHiddenUsersQuery(id: string): UseQueryResult<ChatOverlayHiddenUser[], Error> {
  return useQuery({
    queryKey: chatOverlayKeys.hiddenUsers(id),
    queryFn: ({ signal }) => fetchChatOverlayHiddenUsers(id, signal),
  });
}

export function useAddChatOverlayHiddenUserMutation(
  id: string,
): UseMutationResult<ChatOverlayHiddenUser, Error, AddChatOverlayHiddenUserInput> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AddChatOverlayHiddenUserInput) => addChatOverlayHiddenUser(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.hiddenUsers(id) });
    },
  });
}

export function useRemoveChatOverlayHiddenUserMutation(
  id: string,
): UseMutationResult<
  void,
  Error,
  { providerId: string; connectedAccountId: string; providerUserId: string }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (ref: { providerId: string; connectedAccountId: string; providerUserId: string }) =>
      removeChatOverlayHiddenUser(id, ref),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.hiddenUsers(id) });
    },
  });
}

export function useChatOverlayBlockedTermsQuery(
  id: string,
): UseQueryResult<ChatOverlayBlockedTerm[], Error> {
  return useQuery({
    queryKey: chatOverlayKeys.blockedTerms(id),
    queryFn: ({ signal }) => fetchChatOverlayBlockedTerms(id, signal),
  });
}

export function useAddChatOverlayBlockedTermMutation(
  id: string,
): UseMutationResult<ChatOverlayBlockedTerm, Error, { value: string; matchMode: ChatOverlayMatchMode }> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ value, matchMode }: { value: string; matchMode: ChatOverlayMatchMode }) =>
      addChatOverlayBlockedTerm(id, value, matchMode),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.blockedTerms(id) });
    },
  });
}

export function useRemoveChatOverlayBlockedTermMutation(
  id: string,
): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (termId: string) => removeChatOverlayBlockedTerm(id, termId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: chatOverlayKeys.blockedTerms(id) });
    },
  });
}

export function useChatOverlayActivityTypesQuery(id: string): UseQueryResult<string[], Error> {
  return useQuery({
    queryKey: chatOverlayKeys.activityTypes(id),
    queryFn: ({ signal }) => fetchChatOverlayActivityTypes(id, signal),
  });
}

/** Public, unauthenticated config for the Browser Source route itself
 * (see pages/OverlayChatPage.tsx) - a distinct query key namespace from
 * the management queries above, since it is fetched by public slug, not
 * management id. */
export function usePublicChatOverlayConfigQuery(
  publicSlug: string | undefined,
): UseQueryResult<PublicChatOverlayConfig, Error> {
  return useQuery({
    queryKey: ['public-chat-overlay-config', publicSlug ?? ''],
    queryFn: ({ signal }) => fetchPublicChatOverlayConfig(publicSlug!, signal),
    enabled: publicSlug !== undefined && publicSlug !== '',
    retry: false,
  });
}

export function useSetChatOverlayActivityTypesMutation(
  id: string,
): UseMutationResult<string[], Error, string[]> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (activityTypes: string[]) => setChatOverlayActivityTypes(id, activityTypes),
    onSuccess: (data) => {
      queryClient.setQueryData(chatOverlayKeys.activityTypes(id), data);
    },
  });
}
