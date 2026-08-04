import type { ParseKeys } from 'i18next';

import type {
  IngestState,
  MediaMtxState,
  RuntimeSnapshot,
} from '@/api/runtime-schemas';
import type { PlatformStatus } from './platform';

/**
 * Maps runtime state onto presentation: labels, status tone and which controls
 * are usable.
 *
 * Pure and exhaustive, so the rules can be tested without rendering and a new
 * state cannot be forgotten.
 */

export type RuntimeKey = ParseKeys<'runtime'>;

/** Which runtime controls the current state allows. */
export type RuntimeControls = {
  canInstall: boolean;
  canStart: boolean;
  canStop: boolean;
  canRestart: boolean;
};

export function controlsFor(state: MediaMtxState): RuntimeControls {
  switch (state) {
    case 'missing':
      // Nothing to run yet; installing is the only way forward.
      return { canInstall: true, canStart: false, canStop: false, canRestart: false };
    case 'incompatible':
      // Reinstalling the supported version is the fix.
      return { canInstall: true, canStart: false, canStop: false, canRestart: false };
    case 'installing':
      // A download is in flight; nothing else may run.
      return { canInstall: false, canStart: false, canStop: false, canRestart: false };
    case 'stopped':
      return { canInstall: true, canStart: true, canStop: false, canRestart: false };
    case 'error':
      // A failed start can be retried, and a reinstall may be the real fix.
      return { canInstall: true, canStart: true, canStop: false, canRestart: false };
    case 'starting':
      // Stop is allowed so a hanging start can be abandoned.
      return { canInstall: false, canStart: false, canStop: true, canRestart: false };
    case 'ready':
      return { canInstall: false, canStart: false, canStop: true, canRestart: true };
    case 'stopping':
      return { canInstall: false, canStart: false, canStop: false, canRestart: false };
  }
}

/** Translation key for the MediaMTX process state. */
export function mediaMtxStateKey(state: MediaMtxState): RuntimeKey {
  const keys: Record<MediaMtxState, RuntimeKey> = {
    missing: 'mediamtx.state.missing',
    installing: 'mediamtx.state.installing',
    incompatible: 'mediamtx.state.incompatible',
    stopped: 'mediamtx.state.stopped',
    starting: 'mediamtx.state.starting',
    ready: 'mediamtx.state.ready',
    stopping: 'mediamtx.state.stopping',
    error: 'mediamtx.state.error',
  };
  return keys[state];
}

/** Translation key for the ingest state. */
export function ingestStateKey(state: IngestState): RuntimeKey {
  const keys: Record<IngestState, RuntimeKey> = {
    unavailable: 'ingest.state.unavailable',
    waiting: 'ingest.state.waiting',
    receiving: 'ingest.state.receiving',
    error: 'ingest.state.error',
  };
  return keys[state];
}

/**
 * Status tone for the shared badge.
 *
 * `live` is used only for "receiving an actual stream"; a merely running
 * MediaMTX is not a live transmission.
 */
export function mediaMtxTone(state: MediaMtxState): PlatformStatus {
  switch (state) {
    case 'ready':
      return 'live';
    case 'starting':
    case 'installing':
    case 'stopping':
      return 'starting';
    case 'error':
    case 'incompatible':
      return 'error';
    case 'missing':
    case 'stopped':
      return 'offline';
  }
}

export function ingestTone(state: IngestState): PlatformStatus {
  switch (state) {
    case 'receiving':
      return 'live';
    case 'waiting':
      return 'starting';
    case 'error':
      return 'error';
    case 'unavailable':
      return 'offline';
  }
}

/**
 * One overall system verdict.
 *
 * The system is never called operational while the MediaMTX component is
 * missing or failed: a dashboard that says "all good" with no ingest service is
 * worse than no summary at all.
 */
export type SystemSummary = {
  tone: PlatformStatus;
  labelKey: RuntimeKey;
};

export function summarizeSystem(
  snapshot: RuntimeSnapshot | undefined,
  backendReachable: boolean,
  loading: boolean,
): SystemSummary {
  if (loading) {
    return { tone: 'starting', labelKey: 'system.checking' };
  }
  if (!backendReachable) {
    return { tone: 'error', labelKey: 'system.backendUnavailable' };
  }
  if (snapshot === undefined) {
    return { tone: 'error', labelKey: 'system.runtimeUnavailable' };
  }

  const { mediaMtx, ingest } = snapshot;

  switch (mediaMtx.state) {
    case 'missing':
      return { tone: 'offline', labelKey: 'system.ingestNotInstalled' };
    case 'incompatible':
      return { tone: 'error', labelKey: 'system.ingestIncompatible' };
    case 'installing':
      return { tone: 'starting', labelKey: 'system.ingestInstalling' };
    case 'error':
      return { tone: 'error', labelKey: 'system.ingestFailed' };
    case 'stopped':
      return { tone: 'offline', labelKey: 'system.ingestStopped' };
    case 'starting':
    case 'stopping':
      return { tone: 'starting', labelKey: 'system.ingestStarting' };
    case 'ready':
      break;
  }

  if (ingest.state === 'receiving') {
    return { tone: 'live', labelKey: 'system.receiving' };
  }
  if (ingest.state === 'error') {
    return { tone: 'error', labelKey: 'system.ingestDegraded' };
  }
  return { tone: 'starting', labelKey: 'system.waitingForInput' };
}

/**
 * Localized message for a runtime error code.
 *
 * Returns null for a code this build does not know, so the caller falls back to
 * the English message the backend already supplied. A user must always see a
 * sentence, never an identifier.
 */
export function runtimeErrorKey(code: string): RuntimeKey | null {
  const keys: Record<string, RuntimeKey> = {
    mediamtx_not_installed: 'errors.notInstalled',
    mediamtx_incompatible_version: 'errors.incompatibleVersion',
    mediamtx_unsupported_platform: 'errors.unsupportedPlatform',
    mediamtx_checksum_mismatch: 'errors.checksumMismatch',
    mediamtx_download_failed: 'errors.downloadFailed',
    mediamtx_archive_invalid: 'errors.archiveInvalid',
    mediamtx_install_failed: 'errors.installFailed',
    mediamtx_install_in_progress: 'errors.installInProgress',
    mediamtx_permission_denied: 'errors.permissionDenied',
    mediamtx_port_in_use: 'errors.portInUse',
    mediamtx_start_failed: 'errors.startFailed',
    mediamtx_readiness_timeout: 'errors.readinessTimeout',
    mediamtx_exited_unexpectedly: 'errors.exitedUnexpectedly',
    mediamtx_restart_limit_reached: 'errors.restartLimit',
    mediamtx_api_unreachable: 'errors.apiUnreachable',
    mediamtx_invalid_state: 'errors.invalidState',
  };

  return Object.prototype.hasOwnProperty.call(keys, code) ? (keys[code] ?? null) : null;
}
