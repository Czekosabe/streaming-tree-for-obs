import { useEffect, useState } from 'react';

import { publicWidgetSnapshotSchema, type PublicWidgetSnapshot } from '@/api/goals-schemas';

export type WidgetStreamStatus = 'connecting' | 'open' | 'error' | 'closed';

export type UseWidgetStreamResult = {
  snapshot: PublicWidgetSnapshot | null;
  status: WidgetStreamStatus;
};

function resolveStreamUrl(publicSlug: string): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  const path = `/api/public/widgets/${publicSlug}/stream`;
  return base === '' ? path : `${base.replace(/\/$/, '')}${path}`;
}

function parseJSON(rawEvent: MessageEvent<string>): unknown {
  try {
    return JSON.parse(rawEvent.data);
  } catch {
    return undefined;
  }
}

/**
 * Consumes the Stage 18A public goal-widget SSE stream (docs/goals-
 * widgets.md §19-§20). Only one event type ever arrives -
 * `widget.reset`, always the full current snapshot - mirroring
 * `useAudioStream`/`useAlertStream`'s own identical "one hook per
 * public stream" shape, simplified because there is no delta sequence,
 * no gap event, and no renderer-session handshake to track here. The
 * browser's own native `EventSource` reconnect handles a dropped
 * connection; the server always answers a fresh connection with an
 * immediate reset (never a hard error, even for an unknown/disabled
 * slug), so no manual reconnect loop is needed here either.
 */
export function useWidgetStream(publicSlug: string | undefined): UseWidgetStreamResult {
  const [snapshot, setSnapshot] = useState<PublicWidgetSnapshot | null>(null);
  const [status, setStatus] = useState<WidgetStreamStatus>('connecting');

  useEffect(() => {
    if (publicSlug === undefined || publicSlug === '') {
      setStatus('closed');
      return;
    }

    setStatus('connecting');
    setSnapshot(null);

    const source = new EventSource(resolveStreamUrl(publicSlug));

    source.addEventListener('open', () => setStatus('open'));
    source.addEventListener('error', () => setStatus('error'));

    source.addEventListener('widget.reset', (rawEvent: MessageEvent<string>) => {
      const parsed = publicWidgetSnapshotSchema.safeParse(parseJSON(rawEvent));
      if (!parsed.success) return;
      setSnapshot(parsed.data);
    });

    return () => {
      source.close();
      setStatus('closed');
      setSnapshot(null);
    };
  }, [publicSlug]);

  return { snapshot, status };
}
