import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import type { ApplyDestinationPreview, ApplyPresetResult } from '@/api/metadata-preset-apply-schemas';
import type { MetadataPreset, SavePresetInput } from '@/api/metadata-preset-schemas';
import {
  applyMetadataPreset,
  createMetadataPreset,
  deleteMetadataPreset,
  fetchApplyPreview,
  fetchMetadataPresets,
  updateMetadataPreset,
} from '@/api/metadata-presets';

import { platformKeys } from './use-platforms';

export const metadataPresetKeys = {
  presets: ['metadata-presets'] as const,
  applyPreview: (presetId: string, platformIds: readonly string[]) =>
    ['metadata-presets', presetId, 'apply-preview', ...[...platformIds].sort()] as const,
};

export function useMetadataPresetsQuery(): UseQueryResult<MetadataPreset[], Error> {
  return useQuery({
    queryKey: metadataPresetKeys.presets,
    queryFn: ({ signal }) => fetchMetadataPresets(signal),
  });
}

export function useCreateMetadataPresetMutation(): UseMutationResult<
  MetadataPreset,
  Error,
  SavePresetInput
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: SavePresetInput) => createMetadataPreset(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: metadataPresetKeys.presets });
    },
  });
}

export function useUpdateMetadataPresetMutation(): UseMutationResult<
  MetadataPreset,
  Error,
  { id: string; input: SavePresetInput }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: SavePresetInput }) => updateMetadataPreset(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: metadataPresetKeys.presets });
    },
  });
}

export function useDeleteMetadataPresetMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => deleteMetadataPreset(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: metadataPresetKeys.presets });
    },
  });
}

/**
 * Read-only compatibility preview for applying a preset to one or more
 * destinations (docs/metadata-presets.md §6) - never writes anything.
 * Disabled while no destination is selected, so switching from zero to
 * one selected destination is what first fires the request.
 *
 * Every selection change is a distinct query key (the platform id set
 * is part of it), so `placeholderData: keepPreviousData` keeps the
 * last selection's chips on screen while the new one loads instead of
 * flashing every already-checked destination back to "Checking..."
 * each time one more box is ticked.
 */
export function useApplyPreviewQuery(
  presetId: string,
  platformIds: readonly string[],
): UseQueryResult<ApplyDestinationPreview[], Error> {
  return useQuery({
    queryKey: metadataPresetKeys.applyPreview(presetId, platformIds),
    queryFn: ({ signal }) => fetchApplyPreview(presetId, [...platformIds], signal),
    enabled: platformIds.length > 0,
    placeholderData: keepPreviousData,
  });
}

export function useApplyMetadataPresetMutation(): UseMutationResult<
  ApplyPresetResult,
  Error,
  { presetId: string; platformIds: string[] }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ presetId, platformIds }: { presetId: string; platformIds: string[] }) =>
      applyMetadataPreset(presetId, platformIds),
    onSuccess: () => {
      // Every applied destination's metadata changed; the list is
      // refetched rather than patched field-by-field since several
      // platforms landed in one atomic write at once.
      void queryClient.invalidateQueries({ queryKey: platformKeys.platforms });
    },
  });
}
