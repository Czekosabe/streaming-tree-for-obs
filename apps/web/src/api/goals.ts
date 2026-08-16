import { z } from 'zod';

import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  goalSchema,
  widgetProfileSchema,
  type Goal,
  type GoalInput,
  type WidgetProfile,
  type WidgetProfileInput,
} from './goals-schemas';

/**
 * Transport for the Stage 18A goals/widget-profile API. No caching or
 * React concerns live here - see hooks/use-goals.ts.
 */

const goalListSchema = z.array(goalSchema);
const widgetProfileListSchema = z.array(widgetProfileSchema);

export async function fetchGoals(signal?: AbortSignal): Promise<Goal[]> {
  return apiGet('/api/goals', goalListSchema, { signal });
}

export async function fetchGoal(id: string, signal?: AbortSignal): Promise<Goal> {
  return apiGet(`/api/goals/${id}`, goalSchema, { signal });
}

export async function createGoal(input: GoalInput): Promise<Goal> {
  return apiPost('/api/goals', input, goalSchema);
}

export async function updateGoal(id: string, input: GoalInput): Promise<Goal> {
  return apiPut(`/api/goals/${id}`, input, goalSchema);
}

export async function deleteGoal(id: string): Promise<void> {
  return apiDelete(`/api/goals/${id}`);
}

export async function setGoalCurrent(id: string, current: number): Promise<Goal> {
  return apiPost(`/api/goals/${id}/set-current`, { current }, goalSchema);
}

export async function resetGoal(id: string): Promise<Goal> {
  return apiPost(`/api/goals/${id}/reset`, undefined, goalSchema);
}

export async function fetchWidgetProfiles(goalId?: string, signal?: AbortSignal): Promise<WidgetProfile[]> {
  const query = goalId ? `?goalId=${encodeURIComponent(goalId)}` : '';
  return apiGet(`/api/widget-profiles${query}`, widgetProfileListSchema, { signal });
}

export async function createWidgetProfile(input: WidgetProfileInput): Promise<WidgetProfile> {
  return apiPost('/api/widget-profiles', input, widgetProfileSchema);
}

export async function updateWidgetProfile(id: string, input: WidgetProfileInput): Promise<WidgetProfile> {
  return apiPut(`/api/widget-profiles/${id}`, input, widgetProfileSchema);
}

export async function deleteWidgetProfile(id: string): Promise<void> {
  return apiDelete(`/api/widget-profiles/${id}`);
}

export async function rotateWidgetProfileSlug(id: string): Promise<WidgetProfile> {
  return apiPost(`/api/widget-profiles/${id}/rotate-public-slug`, undefined, widgetProfileSchema);
}
