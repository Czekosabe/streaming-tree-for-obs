import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import { deleteAudioAsset, fetchAudioAssets, uploadAudioAsset } from '@/api/audioasset';
import type { AudioAsset } from '@/api/audioasset-schemas';

/** One managed audio-asset library, shared by every alert rule editor -
 * mirrors visualAssetKeys in hooks/use-visual-assets.ts exactly. */
export const audioAssetKeys = {
  list: () => ['audio-assets'] as const,
};

export function useAudioAssetsQuery(options: { enabled?: boolean } = {}): UseQueryResult<AudioAsset[], Error> {
  return useQuery({
    queryKey: audioAssetKeys.list(),
    queryFn: ({ signal }) => fetchAudioAssets(signal),
    enabled: options.enabled ?? true,
  });
}

export function useUploadAudioAssetMutation(): UseMutationResult<
  AudioAsset,
  Error,
  { file: File; displayName?: string }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ file, displayName }) => uploadAudioAsset(file, displayName),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: audioAssetKeys.list() });
    },
  });
}

/** Rejected with `error.code === 'audio_asset_in_use'` if still
 * referenced - the caller should show that as a stable, explained
 * condition rather than a generic failure. */
export function useDeleteAudioAssetMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id) => deleteAudioAsset(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: audioAssetKeys.list() });
    },
  });
}
