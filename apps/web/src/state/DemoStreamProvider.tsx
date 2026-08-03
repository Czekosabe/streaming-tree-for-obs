import { useCallback, useEffect, useMemo, useReducer, useRef, type ReactNode } from 'react';

import { createDemoPlatforms } from '@/data/demo-platforms';
import type { PlatformId, PlatformMetadata, StreamPlatform } from '@/models/platform';

import { DemoStreamContext, type DemoStreamContextValue } from './demo-stream-context';

/**
 * DEMO provider - see `demo-stream-context.ts` for the full disclaimer.
 *
 * The only "behaviour" implemented here is an artificial `starting -> live`
 * transition after a fixed delay, so that the three-state button and the status
 * counters can be reviewed. No process is spawned and no network call is made.
 */

/** How long a DEMO branch stays in the `starting` state. */
const DEMO_START_DELAY_MS = 1_800;

/** Fixed viewer counts so the demo never looks like a live measurement. */
const DEMO_VIEWER_COUNTS: Record<PlatformId, number> = {
  twitch: 1284,
  youtube: 342,
  kick: 96,
  tiktok: 51,
};

type Action =
  | { type: 'start'; id: PlatformId }
  | { type: 'markLive'; id: PlatformId }
  | { type: 'stop'; id: PlatformId }
  | { type: 'updateMetadata'; id: PlatformId; metadata: PlatformMetadata };

function reducer(state: StreamPlatform[], action: Action): StreamPlatform[] {
  return state.map((platform) => {
    if (platform.id !== action.id) return platform;

    switch (action.type) {
      case 'start':
        return {
          ...platform,
          status: 'starting',
          statusDetail: null,
          viewers: null,
          quality: 'good',
        };
      case 'markLive':
        // Guard against a stop that happened while the timer was pending.
        if (platform.status !== 'starting') return platform;
        return {
          ...platform,
          status: 'live',
          viewers: DEMO_VIEWER_COUNTS[platform.id],
          quality: 'excellent',
        };
      case 'stop':
        return {
          ...platform,
          status: 'offline',
          viewers: null,
          quality: 'unknown',
          statusDetail: null,
        };
      case 'updateMetadata':
        return { ...platform, metadata: action.metadata };
      default:
        return platform;
    }
  });
}

export function DemoStreamProvider({ children }: { children: ReactNode }) {
  const [platforms, dispatch] = useReducer(reducer, undefined, createDemoPlatforms);
  const timers = useRef(new Map<PlatformId, number>());

  // Clear any pending demo timers when the provider unmounts.
  useEffect(() => {
    const pending = timers.current;
    return () => {
      for (const timerId of pending.values()) {
        window.clearTimeout(timerId);
      }
      pending.clear();
    };
  }, []);

  const clearTimer = useCallback((id: PlatformId) => {
    const timerId = timers.current.get(id);
    if (timerId !== undefined) {
      window.clearTimeout(timerId);
      timers.current.delete(id);
    }
  }, []);

  const startPlatform = useCallback(
    (id: PlatformId) => {
      clearTimer(id);
      dispatch({ type: 'start', id });
      const timerId = window.setTimeout(() => {
        timers.current.delete(id);
        dispatch({ type: 'markLive', id });
      }, DEMO_START_DELAY_MS);
      timers.current.set(id, timerId);
    },
    [clearTimer],
  );

  const stopPlatform = useCallback(
    (id: PlatformId) => {
      clearTimer(id);
      dispatch({ type: 'stop', id });
    },
    [clearTimer],
  );

  const updateMetadata = useCallback((id: PlatformId, metadata: PlatformMetadata) => {
    dispatch({ type: 'updateMetadata', id, metadata });
  }, []);

  const value = useMemo<DemoStreamContextValue>(
    () => ({ platforms, startPlatform, stopPlatform, updateMetadata }),
    [platforms, startPlatform, stopPlatform, updateMetadata],
  );

  return <DemoStreamContext.Provider value={value}>{children}</DemoStreamContext.Provider>;
}
