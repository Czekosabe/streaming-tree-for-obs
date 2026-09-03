import { useEffect, useState } from 'react';
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
 * Which destination's metadata tab is open, kept valid as the platform list
 * changes - shared by `DashboardPage` and `MetadataPage` so both pick the
 * same destination on load and never drift into two competing selection
 * behaviours (Stage 20E "complete Platforms/Metadata" work).
 *
 * Picks the first platform once the list loads, and moves off a platform
 * that no longer exists (e.g. just deleted) onto the new first one. An
 * `initialId` (e.g. a specific destination the operator arrived from - see
 * `PlatformsPage`'s "Edit metadata" action) is honoured on the first render
 * and falls back the same way if that id turns out not to exist.
 *
 * Deliberately does nothing while `platforms` is empty, rather than
 * resetting `activeId` to `null`: an empty array is indistinguishable here
 * from "the query has not resolved yet", and a real `initialId` must
 * survive that brief window instead of being clobbered before the real
 * list arrives. Once the list is non-empty, a stale/deleted/never-real id
 * (including the default `null`) is replaced by the first platform, same
 * as before.
 */
export function useActiveMetadataSelection(
  platforms: readonly ConfiguredPlatform[],
  initialId: string | null = null,
): { activeId: string | null; setActiveId: (id: string | null) => void } {
  const [activeId, setActiveId] = useState<string | null>(initialId);

  useEffect(() => {
    if (platforms.length === 0) return;
    const stillExists = platforms.some((platform) => platform.id === activeId);
    if (!stillExists) {
      setActiveId(platforms[0]?.id ?? null);
    }
  }, [platforms, activeId]);

  return { activeId, setActiveId };
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
