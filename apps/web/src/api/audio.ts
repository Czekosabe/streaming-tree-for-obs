/**
 * Transport for the Stage 17A TTS/audio API. No caching or React
 * concerns live here - see hooks/use-audio.ts.
 */

import { z } from 'zod';

import { apiGet, apiPost, apiPostNoContent, apiPut } from '@/lib/api-client';

import {
  audioCapabilitiesSchema,
  audioPendingItemSchema,
  audioSettingsSchema,
  audioStatusSchema,
  audioVoiceSchema,
  type AudioCapabilities,
  type AudioPendingItem,
  type AudioSettings,
  type AudioSettingsInput,
  type AudioStatus,
  type AudioVoice,
} from './audio-schemas';

const pendingListSchema = z.array(audioPendingItemSchema);
const voiceListSchema = z.array(audioVoiceSchema);

export async function fetchAudioSettings(signal?: AbortSignal): Promise<AudioSettings> {
  return apiGet('/api/audio/settings', audioSettingsSchema, { signal });
}

export async function updateAudioSettings(input: AudioSettingsInput): Promise<AudioSettings> {
  return apiPut('/api/audio/settings', input, audioSettingsSchema);
}

export async function fetchAudioCapabilities(signal?: AbortSignal): Promise<AudioCapabilities> {
  return apiGet('/api/audio/capabilities', audioCapabilitiesSchema, { signal });
}

export async function fetchAudioVoices(signal?: AbortSignal): Promise<AudioVoice[]> {
  return apiGet('/api/audio/voices', voiceListSchema, { signal });
}

export async function fetchAudioStatus(signal?: AbortSignal): Promise<AudioStatus> {
  return apiGet('/api/audio/status', audioStatusSchema, { signal });
}

export async function fetchAudioPending(signal?: AbortSignal): Promise<AudioPendingItem[]> {
  return apiGet('/api/audio/pending', pendingListSchema, { signal });
}

export async function skipAudioQueueCurrent(): Promise<AudioStatus> {
  return apiPost('/api/audio/queue/skip-current', undefined, audioStatusSchema);
}

export async function clearAudioQueue(): Promise<AudioStatus> {
  return apiPost('/api/audio/queue/clear', undefined, audioStatusSchema);
}

export async function approveAudioPendingItem(id: string): Promise<AudioStatus> {
  return apiPost(`/api/audio/pending/${id}/approve`, undefined, audioStatusSchema);
}

export async function rejectAudioPendingItem(id: string): Promise<AudioStatus> {
  return apiPost(`/api/audio/pending/${id}/reject`, undefined, audioStatusSchema);
}

export async function rotateAudioPublicSlug(): Promise<AudioSettings> {
  return apiPost('/api/audio/rotate-slug', undefined, audioSettingsSchema);
}

export async function testSpeakAudio(text: string): Promise<AudioPendingItem> {
  return apiPost('/api/audio/test-speak', { text }, audioPendingItemSchema);
}

export type AudioAckKind = 'playback_started' | 'playback_ended' | 'playback_failed';

/** POSTs one public playback acknowledgement - never surfaces an error
 * to the operator UI (there is none on this route); the caller decides
 * what, if anything, to do about a rejected/failed ack. */
export async function ackPublicAudio(
  publicSlug: string,
  token: string,
  itemId: string,
  kind: AudioAckKind,
): Promise<void> {
  await apiPostNoContent(`/api/public/audio/${publicSlug}/ack`, { token, itemId, kind });
}
