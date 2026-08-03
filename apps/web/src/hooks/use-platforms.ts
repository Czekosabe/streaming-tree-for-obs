import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  createPlatform,
  deletePlatform,
  fetchPlatforms,
  fetchProviderDefinitions,
  savePlatformMetadata,
  updatePlatform,
} from '@/api/platforms';
import type {
  ConfiguredPlatform,
  CreatePlatformInput,
  PlatformMetadata,
  ProviderDefinition,
  SaveMetadataInput,
  UpdatePlatformInput,
} from '@/api/platform-schemas';
import { ApiError } from '@/lib/api-client';

import { removePlatform, replaceMetadata, replacePlatform } from './platform-cache';

/**
 * Query keys.
 *
 * Kept in one object so an invalidation can never target a key that does not
 * exist because of a typo.
 */
export const platformKeys = {
  definitions: ['platform-definitions'] as const,
  platforms: ['platforms'] as const,
};

/**
 * Provider definitions change only when the backend binary changes, so they are
 * cached aggressively.
 */
export function usePlatformDefinitionsQuery(): UseQueryResult<ProviderDefinition[], Error> {
  return useQuery({
    queryKey: platformKeys.definitions,
    queryFn: ({ signal }) => fetchProviderDefinitions(signal),
    staleTime: 60 * 60 * 1000,
  });
}

/** Configured platforms, including their metadata and ordered tags. */
export function usePlatformsQuery(): UseQueryResult<ConfiguredPlatform[], Error> {
  return useQuery({
    queryKey: platformKeys.platforms,
    queryFn: ({ signal }) => fetchPlatforms(signal),
  });
}

export function useCreatePlatformMutation(): UseMutationResult<
  ConfiguredPlatform,
  Error,
  CreatePlatformInput
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: CreatePlatformInput) => createPlatform(input),
    onSuccess: () => {
      // The list is refetched rather than patched: the backend decides sort
      // order and timestamps, so its ordering is the one to trust.
      void queryClient.invalidateQueries({ queryKey: platformKeys.platforms });
    },
  });
}

export function useUpdatePlatformMutation(): UseMutationResult<
  ConfiguredPlatform,
  Error,
  { id: string; input: UpdatePlatformInput }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdatePlatformInput }) =>
      updatePlatform(id, input),
    onSuccess: (updated) => {
      // Patch the cached row first so the card updates without a flash, then
      // refetch because sortOrder may have reordered the whole list.
      queryClient.setQueryData<ConfiguredPlatform[]>(platformKeys.platforms, (current) =>
        replacePlatform(current, updated),
      );
      void queryClient.invalidateQueries({ queryKey: platformKeys.platforms });
    },
  });
}

export function useDeletePlatformMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => deletePlatform(id),
    onSuccess: (_result, id) => {
      queryClient.setQueryData<ConfiguredPlatform[]>(platformKeys.platforms, (current) =>
        removePlatform(current, id),
      );
      void queryClient.invalidateQueries({ queryKey: platformKeys.platforms });
    },
  });
}

export function useUpdatePlatformMetadataMutation(): UseMutationResult<
  PlatformMetadata,
  Error,
  { id: string; input: SaveMetadataInput }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: SaveMetadataInput }) =>
      savePlatformMetadata(id, input),
    onSuccess: (metadata, variables) => {
      // Only the metadata is patched in: the configuration fields and the list
      // order are unaffected by a metadata save, so no refetch is needed.
      queryClient.setQueryData<ConfiguredPlatform[]>(platformKeys.platforms, (current) =>
        replaceMetadata(current, variables.id, metadata),
      );
    },
  });
}

/**
 * Extracts per-field errors from a failed mutation.
 *
 * Returns an empty object for any failure that is not a validation rejection,
 * so callers can render field errors and a general message independently.
 */
export function fieldErrorsOf(error: unknown): Record<string, string> {
  return error instanceof ApiError && error.isValidation ? error.fields : {};
}
