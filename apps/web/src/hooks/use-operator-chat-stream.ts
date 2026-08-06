import { useEffect, useReducer, useRef, useState } from 'react';

import { operatorChatItemSchema } from '@/api/operator-chat-schemas';
import {
  createOperatorChatState,
  operatorChatItemsInOrder,
  operatorChatReducer,
} from '@/models/operator-chat-reducer';

/**
 * Focused Server-Sent Events client for the operator-chat projection
 * (`GET /api/operator-chat/stream`), mirroring
 * hooks/use-engagement-stream.ts's own "small typed wrapper" philosophy:
 *  - open exactly one EventSource for the lifetime of the hook,
 *  - validate every incoming payload with Zod before trusting it,
 *  - fold every revision (new item or lifecycle update) into bounded,
 *    keyed-by-id state via operatorChatReducer - never a naive append,
 *  - detect an `operator-chat.gap` event and surface it honestly,
 *  - close the connection and drop all state on unmount.
 */

const DEFAULT_CAPACITY = 500;

export type OperatorChatStreamStatus = 'connecting' | 'open' | 'error' | 'closed';

export type UseOperatorChatStreamResult = {
  items: ReturnType<typeof operatorChatItemsInOrder>;
  status: OperatorChatStreamStatus;
  /** Set once when the server signals a gap (evicted history or a dropped
   * slow-consumer connection) - never cleared, since a past gap stays a
   * past gap regardless of current stream health. */
  gapDetected: boolean;
  /** Count of upserts applied since the connection opened - used by the
   * autoscroll state machine to know how many items just arrived. */
  revisionCount: number;
};

function resolveStreamUrl(): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  const path = '/api/operator-chat/stream';
  return base === '' ? path : `${base.replace(/\/$/, '')}${path}`;
}

export function useOperatorChatStream(
  enabled = true,
  capacity: number = DEFAULT_CAPACITY,
): UseOperatorChatStreamResult {
  const [state, dispatch] = useReducer(operatorChatReducer, capacity, createOperatorChatState);
  const [status, setStatus] = useState<OperatorChatStreamStatus>('connecting');
  const [gapDetected, setGapDetected] = useState(false);
  const [revisionCount, setRevisionCount] = useState(0);
  const capacityRef = useRef(capacity);
  capacityRef.current = capacity;

  useEffect(() => {
    if (!enabled) {
      setStatus('closed');
      return;
    }

    setStatus('connecting');
    setGapDetected(false);
    setRevisionCount(0);
    dispatch({ type: 'reset', items: [] });

    const source = new EventSource(resolveStreamUrl());

    source.addEventListener('open', () => setStatus('open'));
    source.addEventListener('error', () => setStatus('error'));

    source.addEventListener('operator-chat.item', (rawEvent: MessageEvent<string>) => {
      let payload: unknown;
      try {
        payload = JSON.parse(rawEvent.data);
      } catch {
        return;
      }
      const parsed = operatorChatItemSchema.safeParse(payload);
      if (!parsed.success) return;

      dispatch({ type: 'upsert', item: parsed.data });
      setRevisionCount((count) => count + 1);
    });

    source.addEventListener('operator-chat.gap', () => {
      setGapDetected(true);
    });

    return () => {
      source.close();
      setStatus('closed');
    };
  }, [enabled]);

  return { items: operatorChatItemsInOrder(state), status, gapDetected, revisionCount };
}
