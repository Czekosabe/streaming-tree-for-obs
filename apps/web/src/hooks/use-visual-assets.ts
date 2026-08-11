import { useMemo } from 'react';
import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import { deleteVisualAsset, fetchVisualAssets, updateVisualAssetMetadata, uploadVisualAsset } from '@/api/visualasset';
import type { VisualAsset } from '@/api/visualasset-schemas';
import type { VisualAssetMap } from '@/components/visual-design/VisualLayer';

/** Shared by both Designers (mirrors visualTemplateKeys in
 * hooks/use-visual-templates.ts) - one managed-asset library, one
 * picker component, reused everywhere an image/video/font layer or a
 * custom-font reference needs to choose an asset. */
export const visualAssetKeys = {
  list: () => ['visual-assets'] as const,
};

export function useVisualAssetsQuery(options: { enabled?: boolean } = {}): UseQueryResult<VisualAsset[], Error> {
  return useQuery({
    queryKey: visualAssetKeys.list(),
    queryFn: ({ signal }) => fetchVisualAssets(signal),
    enabled: options.enabled ?? true,
  });
}

/** Builds the resolved-asset lookup a Designer's own preview needs
 * (Stage 14B task Part 42) - `{ <localAssetId>: { url, mediaType } }`,
 * derived from the same managed-asset library query the asset picker
 * uses, so both stay consistent with zero extra requests. */
export function useVisualAssetMap(): VisualAssetMap {
  const { data } = useVisualAssetsQuery();
  return useMemo(() => {
    const map: VisualAssetMap = {};
    for (const asset of data ?? []) {
      map[asset.id] = { url: asset.url, mediaType: asset.mediaType };
    }
    return map;
  }, [data]);
}

export function useUploadVisualAssetMutation(): UseMutationResult<
  VisualAsset,
  Error,
  { file: File; metadata?: { displayName?: string; author?: string; license?: string; notice?: string } }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ file, metadata }) => uploadVisualAsset(file, metadata),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: visualAssetKeys.list() });
    },
  });
}

export function useUpdateVisualAssetMetadataMutation(
  id: string,
): UseMutationResult<VisualAsset, Error, { displayName: string; author: string; license: string; notice: string }> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input) => updateVisualAssetMetadata(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: visualAssetKeys.list() });
    },
  });
}

/** Rejected with `error.code === 'visual_asset_in_use'` if still
 * referenced - the caller should show that as a stable, explained
 * condition rather than a generic failure. */
export function useDeleteVisualAssetMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id) => deleteVisualAsset(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: visualAssetKeys.list() });
    },
  });
}
