import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  createCommand,
  createSchedule,
  deleteCommand,
  deleteSchedule,
  fetchChatAutomationStatus,
  fetchCommand,
  fetchCommands,
  fetchSchedule,
  fetchSchedules,
  previewTemplate,
  sendScheduleNow,
  updateCommand,
  updateSchedule,
} from '@/api/chat-automation';
import type {
  ChatAutomationStatus,
  Command,
  CommandInput,
  PreviewInput,
  PreviewResponse,
  Schedule,
  ScheduleInput,
  SendNowResponse,
} from '@/api/chat-automation-schemas';

export const chatAutomationKeys = {
  status: () => ['chat-automation-status'] as const,
  schedules: () => ['chat-automation-schedules'] as const,
  schedule: (id: string) => ['chat-automation-schedules', id] as const,
  commands: () => ['chat-automation-commands'] as const,
  command: (id: string) => ['chat-automation-commands', id] as const,
};

/** How often the status/list views poll while mounted - matches this
 * project's existing engagement/outbound-chat precedent. */
const POLL_INTERVAL_MS = 5_000;

export function useChatAutomationStatusQuery(): UseQueryResult<ChatAutomationStatus, Error> {
  return useQuery({
    queryKey: chatAutomationKeys.status(),
    queryFn: ({ signal }) => fetchChatAutomationStatus(signal),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useSchedulesQuery(): UseQueryResult<Schedule[], Error> {
  return useQuery({
    queryKey: chatAutomationKeys.schedules(),
    queryFn: ({ signal }) => fetchSchedules(signal),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useScheduleQuery(id: string | null): UseQueryResult<Schedule, Error> {
  return useQuery({
    queryKey: chatAutomationKeys.schedule(id ?? ''),
    queryFn: ({ signal }) => fetchSchedule(id ?? '', signal),
    enabled: id !== null,
  });
}

function useInvalidateSchedules() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: chatAutomationKeys.schedules() });
    void queryClient.invalidateQueries({ queryKey: chatAutomationKeys.status() });
  };
}

export function useCreateScheduleMutation(): UseMutationResult<Schedule, Error, ScheduleInput> {
  const invalidate = useInvalidateSchedules();
  return useMutation({
    mutationFn: (input: ScheduleInput) => createSchedule(input),
    onSuccess: invalidate,
  });
}

export function useUpdateScheduleMutation(): UseMutationResult<
  Schedule,
  Error,
  { id: string; input: ScheduleInput }
> {
  const invalidate = useInvalidateSchedules();
  return useMutation({
    mutationFn: ({ id, input }) => updateSchedule(id, input),
    onSuccess: invalidate,
  });
}

export function useDeleteScheduleMutation(): UseMutationResult<void, Error, string> {
  const invalidate = useInvalidateSchedules();
  return useMutation({
    mutationFn: (id: string) => deleteSchedule(id),
    onSuccess: invalidate,
  });
}

export function useSendScheduleNowMutation(): UseMutationResult<
  SendNowResponse,
  Error,
  { id: string; accountIds: string[] }
> {
  const invalidate = useInvalidateSchedules();
  return useMutation({
    mutationFn: ({ id, accountIds }) => sendScheduleNow(id, accountIds),
    onSuccess: invalidate,
  });
}

export function useCommandsQuery(): UseQueryResult<Command[], Error> {
  return useQuery({
    queryKey: chatAutomationKeys.commands(),
    queryFn: ({ signal }) => fetchCommands(signal),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

export function useCommandQuery(id: string | null): UseQueryResult<Command, Error> {
  return useQuery({
    queryKey: chatAutomationKeys.command(id ?? ''),
    queryFn: ({ signal }) => fetchCommand(id ?? '', signal),
    enabled: id !== null,
  });
}

function useInvalidateCommands() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: chatAutomationKeys.commands() });
    void queryClient.invalidateQueries({ queryKey: chatAutomationKeys.status() });
  };
}

export function useCreateCommandMutation(): UseMutationResult<Command, Error, CommandInput> {
  const invalidate = useInvalidateCommands();
  return useMutation({
    mutationFn: (input: CommandInput) => createCommand(input),
    onSuccess: invalidate,
  });
}

export function useUpdateCommandMutation(): UseMutationResult<
  Command,
  Error,
  { id: string; input: CommandInput }
> {
  const invalidate = useInvalidateCommands();
  return useMutation({
    mutationFn: ({ id, input }) => updateCommand(id, input),
    onSuccess: invalidate,
  });
}

export function useDeleteCommandMutation(): UseMutationResult<void, Error, string> {
  const invalidate = useInvalidateCommands();
  return useMutation({
    mutationFn: (id: string) => deleteCommand(id),
    onSuccess: invalidate,
  });
}

/** Local preview rendering - never invalidates any cache, since it
 * neither sends nor persists anything. */
export function usePreviewMutation(): UseMutationResult<PreviewResponse, Error, PreviewInput> {
  return useMutation({
    mutationFn: (input: PreviewInput) => previewTemplate(input),
  });
}
