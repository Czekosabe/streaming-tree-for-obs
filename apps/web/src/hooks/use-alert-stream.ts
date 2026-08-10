import { useEffect, useReducer, useState } from 'react';

import { publicAlertRevisionPayloadSchema } from '@/api/alerts-schemas';
import { alertStreamReducer, createAlertStreamState } from '@/models/alert-stream-reducer';

/**
 * Focused Server-Sent Events client for one public alert profile
 * (`GET /api/public/alert-profiles/{publicSlug}/stream`), mirroring
 * hooks/use-chat-overlay-stream.ts's own "small typed wrapper"
 * philosophy:
 *  - open exactly one EventSource for the lifetime of the hook (per
 *    slug) - the browser's own EventSource sends `Last-Event-ID`
 *    automatically on reconnect, so no separate replay bookkeeping is
 *    needed here,
 *  - validate every incoming payload with Zod before trusting it,
 *  - fold every revision (show/hide/reset/paused) into the tiny
 *    single-current-item state via alertStreamReducer,
 *  - detect an `alert.gap` event and surface it honestly,
 *  - close the connection and drop all state on unmount or slug
 *    change - the public overlay never keeps alert content around
 *    longer than it needs to.
 *
 * The stream's own first event is always a complete `alert.reset` of
 * current state (see internal/alerts's own projection.go and
 * internal/httpapi/alerts.go's handlePublicAlertStream) - this hook
 * therefore never issues a separate snapshot fetch to hydrate initial
 * state.
 */

export type AlertStreamStatus = 'connecting' | 'open' | 'error' | 'closed';

export type UseAlertStreamResult = {
  current: ReturnType<typeof alertStreamReducer>['current'];
  paused: boolean;
  status: AlertStreamStatus;
  /** Set once when the server signals a gap - never cleared, since a
   * past gap stays a past gap regardless of current stream health. */
  gapDetected: boolean;
};

function resolveStreamUrl(publicSlug: string): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  const path = `/api/public/alert-profiles/${publicSlug}/stream`;
  return base === '' ? path : `${base.replace(/\/$/, '')}${path}`;
}

function parseRevisionPayload(rawEvent: MessageEvent<string>) {
  let payload: unknown;
  try {
    payload = JSON.parse(rawEvent.data);
  } catch {
    return undefined;
  }
  const parsed = publicAlertRevisionPayloadSchema.safeParse(payload);
  return parsed.success ? parsed.data : undefined;
}

export function useAlertStream(publicSlug: string | undefined): UseAlertStreamResult {
  const [state, dispatch] = useReducer(alertStreamReducer, undefined, createAlertStreamState);
  const [status, setStatus] = useState<AlertStreamStatus>('connecting');
  const [gapDetected, setGapDetected] = useState(false);

  useEffect(() => {
    if (publicSlug === undefined || publicSlug === '') {
      setStatus('closed');
      return;
    }

    setStatus('connecting');
    setGapDetected(false);
    dispatch({ type: 'reset', alert: null, paused: false });

    const source = new EventSource(resolveStreamUrl(publicSlug));

    source.addEventListener('open', () => setStatus('open'));
    source.addEventListener('error', () => setStatus('error'));

    source.addEventListener('alert.show', (rawEvent: MessageEvent<string>) => {
      const payload = parseRevisionPayload(rawEvent);
      if (payload?.alert === null || payload?.alert === undefined) return;
      dispatch({ type: 'show', alert: payload.alert, paused: payload.paused });
    });

    source.addEventListener('alert.hide', (rawEvent: MessageEvent<string>) => {
      const payload = parseRevisionPayload(rawEvent);
      if (payload === undefined) return;
      dispatch({ type: 'hide', paused: payload.paused });
    });

    source.addEventListener('alert.reset', (rawEvent: MessageEvent<string>) => {
      const payload = parseRevisionPayload(rawEvent);
      if (payload === undefined) return;
      dispatch({ type: 'reset', alert: payload.alert, paused: payload.paused });
    });

    source.addEventListener('alert.paused', (rawEvent: MessageEvent<string>) => {
      const payload = parseRevisionPayload(rawEvent);
      if (payload === undefined) return;
      dispatch({ type: 'paused', paused: payload.paused });
    });

    source.addEventListener('alert.gap', () => {
      setGapDetected(true);
    });

    return () => {
      source.close();
      setStatus('closed');
      dispatch({ type: 'reset', alert: null, paused: false });
    };
  }, [publicSlug]);

  return { current: state.current, paused: state.paused, status, gapDetected };
}
