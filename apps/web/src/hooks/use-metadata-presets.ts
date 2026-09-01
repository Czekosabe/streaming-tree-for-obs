import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  createMetadataPreset,
  deleteMetadataPreset,
  fetchMetadataPresets,
  updateMetadataPreset,
} from '@/api/metadata-presets';
import type { MetadataPreset, SavePresetInput } from '@/api/metadata-preset-schemas';

export const metadataPresetKeys = {
  presets: ['metadata-presets'] as const,
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
