import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  createDonationSource,
  deleteDonationSource,
  fetchDonationSource,
  fetchDonationSourceEngagement,
  fetchDonationSources,
  replaceDonationSourceCredential,
  restartDonationSourceEngagement,
  setDonationSourceEngagement,
  updateDonationSource,
} from '@/api/donationsources';
import type {
  CreateDonationSourceInput,
  DonationConnector,
  DonationSource,
  ReplaceDonationSourceCredentialInput,
  SetDonationEngagementInput,
  UpdateDonationSourceInput,
} from '@/api/donationsource-schemas';

/** Query keys. None carries a secret - a donation source id is a
 * non-secret identifier, matching hooks/use-accounts.ts's own precedent. */
export const donationSourceKeys = {
  list: ['donation-sources'] as const,
  source: (id: string) => ['donation-source', id] as const,
  engagement: (id: string) => ['donation-source-engagement', id] as const,
};

/** How often the connector-status card polls while mounted - matches
 * hooks/use-engagement.ts's own STATUS_POLL_INTERVAL_MS. */
const STATUS_POLL_INTERVAL_MS = 5_000;

export function useDonationSourcesQuery(): UseQueryResult<DonationSource[], Error> {
  return useQuery({
    queryKey: donationSourceKeys.list,
    queryFn: ({ signal }) => fetchDonationSources(signal),
  });
}

export function useDonationSourceQuery(id: string): UseQueryResult<DonationSource, Error> {
  return useQuery({
    queryKey: donationSourceKeys.source(id),
    queryFn: ({ signal }) => fetchDonationSource(id, signal),
  });
}

export function useDonationSourceEngagementQuery(id: string): UseQueryResult<DonationConnector, Error> {
  return useQuery({
    queryKey: donationSourceKeys.engagement(id),
    queryFn: ({ signal }) => fetchDonationSourceEngagement(id, signal),
    refetchInterval: STATUS_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useCreateDonationSourceMutation(): UseMutationResult<
  DonationSource,
  Error,
  CreateDonationSourceInput
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDonationSourceInput) => createDonationSource(input),
    // The credential the operator just pasted is a mutation variable -
    // never left in the mutation cache beyond garbage collection, mirrors
    // hooks/use-credentials.ts's own useSetStreamKeyMutation exactly.
    gcTime: 0,
    onSuccess: (created) => {
      queryClient.setQueryData(donationSourceKeys.source(created.id), created);
      void queryClient.invalidateQueries({ queryKey: donationSourceKeys.list });
    },
  });
}

export function useUpdateDonationSourceMutation(): UseMutationResult<
  DonationSource,
  Error,
  { id: string; input: UpdateDonationSourceInput }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }) => updateDonationSource(id, input),
    onSuccess: (updated, variables) => {
      queryClient.setQueryData(donationSourceKeys.source(variables.id), updated);
      void queryClient.invalidateQueries({ queryKey: donationSourceKeys.list });
    },
  });
}

export function useReplaceDonationSourceCredentialMutation(): UseMutationResult<
  { configured: boolean },
  Error,
  { id: string; input: ReplaceDonationSourceCredentialInput }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }) => replaceDonationSourceCredential(id, input),
    gcTime: 0,
    onSuccess: (_status, variables) => {
      void queryClient.invalidateQueries({ queryKey: donationSourceKeys.source(variables.id) });
      void queryClient.invalidateQueries({ queryKey: donationSourceKeys.list });
    },
  });
}

export function useDeleteDonationSourceMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteDonationSource(id),
    onSuccess: (_result, id) => {
      queryClient.removeQueries({ queryKey: donationSourceKeys.source(id) });
      queryClient.removeQueries({ queryKey: donationSourceKeys.engagement(id) });
      void queryClient.invalidateQueries({ queryKey: donationSourceKeys.list });
    },
  });
}

export function useSetDonationEngagementMutation(): UseMutationResult<
  DonationConnector,
  Error,
  { id: string; input: SetDonationEngagementInput }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }) => setDonationSourceEngagement(id, input),
    onSuccess: (data, variables) => {
      queryClient.setQueryData(donationSourceKeys.engagement(variables.id), data);
      void queryClient.invalidateQueries({ queryKey: donationSourceKeys.list });
    },
  });
}

export function useRestartDonationEngagementMutation(): UseMutationResult<DonationConnector, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => restartDonationSourceEngagement(id),
    onSuccess: (data, id) => {
      queryClient.setQueryData(donationSourceKeys.engagement(id), data);
    },
  });
}
