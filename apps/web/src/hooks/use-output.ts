import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import { fetchOutputSettings, updateOutputSettings } from '@/api/output';
import type { OutputSettings, UpdateOutputSettingsInput } from '@/api/output-schemas';

/** Query keys, scoped per platform. */
export const outputKeys = {
  settings: (platformId: string) => ['platform-output', platformId] as const,
};

export function useOutputSettingsQuery(platformId: string): UseQueryResult<OutputSettings, Error> {
  return useQuery({
    queryKey: outputKeys.settings(platformId),
    queryFn: ({ signal }) => fetchOutputSettings(platformId, signal),
  });
}

export function useUpdateOutputSettingsMutation(): UseMutationResult<
  OutputSettings,
  Error,
  { platformId: string; input: UpdateOutputSettingsInput }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ platformId, input }) => updateOutputSettings(platformId, input),
    onSuccess: (settings, variables) => {
      queryClient.setQueryData(outputKeys.settings(variables.platformId), settings);
    },
  });
}
