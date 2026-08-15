import { useEffect, useState } from 'react';

import {
  publicAudioCurrentPayloadSchema,
  publicAudioGapPayloadSchema,
  publicAudioResetPayloadSchema,
  type PublicAudioCurrentPayload,
} from '@/api/audio-schemas';

export type AudioStreamStatus = 'connecting' | 'open' | 'error' | 'closed';

export type UseAudioStreamResult = {
  /** The current playable item, or null while idle/still synthesizing/
   * waiting for a renderer - never the future queue (docs/audio-tts.md
   * §14). */
  current: PublicAudioCurrentPayload | null;
  /** This browser tab's own ephemeral renderer session token - kept
   * only in memory, never logged, never persisted. Required on every
   * POST .../ack call. Empty until the first `audio.reset` event
   * arrives. */
  rendererToken: string;
  status: AudioStreamStatus;
  /** Set once when the server signals a gap - never cleared, mirrors
   * useAlertStream's own identical convention. */
  gapDetected: boolean;
};

function resolveStreamUrl(publicSlug: string): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  const path = `/api/public/audio/${publicSlug}/stream`;
  return base === '' ? path : `${base.replace(/\/$/, '')}${path}`;
}

function parseJSON(rawEvent: MessageEvent<string>): unknown {
  try {
    return JSON.parse(rawEvent.data);
  } catch {
    return undefined;
  }
}

export function useAudioStream(publicSlug: string | undefined): UseAudioStreamResult {
  const [current, setCurrent] = useState<PublicAudioCurrentPayload | null>(null);
  const [rendererToken, setRendererToken] = useState('');
  const [status, setStatus] = useState<AudioStreamStatus>('connecting');
  const [gapDetected, setGapDetected] = useState(false);

  useEffect(() => {
    if (publicSlug === undefined || publicSlug === '') {
      setStatus('closed');
      return;
    }

    setStatus('connecting');
    setGapDetected(false);
    setCurrent(null);
    setRendererToken('');

    const source = new EventSource(resolveStreamUrl(publicSlug));

    source.addEventListener('open', () => setStatus('open'));
    source.addEventListener('error', () => setStatus('error'));

    source.addEventListener('audio.reset', (rawEvent: MessageEvent<string>) => {
      const parsed = publicAudioResetPayloadSchema.safeParse(parseJSON(rawEvent));
      if (!parsed.success) return;
      setRendererToken(parsed.data.rendererToken);
    });

    source.addEventListener('audio.current', (rawEvent: MessageEvent<string>) => {
      const parsed = publicAudioCurrentPayloadSchema.safeParse(parseJSON(rawEvent));
      if (!parsed.success) return;
      setCurrent(parsed.data);
    });

    source.addEventListener('audio.idle', () => {
      setCurrent(null);
    });

    source.addEventListener('audio.gap', (rawEvent: MessageEvent<string>) => {
      const parsed = publicAudioGapPayloadSchema.safeParse(parseJSON(rawEvent));
      void parsed; // reason is diagnostic only - the boolean is what the renderer acts on
      setGapDetected(true);
    });

    return () => {
      source.close();
      setStatus('closed');
      setCurrent(null);
      setRendererToken('');
    };
  }, [publicSlug]);

  return { current, rendererToken, status, gapDetected };
}
