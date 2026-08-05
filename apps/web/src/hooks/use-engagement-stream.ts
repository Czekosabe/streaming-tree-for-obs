import { useEffect, useRef, useState } from 'react';

import { engagementEventSchema, type EngagementEvent } from '@/api/engagement-schemas';

/**
 * Focused Server-Sent Events client for the Engagement Event Bus
 * (`GET /api/engagement/stream`).
 *
 * Responsibilities kept deliberately small, mirroring lib/api-client.ts's
 * own "small typed wrapper" philosophy:
 *  - open exactly one EventSource for the lifetime of the hook,
 *  - validate every incoming payload with Zod before trusting it,
 *  - track the last accepted sequence and ignore anything not strictly
 *    greater (a duplicate or an out-of-order delivery),
 *  - detect an `engagement.gap` event and surface it rather than silently
 *    understating what may have been missed,
 *  - keep a bounded list of recent events in React state - never grow
 *    without bound,
 *  - close the connection and drop all state on unmount.
 *
 * The browser's own EventSource implementation resends `Last-Event-ID` on
 * automatic reconnect, so no manual header handling is needed here.
 */

const MAX_RETAINED_EVENTS = 200;

export type EngagementStreamStatus = 'connecting' | 'open' | 'error' | 'closed';

export type UseEngagementStreamResult = {
  events: EngagementEvent[];
  status: EngagementStreamStatus;
  /** Set once when the server signals a gap (evicted history or a dropped
   * slow-consumer connection) - the bounded local list was reset because it
   * can no longer be considered a complete recent history. */
  gapDetected: boolean;
};

function resolveStreamUrl(): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  const path = '/api/engagement/stream';
  return base === '' ? path : `${base.replace(/\/$/, '')}${path}`;
}

export function useEngagementStream(enabled = true): UseEngagementStreamResult {
  const [events, setEvents] = useState<EngagementEvent[]>([]);
  const [status, setStatus] = useState<EngagementStreamStatus>('connecting');
  const [gapDetected, setGapDetected] = useState(false);
  const lastSequenceRef = useRef(0);

  useEffect(() => {
    if (!enabled) {
      setStatus('closed');
      return;
    }

    setStatus('connecting');
    setGapDetected(false);
    lastSequenceRef.current = 0;
    setEvents([]);

    const source = new EventSource(resolveStreamUrl());

    source.addEventListener('open', () => setStatus('open'));
    source.addEventListener('error', () => setStatus('error'));

    source.addEventListener('engagement.event', (rawEvent: MessageEvent<string>) => {
      let payload: unknown;
      try {
        payload = JSON.parse(rawEvent.data);
      } catch {
        return;
      }
      const parsed = engagementEventSchema.safeParse(payload);
      if (!parsed.success) return;

      const event = parsed.data;
      if (event.sequence <= lastSequenceRef.current) {
        // Duplicate or out-of-order delivery - ignore rather than
        // re-render or double-count it.
        return;
      }
      lastSequenceRef.current = event.sequence;

      setEvents((previous) => {
        const next = [...previous, event];
        return next.length > MAX_RETAINED_EVENTS
          ? next.slice(next.length - MAX_RETAINED_EVENTS)
          : next;
      });
    });

    source.addEventListener('engagement.gap', () => {
      setGapDetected(true);
    });

    return () => {
      source.close();
      setStatus('closed');
    };
  }, [enabled]);

  return { events, status, gapDetected };
}
