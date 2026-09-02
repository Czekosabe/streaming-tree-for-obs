import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  applyStreamSetup,
  createStreamSetup,
  deleteStreamSetup,
  duplicateStreamSetup,
  fetchStreamSetupPreview,
  fetchStreamSetups,
  saveCurrentStreamSetup,
  updateStreamSetup,
} from '@/api/stream-setups';
import type {
  SaveStreamSetupInput,
  StreamSetupApplyResult,
  StreamSetupPreview,
  StreamSetupProfile,
} from '@/api/stream-setup-schemas';

import { platformKeys } from './use-platforms';

export const streamSetupKeys = {
  profiles: ['stream-setups'] as const,
  preview: (id: string) => ['stream-setups', id, 'preview'] as const,
};

export function useStreamSetupsQuery(): UseQueryResult<StreamSetupProfile[], Error> {
  return useQuery({
    queryKey: streamSetupKeys.profiles,
    queryFn: ({ signal }) => fetchStreamSetups(signal),
  });
}

export function useCreateStreamSetupMutation(): UseMutationResult<
  StreamSetupProfile,
  Error,
  SaveStreamSetupInput
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: SaveStreamSetupInput) => createStreamSetup(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: streamSetupKeys.profiles });
    },
  });
}

export function useUpdateStreamSetupMutation(): UseMutationResult<
  StreamSetupProfile,
  Error,
  { id: string; input: SaveStreamSetupInput }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: SaveStreamSetupInput }) =>
      updateStreamSetup(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: streamSetupKeys.profiles });
    },
  });
}

export function useDeleteStreamSetupMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteStreamSetup(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: streamSetupKeys.profiles });
    },
  });
}

export function useDuplicateStreamSetupMutation(): UseMutationResult<
  StreamSetupProfile,
  Error,
  { id: string; name: string }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => duplicateStreamSetup(id, name),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: streamSetupKeys.profiles });
    },
  });
}

export function useSaveCurrentStreamSetupMutation(): UseMutationResult<
  StreamSetupProfile,
  Error,
  { name: string; note: string; metadataPresetId: string | null }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, note, metadataPresetId }) => saveCurrentStreamSetup(name, note, metadataPresetId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: streamSetupKeys.profiles });
    },
  });
}

/** Read-only preview (docs/stream-setup-profiles.md §3) - never writes anything. */
export function useStreamSetupPreviewQuery(id: string | null): UseQueryResult<StreamSetupPreview, Error> {
  return useQuery({
    queryKey: streamSetupKeys.preview(id ?? ''),
    queryFn: ({ signal }) => fetchStreamSetupPreview(id ?? '', signal),
    enabled: id !== null,
  });
}

export function useApplyStreamSetupMutation(): UseMutationResult<StreamSetupApplyResult, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => applyStreamSetup(id),
    onSuccess: () => {
      // Destination enabled-state and possibly metadata both changed in
      // one atomic-ish apply - refetched rather than patched, since
      // several platforms may have landed at once (mirrors
      // useApplyMetadataPresetMutation's own reasoning).
      void queryClient.invalidateQueries({ queryKey: platformKeys.platforms });
    },
  });
}
