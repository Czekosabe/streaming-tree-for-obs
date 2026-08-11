import { useCallback, useEffect, useReducer, useState } from 'react';

import {
  publicChatOverlayItemSchema,
  publicChatOverlayRemovePayloadSchema,
  publicChatOverlayResetPayloadSchema,
} from '@/api/chat-overlay-schemas';
import {
  chatOverlayItemsInOrder,
  chatOverlayLeavingItemsInOrder,
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
 *
 * The stream's own first event is always a complete `chat-overlay.reset`
 * of the current visible set (see internal/chatoverlay.Projection.
 * Subscribe's own replay-before-live guarantee) - this hook therefore
 * never issues a separate snapshot fetch to hydrate initial state; doing
 * so would race two independently-timed reads of the same mutable
 * projection against each other for no benefit, since the reset is
 * already a strict superset of what a snapshot read would return. See
 * docs/obs-browser-source.md's own "How the retained projection is
 * restored after a reload" section for the full reasoning.
 */

export type ChatOverlayStreamStatus = 'connecting' | 'open' | 'error' | 'closed';

export type UseChatOverlayStreamResult = {
  items: ReturnType<typeof chatOverlayItemsInOrder>;
  /** Items the server has already removed for a cosmetic reason
   * (expiry/capacity eviction) but that are still mid exit-animation on
   * screen - see the reducer's own doc comment. Call `completeLeaving`
   * once an item's animation has actually finished. */
  leaving: ReturnType<typeof chatOverlayLeavingItemsInOrder>;
  completeLeaving: (id: string) => void;
  status: ChatOverlayStreamStatus;
  /** Set once when the server signals a gap - never cleared, since a past
   * gap stays a past gap regardless of current stream health. */
  gapDetected: boolean;
  /** Increments once per `chat-overlay.presentation` event received
   * (Stage 13B, docs/visual-designs.md §25) - a "your presentation
   * config is now stale" signal carrying no item content at all. The
   * caller (OverlayChatPage.tsx) refetches its own public config query
   * whenever this value changes; the item reducer's own state above is
   * completely unaffected by it. */
  presentationRevision: number;
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
  const [presentationRevision, setPresentationRevision] = useState(0);

  useEffect(() => {
    if (publicSlug === undefined || publicSlug === '') {
      setStatus('closed');
      return;
    }

    setStatus('connecting');
    setGapDetected(false);
    setPresentationRevision(0);
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
      dispatch({ type: 'remove', id: parsed.data.id, reason: parsed.data.reason });
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

    source.addEventListener('chat-overlay.presentation', () => {
      setPresentationRevision((n) => n + 1);
    });

    return () => {
      source.close();
      setStatus('closed');
      dispatch({ type: 'reset', items: [] });
    };
  }, [publicSlug]);

  const completeLeaving = useCallback((id: string) => {
    dispatch({ type: 'completeLeaving', id });
  }, []);

  return {
    items: chatOverlayItemsInOrder(state),
    leaving: chatOverlayLeavingItemsInOrder(state),
    completeLeaving,
    status,
    gapDetected,
    presentationRevision,
  };
}
