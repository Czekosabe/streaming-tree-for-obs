import { z } from 'zod';

import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  alertEventTypeCapabilitySchema,
  alertProfileSchema,
  alertQueueStatusSchema,
  alertRulePreviewResponseSchema,
  alertRuleSchema,
  alertSummarySchema,
  listAlertRulesResponseSchema,
  publicAlertProfileConfigSchema,
  type AlertEventTypeCapability,
  type AlertProfile,
  type AlertProfileInput,
  type AlertQueueStatus,
  type AlertRule,
  type AlertRuleInput,
  type AlertRulePreviewInput,
  type AlertRulePreviewResponse,
  type AlertSummary,
  type ListAlertRulesResponse,
  type PublicAlertProfileConfig,
} from './alerts-schemas';

/**
 * Transport for the Stage 12A alert API. No caching or React concerns
 * live here - see hooks/use-alerts.ts.
 */

const profileListSchema = z.array(alertProfileSchema);
const eventTypeListSchema = z.array(alertEventTypeCapabilitySchema);

export async function fetchAlertEventTypes(signal?: AbortSignal): Promise<AlertEventTypeCapability[]> {
  return apiGet('/api/alert-event-types', eventTypeListSchema, { signal });
}

export async function fetchAlertProfiles(signal?: AbortSignal): Promise<AlertProfile[]> {
  return apiGet('/api/alert-profiles', profileListSchema, { signal });
}

export async function fetchAlertProfile(id: string, signal?: AbortSignal): Promise<AlertProfile> {
  return apiGet(`/api/alert-profiles/${id}`, alertProfileSchema, { signal });
}

export async function createAlertProfile(name: string): Promise<AlertProfile> {
  return apiPost('/api/alert-profiles', { name }, alertProfileSchema);
}

export async function updateAlertProfile(id: string, input: AlertProfileInput): Promise<AlertProfile> {
  return apiPut(`/api/alert-profiles/${id}`, input, alertProfileSchema);
}

export async function deleteAlertProfile(id: string): Promise<void> {
  return apiDelete(`/api/alert-profiles/${id}`);
}

export async function rotateAlertProfileSlug(id: string): Promise<AlertProfile> {
  return apiPost(`/api/alert-profiles/${id}/rotate-public-slug`, undefined, alertProfileSchema);
}

export async function fetchAlertRules(profileId: string, signal?: AbortSignal): Promise<ListAlertRulesResponse> {
  return apiGet(`/api/alert-profiles/${profileId}/rules`, listAlertRulesResponseSchema, { signal });
}

export async function fetchAlertRule(id: string, signal?: AbortSignal): Promise<AlertRule> {
  return apiGet(`/api/alert-rules/${id}`, alertRuleSchema, { signal });
}

export async function createAlertRule(profileId: string, input: AlertRuleInput): Promise<AlertRule> {
  return apiPost(`/api/alert-profiles/${profileId}/rules`, input, alertRuleSchema);
}

export async function updateAlertRule(id: string, input: AlertRuleInput): Promise<AlertRule> {
  return apiPut(`/api/alert-rules/${id}`, input, alertRuleSchema);
}

export async function deleteAlertRule(id: string): Promise<void> {
  return apiDelete(`/api/alert-rules/${id}`);
}

export async function testAlertRule(id: string, scenario?: string): Promise<AlertSummary> {
  return apiPost(
    `/api/alert-rules/${id}/test`,
    scenario !== undefined && scenario !== '' ? { scenario } : {},
    alertSummarySchema,
  );
}

/** Local template preview against representative fixture data - never
 * sends anything, never persists anything, never touches the queue. */
export async function previewAlertTemplate(input: AlertRulePreviewInput): Promise<AlertRulePreviewResponse> {
  return apiPost('/api/alert-rule-preview', input, alertRulePreviewResponseSchema);
}

export async function fetchAlertQueueStatus(profileId: string, signal?: AbortSignal): Promise<AlertQueueStatus> {
  return apiGet(`/api/alert-profiles/${profileId}/queue`, alertQueueStatusSchema, { signal });
}

export async function pauseAlertQueue(profileId: string): Promise<AlertQueueStatus> {
  return apiPost(`/api/alert-profiles/${profileId}/queue/pause`, undefined, alertQueueStatusSchema);
}

export async function resumeAlertQueue(profileId: string): Promise<AlertQueueStatus> {
  return apiPost(`/api/alert-profiles/${profileId}/queue/resume`, undefined, alertQueueStatusSchema);
}

export async function skipCurrentAlert(profileId: string): Promise<AlertQueueStatus> {
  return apiPost(`/api/alert-profiles/${profileId}/queue/skip-current`, undefined, alertQueueStatusSchema);
}

export async function replayPreviousAlert(profileId: string): Promise<AlertQueueStatus> {
  return apiPost(`/api/alert-profiles/${profileId}/queue/replay-previous`, undefined, alertQueueStatusSchema);
}

export async function clearAlertQueue(profileId: string): Promise<AlertQueueStatus> {
  return apiPost(`/api/alert-profiles/${profileId}/queue/clear`, undefined, alertQueueStatusSchema);
}

export async function fetchPublicAlertProfileConfig(
  publicSlug: string,
  signal?: AbortSignal,
): Promise<PublicAlertProfileConfig> {
  return apiGet(`/api/public/alert-profiles/${publicSlug}/config`, publicAlertProfileConfigSchema, { signal });
}
