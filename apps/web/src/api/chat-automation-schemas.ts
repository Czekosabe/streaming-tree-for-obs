import { z } from 'zod';

/**
 * Zod contracts for the Stage 11B chat-automation API
 * (`internal/httpapi/chatautomation.go`): scheduled messages and safe
 * chat commands built on top of Stage 11A's outbound dispatcher.
 *
 * No field here ever carries a token, a raw provider response, or a
 * triggering chat username - the backend response shapes never carry
 * one, so there is nothing to strip.
 */

export const chatAutomationTargetSchema = z.object({
  accountId: z.string(),
  platformId: z.string().optional(),
});
export type ChatAutomationTarget = z.infer<typeof chatAutomationTargetSchema>;

export const chatAutomationTargetStatusSchema = z.object({
  accountId: z.string(),
  lastAttemptAt: z.string().optional(),
  lastSuccessAt: z.string().optional(),
  lastSkipReason: z.string().optional(),
  sendsThisHour: z.number(),
});
export type ChatAutomationTargetStatus = z.infer<typeof chatAutomationTargetStatusSchema>;

export const scheduleMessageSchema = z.object({
  id: z.string().optional(),
  template: z.string(),
});
export type ScheduleMessage = z.infer<typeof scheduleMessageSchema>;

export const scheduleStateSchema = z.enum([
  'disabled',
  'scheduled',
  'waiting_for_stream',
  'waiting_for_activity',
  'rate_limited',
  'permission_required',
  'sending',
  'error',
]);
export type ScheduleState = z.infer<typeof scheduleStateSchema>;

export const scheduleSchema = z.object({
  id: z.string(),
  name: z.string(),
  enabled: z.boolean(),
  intervalSeconds: z.number(),
  firstDelaySeconds: z.number(),
  jitterSeconds: z.number(),
  onlyWhileIngestReceiving: z.boolean(),
  minimumChatMessages: z.number(),
  maximumSendsPerHour: z.number(),
  targets: z.array(chatAutomationTargetSchema),
  messages: z.array(scheduleMessageSchema),
  createdAt: z.string(),
  updatedAt: z.string(),
  state: scheduleStateSchema,
  nextRunAt: z.string().optional(),
  lastAttemptAt: z.string().optional(),
  lastSuccessAt: z.string().optional(),
  lastSkipReason: z.string().optional(),
  targetStatus: z.array(chatAutomationTargetStatusSchema).optional(),
});
export type Schedule = z.infer<typeof scheduleSchema>;

/** Request body for creating/replacing a schedule. */
export type ScheduleInput = {
  name: string;
  enabled: boolean;
  intervalSeconds: number;
  firstDelaySeconds: number;
  jitterSeconds: number;
  onlyWhileIngestReceiving: boolean;
  minimumChatMessages: number;
  maximumSendsPerHour: number;
  targets: ChatAutomationTarget[];
  messages: string[];
};

export const commandRoleSchema = z.enum(['everyone', 'subscriber', 'vip', 'moderator', 'broadcaster']);
export type CommandRole = z.infer<typeof commandRoleSchema>;

export const commandSchema = z.object({
  id: z.string(),
  name: z.string(),
  enabled: z.boolean(),
  responseTemplate: z.string(),
  requiredRole: commandRoleSchema,
  globalCooldownSeconds: z.number(),
  userCooldownSeconds: z.number(),
  aliases: z.array(z.string()),
  targets: z.array(chatAutomationTargetSchema),
  createdAt: z.string(),
  updatedAt: z.string(),
  matchCount: z.number(),
  responseCount: z.number(),
  lastResponseAt: z.string().optional(),
});
export type Command = z.infer<typeof commandSchema>;

/** Request body for creating/replacing a command. */
export type CommandInput = {
  name: string;
  enabled: boolean;
  responseTemplate: string;
  requiredRole: CommandRole;
  globalCooldownSeconds: number;
  userCooldownSeconds: number;
  aliases: string[];
  targets: ChatAutomationTarget[];
};

export const engineStatusSchema = z.object({
  running: z.boolean(),
  subscribedToBus: z.boolean(),
  commandCount: z.number(),
  enabledCommandCount: z.number(),
  totalMatched: z.number(),
  totalResponses: z.number(),
  totalCooldownSkips: z.number(),
  totalRoleSkips: z.number(),
  totalSelfSkips: z.number(),
  lastErrorCode: z.string().optional(),
});
export type EngineStatus = z.infer<typeof engineStatusSchema>;

export const chatAutomationStatusSchema = z.object({
  engine: engineStatusSchema,
  schedules: z.array(scheduleSchema),
  commands: z.array(commandSchema),
});
export type ChatAutomationStatus = z.infer<typeof chatAutomationStatusSchema>;

export const sendNowResultSchema = z.object({
  accountId: z.string(),
  sent: z.boolean(),
  providerMessageId: z.string().optional(),
  skipReason: z.string().optional(),
});
export type SendNowResult = z.infer<typeof sendNowResultSchema>;

export const sendNowResponseSchema = z.object({
  results: z.array(sendNowResultSchema),
});
export type SendNowResponse = z.infer<typeof sendNowResponseSchema>;

export const previewResponseSchema = z.object({
  renderedText: z.string(),
  codePointCount: z.number(),
  resolvedPlaceholders: z.array(z.string()).optional(),
  unresolvedPlaceholders: z.array(z.string()).optional(),
  validForProvider: z.boolean(),
  warnings: z.array(z.string()).optional(),
});
export type PreviewResponse = z.infer<typeof previewResponseSchema>;

export type PreviewInput = {
  template: string;
  accountId: string;
  platformId?: string;
};

/** Every stable chat_automation_* error code the backend can return,
 * plus shared codes it reuses from the rest of the API. */
export const chatAutomationErrorCodeSchema = z.enum([
  'chat_automation_not_found',
  'chat_automation_account_not_found',
  'chat_automation_target_required',
  'chat_automation_target_invalid',
  'chat_automation_command_conflict',
  'chat_automation_invalid',
  'chat_automation_placeholder_invalid',
  'chat_automation_provider_unsupported',
  'chat_automation_permission_required',
  'chat_automation_queue_full',
  'chat_automation_rate_limited',
  'account_reconnect_required',
]);
export type ChatAutomationErrorCode = z.infer<typeof chatAutomationErrorCodeSchema>;
