import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import { deleteVisualDesign, fetchVisualDesign, saveVisualDesign } from '@/api/visualdesign';
import type { VisualDesignDocument, VisualDesignResponse } from '@/api/visualdesign-schemas';

export const visualDesignKeys = {
  design: (ruleId: string) => ['visual-design', ruleId] as const,
};

export function useVisualDesignQuery(ruleId: string | null): UseQueryResult<VisualDesignResponse, Error> {
  return useQuery({
    queryKey: visualDesignKeys.design(ruleId ?? ''),
    queryFn: ({ signal }) => fetchVisualDesign(ruleId ?? '', signal),
    enabled: ruleId !== null,
  });
}

export function useSaveVisualDesignMutation(
  ruleId: string,
): UseMutationResult<VisualDesignResponse, Error, { document: VisualDesignDocument; expectedRevision: number }> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ document, expectedRevision }) => saveVisualDesign(ruleId, document, expectedRevision),
    onSuccess: (data) => {
      queryClient.setQueryData(visualDesignKeys.design(ruleId), data);
    },
  });
}

export function useDeleteVisualDesignMutation(ruleId: string): UseMutationResult<void, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => deleteVisualDesign(ruleId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: visualDesignKeys.design(ruleId) });
    },
  });
}
