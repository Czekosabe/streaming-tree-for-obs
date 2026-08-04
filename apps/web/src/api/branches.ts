import { ApiError, apiGet, apiPost } from '@/lib/api-client';

import {
  BRANCHES_SCHEMA_VERSION,
  branchCommandSchema,
  branchesResponseSchema,
  FFMPEG_SCHEMA_VERSION,
  ffmpegStatusSchema,
  startEnabledResponseSchema,
  type BranchCommandResponse,
  type BranchSnapshot,
  type FFmpegStatus,
  type StartEnabledResponse,
} from './branch-schemas';

/**
 * Transport for the FFmpeg dependency and destination-branch runtime APIs.
 * No caching or React concerns live here - see hooks/use-branches.ts.
 */

const NO_BODY = undefined;

export async function fetchFFmpegStatus(signal?: AbortSignal): Promise<FFmpegStatus> {
  const status = await apiGet('/api/runtime/ffmpeg', ffmpegStatusSchema, { signal });
  if (status.version !== FFMPEG_SCHEMA_VERSION) {
    throw new ApiError(
      'parse',
      `The backend returned FFmpeg status schema version ${status.version}, but this build understands ${FFMPEG_SCHEMA_VERSION}.`,
    );
  }
  return status;
}

export async function fetchBranches(signal?: AbortSignal): Promise<BranchSnapshot[]> {
  const response = await apiGet('/api/runtime/branches', branchesResponseSchema, { signal });
  if (response.version !== BRANCHES_SCHEMA_VERSION) {
    throw new ApiError(
      'parse',
      `The backend returned branch schema version ${response.version}, but this build understands ${BRANCHES_SCHEMA_VERSION}.`,
    );
  }
  return response.branches;
}

export async function startBranch(platformId: string): Promise<BranchCommandResponse> {
  return apiPost(
    `/api/runtime/branches/${encodeURIComponent(platformId)}/start`,
    NO_BODY,
    branchCommandSchema,
  );
}

export async function stopBranch(platformId: string): Promise<BranchCommandResponse> {
  return apiPost(
    `/api/runtime/branches/${encodeURIComponent(platformId)}/stop`,
    NO_BODY,
    branchCommandSchema,
  );
}

export async function restartBranch(platformId: string): Promise<BranchCommandResponse> {
  return apiPost(
    `/api/runtime/branches/${encodeURIComponent(platformId)}/restart`,
    NO_BODY,
    branchCommandSchema,
  );
}

export async function startEnabledBranches(): Promise<StartEnabledResponse> {
  return apiPost('/api/runtime/branches/start-enabled', NO_BODY, startEnabledResponseSchema);
}

export async function stopAllBranches(): Promise<BranchCommandResponse> {
  return apiPost('/api/runtime/branches/stop-all', NO_BODY, branchCommandSchema);
}
