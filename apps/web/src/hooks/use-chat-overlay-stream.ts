import { useEffect, useReducer, useState } from 'react';

import {
  publicChatOverlayItemSchema,
  publicChatOverlayRemovePayloadSchema,
  publicChatOverlayResetPayloadSchema,
} from '@/api/chat-overlay-schemas';
import {
  chatOverlayItemsInOrder,
  chatOverlayReducer,
  createChatOverlayState,
} from '@/models/chat-overlay-reducer';

/**
 * Focused Server-Sent Events client for one public chat overlay
 * (`GET /api/public/chat-overlays/{publicSlug}/stream`), mirroring
 * hooks/use-operator-chat-stream.ts's own "small typed wrapper" philosophy:
 *  - open exactly one EventSource for the lifetime of the hook (per slug),
 *  - validate every incoming payload with Zod before trusting it,
 *  - fold every revision (upsert/remove/reset) into bounded,
 *    keyed-by-id state via chatOverlayReducer,
 *  - detect a `chat-overlay.gap` event and surface it honestly,
 *  - close the connection and drop all state on unmount or slug change -
 *    the public overlay never keeps chat content around longer than it
 *    needs to.
 */

export type ChatOverlayStreamStatus = 'connecting' | 'open' | 'error' | 'closed';

export type UseChatOverlayStreamResult = {
  items: ReturnType<typeof chatOverlayItemsInOrder>;
  status: ChatOverlayStreamStatus;
  /** Set once when the server signals a gap - never cleared, since a past
   * gap stays a past gap regardless of current stream health. */
  gapDetected: boolean;
};

function resolveStreamUrl(publicSlug: string): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  const path = `/api/public/chat-overlays/${publicSlug}/stream`;
  return base === '' ? path : `${base.replace(/\/$/, '')}${path}`;
}

export function useChatOverlayStream(publicSlug: string | undefined): UseChatOverlayStreamResult {
  const [state, dispatch] = useReducer(chatOverlayReducer, undefined, createChatOverlayState);
  const [status, setStatus] = useState<ChatOverlayStreamStatus>('connecting');
  const [gapDetected, setGapDetected] = useState(false);

  useEffect(() => {
    if (publicSlug === undefined || publicSlug === '') {
      setStatus('closed');
      return;
    }

    setStatus('connecting');
    setGapDetected(false);
    dispatch({ type: 'reset', items: [] });

    const source = new EventSource(resolveStreamUrl(publicSlug));

    source.addEventListener('open', () => setStatus('open'));
    source.addEventListener('error', () => setStatus('error'));

    source.addEventListener('chat-overlay.upsert', (rawEvent: MessageEvent<string>) => {
      let payload: unknown;
      try {
        payload = JSON.parse(rawEvent.data);
      } catch {
        return;
      }
      const parsed = publicChatOverlayItemSchema.safeParse(payload);
      if (!parsed.success) return;
      dispatch({ type: 'upsert', item: parsed.data });
    });

    source.addEventListener('chat-overlay.remove', (rawEvent: MessageEvent<string>) => {
      let payload: unknown;
      try {
        payload = JSON.parse(rawEvent.data);
      } catch {
        return;
      }
      const parsed = publicChatOverlayRemovePayloadSchema.safeParse(payload);
      if (!parsed.success) return;
      dispatch({ type: 'remove', id: parsed.data.id });
    });

    source.addEventListener('chat-overlay.reset', (rawEvent: MessageEvent<string>) => {
      let payload: unknown;
      try {
        payload = JSON.parse(rawEvent.data);
      } catch {
        return;
      }
      const parsed = publicChatOverlayResetPayloadSchema.safeParse(payload);
      if (!parsed.success) return;
      dispatch({ type: 'reset', items: parsed.data.items });
    });

    source.addEventListener('chat-overlay.gap', () => {
      setGapDetected(true);
    });

    return () => {
      source.close();
      setStatus('closed');
      dispatch({ type: 'reset', items: [] });
    };
  }, [publicSlug]);

  return { items: chatOverlayItemsInOrder(state), status, gapDetected };
}
