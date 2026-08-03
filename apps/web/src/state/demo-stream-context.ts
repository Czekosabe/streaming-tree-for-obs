import { createContext } from 'react';

import type { PlatformId, PlatformMetadata, StreamPlatform } from '@/models/platform';

/**
 * DEMO STATE ONLY.
 *
 * This store holds an in-memory, client-side representation of the platform
 * branches. Nothing here starts, stops or touches a real stream: "starting" a
 * platform only flips a local status value after a short delay so the UI can be
 * exercised. Real control will move to the Go backend (MediaMTX + FFmpeg) in a
 * later stage, at which point this store becomes a cache of server state.
 */
export type DemoStreamContextValue = {
  platforms: StreamPlatform[];
  /** DEMO: flips status offline -> starting -> live locally. */
  startPlatform: (id: PlatformId) => void;
  /** DEMO: flips status back to offline locally. */
  stopPlatform: (id: PlatformId) => void;
  /** Persists edited metadata into the in-memory store. */
  updateMetadata: (id: PlatformId, metadata: PlatformMetadata) => void;
};

export const DemoStreamContext = createContext<DemoStreamContextValue | null>(null);
