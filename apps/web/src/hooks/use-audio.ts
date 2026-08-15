import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  approveAudioPendingItem,
  clearAudioQueue,
  fetchAudioCapabilities,
  fetchAudioPending,
  fetchAudioSettings,
  fetchAudioStatus,
  fetchAudioVoices,
  rejectAudioPendingItem,
  rotateAudioPublicSlug,
  skipAudioQueueCurrent,
  testSpeakAudio,
  updateAudioSettings,
} from '@/api/audio';
import type {
  AudioCapabilities,
  AudioPendingItem,
  AudioSettings,
  AudioSettingsInput,
  AudioStatus,
  AudioVoice,
} from '@/api/audio-schemas';

/** How often the status/pending views poll while mounted - matches
 * this project's existing chat-automation/alerts-queue precedent. */
const POLL_INTERVAL_MS = 5_000;

export const audioKeys = {
  settings: () => ['audio-settings'] as const,
  capabilities: () => ['audio-capabilities'] as const,
  voices: () => ['audio-voices'] as const,
  status: () => ['audio-status'] as const,
  pending: () => ['audio-pending'] as const,
};

export function useAudioSettingsQuery(): UseQueryResult<AudioSettings, Error> {
  return useQuery({
    queryKey: audioKeys.settings(),
    queryFn: ({ signal }) => fetchAudioSettings(signal),
  });
}

export function useUpdateAudioSettingsMutation(): UseMutationResult<AudioSettings, Error, AudioSettingsInput> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AudioSettingsInput) => updateAudioSettings(input),
    onSuccess: (settings) => {
      queryClient.setQueryData(audioKeys.settings(), settings);
    },
  });
}

export function useAudioCapabilitiesQuery(): UseQueryResult<AudioCapabilities, Error> {
  return useQuery({
    queryKey: audioKeys.capabilities(),
    queryFn: ({ signal }) => fetchAudioCapabilities(signal),
  });
}

export function useAudioVoicesQuery(): UseQueryResult<AudioVoice[], Error> {
  return useQuery({
    queryKey: audioKeys.voices(),
    queryFn: ({ signal }) => fetchAudioVoices(signal),
  });
}

export function useAudioStatusQuery(): UseQueryResult<AudioStatus, Error> {
  return useQuery({
    queryKey: audioKeys.status(),
    queryFn: ({ signal }) => fetchAudioStatus(signal),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useAudioPendingQuery(): UseQueryResult<AudioPendingItem[], Error> {
  return useQuery({
    queryKey: audioKeys.pending(),
    queryFn: ({ signal }) => fetchAudioPending(signal),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

function useInvalidateStatusAndPending() {
  const queryClient = useQueryClient();
  return (status: AudioStatus) => {
    queryClient.setQueryData(audioKeys.status(), status);
    void queryClient.invalidateQueries({ queryKey: audioKeys.pending() });
  };
}

export function useSkipAudioQueueCurrentMutation(): UseMutationResult<AudioStatus, Error, void> {
  const applyResult = useInvalidateStatusAndPending();
  return useMutation({ mutationFn: () => skipAudioQueueCurrent(), onSuccess: applyResult });
}

export function useClearAudioQueueMutation(): UseMutationResult<AudioStatus, Error, void> {
  const applyResult = useInvalidateStatusAndPending();
  return useMutation({ mutationFn: () => clearAudioQueue(), onSuccess: applyResult });
}

export function useApproveAudioPendingItemMutation(): UseMutationResult<AudioStatus, Error, string> {
  const applyResult = useInvalidateStatusAndPending();
  return useMutation({ mutationFn: (id: string) => approveAudioPendingItem(id), onSuccess: applyResult });
}

export function useRejectAudioPendingItemMutation(): UseMutationResult<AudioStatus, Error, string> {
  const applyResult = useInvalidateStatusAndPending();
  return useMutation({ mutationFn: (id: string) => rejectAudioPendingItem(id), onSuccess: applyResult });
}

export function useRotateAudioPublicSlugMutation(): UseMutationResult<AudioSettings, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => rotateAudioPublicSlug(),
    onSuccess: (settings) => {
      queryClient.setQueryData(audioKeys.settings(), settings);
    },
  });
}

/** Test Speak persists nothing else worth caching beyond the queue/
 * status counters it affects - refresh those, but there is no
 * dedicated "test speak result" query to update. */
export function useTestSpeakAudioMutation(): UseMutationResult<AudioPendingItem, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (text: string) => testSpeakAudio(text),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: audioKeys.status() });
      void queryClient.invalidateQueries({ queryKey: audioKeys.pending() });
    },
  });
}
