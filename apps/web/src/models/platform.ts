/**
 * Domain model for streaming platforms.
 *
 * The central idea of Streaming Tree is that every platform is an independent
 * branch of the same source stream. Platforms do NOT share a metadata schema:
 * each one declares which metadata fields it supports through
 * `PlatformCapabilities`, and the UI renders only the supported fields.
 *
 * Nothing in this file talks to a real platform API. The capability tables in
 * `src/data/demo-platforms.ts` are hand-written demo configurations.
 */

export const PLATFORM_IDS = ['twitch', 'youtube', 'kick', 'tiktok'] as const;
export type PlatformId = (typeof PLATFORM_IDS)[number];

export const PLATFORM_STATUSES = ['offline', 'starting', 'live', 'error'] as const;
export type PlatformStatus = (typeof PLATFORM_STATUSES)[number];

export const CONNECTION_QUALITIES = ['excellent', 'good', 'fair', 'poor', 'unknown'] as const;
export type ConnectionQuality = (typeof CONNECTION_QUALITIES)[number];

/**
 * Which metadata fields a platform exposes.
 *
 * A `false` value means the platform has no equivalent concept at all, so the
 * field must not be rendered - not merely disabled. Fields that exist but are
 * constrained differently per platform are described by `PlatformFieldLimits`
 * and `PlatformFieldOptions` instead.
 */
export type PlatformCapabilities = {
  title: boolean;
  description: boolean;
  category: boolean;
  tags: boolean;
  language: boolean;
  visibility: boolean;
  matureContent: boolean;
  dvr: boolean;
  latencyMode: boolean;
};

export type PlatformFieldLimits = {
  titleMaxLength: number;
  descriptionMaxLength: number;
  maxTags: number;
  tagMaxLength: number;
};

export type SelectOption = {
  value: string;
  label: string;
};

/**
 * Per-platform vocabulary. Platforms use different words for the same idea
 * ("Category" on Twitch, "Game" elsewhere) and offer different option sets.
 */
export type PlatformFieldOptions = {
  categoryLabel: string;
  categoryPlaceholder: string;
  visibility: readonly SelectOption[];
  latencyModes: readonly SelectOption[];
  languages: readonly SelectOption[];
};

/**
 * The metadata document held for a single platform.
 *
 * All fields are always present in the object for simplicity, but only the
 * fields enabled in `PlatformCapabilities` are rendered, validated and - in a
 * later stage - submitted to the platform API.
 */
export type PlatformMetadata = {
  title: string;
  description: string;
  category: string;
  tags: string[];
  language: string;
  visibility: string;
  matureContent: boolean;
  dvr: boolean;
  latencyMode: string;
};

/** Static, non-runtime description of a platform branch. */
export type PlatformDefinition = {
  id: PlatformId;
  name: string;
  /** Two/three letter text mark used instead of a copyrighted logo. */
  shortLabel: string;
  /** Human readable hint about where the branch would be forwarded to. */
  ingestHint: string;
  capabilities: PlatformCapabilities;
  limits: PlatformFieldLimits;
  options: PlatformFieldOptions;
};

/** Runtime state of a platform branch (demo data in this stage). */
export type PlatformRuntimeState = {
  status: PlatformStatus;
  /** `null` when the platform does not expose a viewer count, or is offline. */
  viewers: number | null;
  quality: ConnectionQuality;
  /** Short human readable explanation shown when `status === 'error'`. */
  statusDetail: string | null;
  metadata: PlatformMetadata;
};

/**
 * A platform branch as consumed by the UI: static definition plus runtime
 * state. Kept as one flat object because every card needs both halves.
 */
export type StreamPlatform = PlatformDefinition & PlatformRuntimeState;

export function isLiveLike(status: PlatformStatus): boolean {
  return status === 'live' || status === 'starting';
}

/** Human readable label for a status value. */
export const PLATFORM_STATUS_LABELS: Record<PlatformStatus, string> = {
  offline: 'Offline',
  starting: 'Starting',
  live: 'Live',
  error: 'Error',
};

export const CONNECTION_QUALITY_LABELS: Record<ConnectionQuality, string> = {
  excellent: 'Excellent',
  good: 'Good',
  fair: 'Fair',
  poor: 'Poor',
  unknown: 'Unknown',
};
