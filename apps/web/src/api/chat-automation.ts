import { z } from 'zod';

import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  chatAutomationStatusSchema,
  commandSchema,
  previewResponseSchema,
  scheduleSchema,
  sendNowResponseSchema,
  type ChatAutomationStatus,
  type Command,
  type CommandInput,
  type PreviewInput,
  type PreviewResponse,
  type Schedule,
  type ScheduleInput,
  type SendNowResponse,
} from './chat-automation-schemas';

/**
 * Transport for the Stage 11B chat-automation API. No caching or React
 * concerns live here - see hooks/use-chat-automation.ts.
 */

const scheduleListSchema = z.array(scheduleSchema);
const commandListSchema = z.array(commandSchema);

export async function fetchChatAutomationStatus(signal?: AbortSignal): Promise<ChatAutomationStatus> {
  return apiGet('/api/chat-automation/status', chatAutomationStatusSchema, { signal });
}

export async function fetchSchedules(signal?: AbortSignal): Promise<Schedule[]> {
  return apiGet('/api/chat-automation/schedules', scheduleListSchema, { signal });
}

export async function fetchSchedule(id: string, signal?: AbortSignal): Promise<Schedule> {
  return apiGet(`/api/chat-automation/schedules/${id}`, scheduleSchema, { signal });
}

export async function createSchedule(input: ScheduleInput): Promise<Schedule> {
  return apiPost('/api/chat-automation/schedules', input, scheduleSchema);
}

export async function updateSchedule(id: string, input: ScheduleInput): Promise<Schedule> {
  return apiPut(`/api/chat-automation/schedules/${id}`, input, scheduleSchema);
}

export async function deleteSchedule(id: string): Promise<void> {
  return apiDelete(`/api/chat-automation/schedules/${id}`);
}

export async function sendScheduleNow(id: string, accountIds: string[]): Promise<SendNowResponse> {
  return apiPost(
    `/api/chat-automation/schedules/${id}/send-now`,
    accountIds.length > 0 ? { accountIds } : {},
    sendNowResponseSchema,
  );
}

export async function fetchCommands(signal?: AbortSignal): Promise<Command[]> {
  return apiGet('/api/chat-automation/commands', commandListSchema, { signal });
}

export async function fetchCommand(id: string, signal?: AbortSignal): Promise<Command> {
  return apiGet(`/api/chat-automation/commands/${id}`, commandSchema, { signal });
}

export async function createCommand(input: CommandInput): Promise<Command> {
  return apiPost('/api/chat-automation/commands', input, commandSchema);
}

export async function updateCommand(id: string, input: CommandInput): Promise<Command> {
  return apiPut(`/api/chat-automation/commands/${id}`, input, commandSchema);
}

export async function deleteCommand(id: string): Promise<void> {
  return apiDelete(`/api/chat-automation/commands/${id}`);
}

/** Local template preview - never sends anything, never persists
 * anything, never makes a provider network request. */
export async function previewTemplate(input: PreviewInput): Promise<PreviewResponse> {
  return apiPost('/api/chat-automation/preview', input, previewResponseSchema);
}
