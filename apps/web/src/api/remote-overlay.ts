import { z } from 'zod';

import { apiGet, apiPost } from '@/lib/api-client';

import {
  remoteOverlayStatusSchema,
  remoteOverlayUrlResponseSchema,
  type RemoteOverlayDomain,
  type RemoteOverlayStatus,
  type RemoteOverlayUrlResponse,
} from './remote-overlay-schemas';

const disableResponseSchema = z.object({ status: z.string() });

function basePath(domain: RemoteOverlayDomain, localSlug: string): string {
  return `/api/remote-overlay/${domain}/${encodeURIComponent(localSlug)}`;
}

export async function fetchRemoteOverlayStatus(
  domain: RemoteOverlayDomain,
  localSlug: string,
  signal?: AbortSignal,
): Promise<RemoteOverlayStatus> {
  return apiGet(`${basePath(domain, localSlug)}/status`, remoteOverlayStatusSchema, { signal });
}

export async function enableRemoteOverlay(
  domain: RemoteOverlayDomain,
  localSlug: string,
): Promise<RemoteOverlayUrlResponse> {
  return apiPost(`${basePath(domain, localSlug)}/enable`, undefined, remoteOverlayUrlResponseSchema);
}

export async function rotateRemoteOverlay(
  domain: RemoteOverlayDomain,
  localSlug: string,
): Promise<RemoteOverlayUrlResponse> {
  return apiPost(`${basePath(domain, localSlug)}/rotate`, undefined, remoteOverlayUrlResponseSchema);
}

export async function disableRemoteOverlay(domain: RemoteOverlayDomain, localSlug: string): Promise<void> {
  await apiPost(`${basePath(domain, localSlug)}/disable`, undefined, disableResponseSchema);
}
