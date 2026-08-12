import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  donationConnectorSchema,
  donationCredentialStatusSchema,
  donationSourceSchema,
  donationSourcesResponseSchema,
  type CreateDonationSourceInput,
  type DonationConnector,
  type DonationSource,
  type ReplaceDonationSourceCredentialInput,
  type SetDonationEngagementInput,
  type UpdateDonationSourceInput,
} from './donationsource-schemas';

/**
 * Transport for the Stage 16A external-donation-source management API. No
 * caching or React concerns live here - see hooks/use-donationsources.ts.
 */

const NO_BODY = undefined;

export async function fetchDonationSources(signal?: AbortSignal): Promise<DonationSource[]> {
  const response = await apiGet('/api/donation-sources', donationSourcesResponseSchema, { signal });
  return response.items;
}

export async function fetchDonationSource(id: string, signal?: AbortSignal): Promise<DonationSource> {
  return apiGet(`/api/donation-sources/${id}`, donationSourceSchema, { signal });
}

export async function createDonationSource(input: CreateDonationSourceInput): Promise<DonationSource> {
  return apiPost('/api/donation-sources', input, donationSourceSchema);
}

export async function updateDonationSource(
  id: string,
  input: UpdateDonationSourceInput,
): Promise<DonationSource> {
  return apiPut(`/api/donation-sources/${id}`, input, donationSourceSchema);
}

export async function deleteDonationSource(id: string): Promise<void> {
  await apiDelete(`/api/donation-sources/${id}`);
}

export async function replaceDonationSourceCredential(
  id: string,
  input: ReplaceDonationSourceCredentialInput,
): Promise<{ configured: boolean }> {
  return apiPut(`/api/donation-sources/${id}/credential`, input, donationCredentialStatusSchema);
}

export async function fetchDonationSourceEngagement(
  id: string,
  signal?: AbortSignal,
): Promise<DonationConnector> {
  return apiGet(`/api/donation-sources/${id}/engagement`, donationConnectorSchema, { signal });
}

export async function setDonationSourceEngagement(
  id: string,
  input: SetDonationEngagementInput,
): Promise<DonationConnector> {
  return apiPut(`/api/donation-sources/${id}/engagement`, input, donationConnectorSchema);
}

export async function restartDonationSourceEngagement(id: string): Promise<DonationConnector> {
  return apiPost(`/api/donation-sources/${id}/engagement/restart`, NO_BODY, donationConnectorSchema);
}
