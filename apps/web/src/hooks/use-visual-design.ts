import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import { deleteVisualDesign, fetchVisualDesign, saveVisualDesign, type VisualDesignOwnerKind } from '@/api/visualdesign';
import type { VisualDesignDocument, VisualDesignResponse } from '@/api/visualdesign-schemas';

/** Shared by both designers (Stage 13B task Part 16) - `ownerKind`
 * selects the management-API path segment
 * (`/api/alert-rules/{id}/visual-design` vs
 * `/api/chat-overlays/{id}/visual-design`); everything else is
 * identical. */
export const visualDesignKeys = {
  design: (ownerKind: VisualDesignOwnerKind, ownerId: string) => ['visual-design', ownerKind, ownerId] as const,
};

export function useVisualDesignQuery(
  ownerKind: VisualDesignOwnerKind,
  ownerId: string | null,
): UseQueryResult<VisualDesignResponse, Error> {
  return useQuery({
    queryKey: visualDesignKeys.design(ownerKind, ownerId ?? ''),
    queryFn: ({ signal }) => fetchVisualDesign(ownerKind, ownerId ?? '', signal),
    enabled: ownerId !== null,
  });
}

export function useSaveVisualDesignMutation(
  ownerKind: VisualDesignOwnerKind,
  ownerId: string,
): UseMutationResult<VisualDesignResponse, Error, { document: VisualDesignDocument; expectedRevision: number }> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ document, expectedRevision }) => saveVisualDesign(ownerKind, ownerId, document, expectedRevision),
    onSuccess: (data) => {
      queryClient.setQueryData(visualDesignKeys.design(ownerKind, ownerId), data);
    },
  });
}

export function useDeleteVisualDesignMutation(ownerKind: VisualDesignOwnerKind, ownerId: string): UseMutationResult<void, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => deleteVisualDesign(ownerKind, ownerId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: visualDesignKeys.design(ownerKind, ownerId) });
    },
  });
}
