import { describe, expect, it } from 'vitest';

import {
  INGEST_STATES,
  MEDIAMTX_STATES,
  RUNTIME_SCHEMA_VERSION,
  type IngestState,
  type MediaMtxState,
  type RuntimeSnapshot,
} from '@/api/runtime-schemas';

import {
  controlsFor,
  ingestStateKey,
  ingestTone,
  mediaMtxStateKey,
  mediaMtxTone,
  runtimeErrorKey,
  summarizeSystem,
} from './runtime-presentation';

function snapshotWith(
  mediaMtxState: MediaMtxState,
  ingestState: IngestState = 'unavailable',
): RuntimeSnapshot {
  return {
    version: RUNTIME_SCHEMA_VERSION,
    mediaMtx: {
      supportedVersion: 'v1.19.3',
      source: 'managed',
      state: mediaMtxState,
      autoStart: true,
      autoRestart: true,
      restartCount: 0,
      lastError: null,
    },
    ingest: { state: ingestState, path: 'live', trackCount: null, tracks: [] },
    connection: {
      serverUrl: 'rtmp://127.0.0.1:1935',
      streamKey: 'live',
      publishUrl: 'rtmp://127.0.0.1:1935/live',
    },
  };
}

describe('state labels', () => {
  it.each(MEDIAMTX_STATES)('maps the MediaMTX state %s to a key', (state) => {
    const key = mediaMtxStateKey(state);
    expect(key).toBe(`mediamtx.state.${state}`);
  });

  it.each(INGEST_STATES)('maps the ingest state %s to a key', (state) => {
    expect(ingestStateKey(state)).toBe(`ingest.state.${state}`);
  });
});

describe('status tone', () => {
  it('treats only a ready service as live', () => {
    expect(mediaMtxTone('ready')).toBe('live');
    for (const state of ['missing', 'stopped'] as const) {
      expect(mediaMtxTone(state)).toBe('offline');
    }
    for (const state of ['error', 'incompatible'] as const) {
      expect(mediaMtxTone(state)).toBe('error');
    }
  });

  it('treats only an actual stream as live ingest', () => {
    expect(ingestTone('receiving')).toBe('live');
    expect(ingestTone('waiting')).toBe('starting');
    expect(ingestTone('unavailable')).toBe('offline');
    expect(ingestTone('error')).toBe('error');
  });
});

describe('control availability', () => {
  it('offers only installation when MediaMTX is missing', () => {
    const controls = controlsFor('missing');

    expect(controls).toEqual({
      canInstall: true,
      canStart: false,
      canStop: false,
      canRestart: false,
    });
  });

  it('offers reinstalling for an unsupported version, but not starting it', () => {
    const controls = controlsFor('incompatible');

    // Starting an unsupported build is exactly what must not happen.
    expect(controls.canStart).toBe(false);
    expect(controls.canInstall).toBe(true);
  });

  it('offers nothing while installing', () => {
    expect(controlsFor('installing')).toEqual({
      canInstall: false,
      canStart: false,
      canStop: false,
      canRestart: false,
    });
  });

  it('offers start when stopped and stop plus restart when ready', () => {
    expect(controlsFor('stopped').canStart).toBe(true);
    expect(controlsFor('stopped').canStop).toBe(false);

    expect(controlsFor('ready').canStop).toBe(true);
    expect(controlsFor('ready').canRestart).toBe(true);
    expect(controlsFor('ready').canStart).toBe(false);
  });

  it('lets a hanging start be abandoned', () => {
    expect(controlsFor('starting').canStop).toBe(true);
    expect(controlsFor('starting').canStart).toBe(false);
  });

  it('offers nothing while stopping', () => {
    expect(controlsFor('stopping')).toEqual({
      canInstall: false,
      canStart: false,
      canStop: false,
      canRestart: false,
    });
  });

  it.each(MEDIAMTX_STATES)('never enables both start and stop in state %s', (state) => {
    const controls = controlsFor(state);
    expect(controls.canStart && controls.canStop).toBe(false);
  });
});

describe('system summary', () => {
  it('reports checking while loading', () => {
    expect(summarizeSystem(undefined, true, true).labelKey).toBe('system.checking');
  });

  it('reports an unreachable backend', () => {
    const summary = summarizeSystem(undefined, false, false);

    expect(summary.tone).toBe('error');
    expect(summary.labelKey).toBe('system.backendUnavailable');
  });

  it('never calls the system operational while the ingest service is missing', () => {
    const summary = summarizeSystem(snapshotWith('missing'), true, false);

    expect(summary.labelKey).toBe('system.ingestNotInstalled');
    expect(summary.tone).not.toBe('live');
  });

  it.each(['error', 'incompatible'] as const)(
    'reports a failed component in state %s',
    (state) => {
      const summary = summarizeSystem(snapshotWith(state), true, false);

      expect(summary.tone).toBe('error');
      expect(summary.labelKey).not.toBe('system.receiving');
    },
  );

  it('reports waiting for input once ready with no publisher', () => {
    const summary = summarizeSystem(snapshotWith('ready', 'waiting'), true, false);

    expect(summary.labelKey).toBe('system.waitingForInput');
    // Running is not the same as receiving.
    expect(summary.tone).not.toBe('live');
  });

  it('reports receiving only when a stream is actually arriving', () => {
    const summary = summarizeSystem(snapshotWith('ready', 'receiving'), true, false);

    expect(summary.tone).toBe('live');
    expect(summary.labelKey).toBe('system.receiving');
  });

  it('reports a degraded component when the path status cannot be read', () => {
    const summary = summarizeSystem(snapshotWith('ready', 'error'), true, false);

    expect(summary.tone).toBe('error');
    expect(summary.labelKey).toBe('system.ingestDegraded');
  });

  it.each(MEDIAMTX_STATES)('produces a summary for every state (%s)', (state) => {
    const summary = summarizeSystem(snapshotWith(state), true, false);

    expect(summary.labelKey).not.toBe('');
    expect(['live', 'starting', 'error', 'offline']).toContain(summary.tone);
  });
});

describe('runtime error codes', () => {
  it.each([
    'mediamtx_not_installed',
    'mediamtx_incompatible_version',
    'mediamtx_unsupported_platform',
    'mediamtx_checksum_mismatch',
    'mediamtx_download_failed',
    'mediamtx_archive_invalid',
    'mediamtx_install_in_progress',
    'mediamtx_permission_denied',
    'mediamtx_port_in_use',
    'mediamtx_readiness_timeout',
    'mediamtx_exited_unexpectedly',
    'mediamtx_restart_limit_reached',
    'mediamtx_api_unreachable',
  ])('localizes the known code %s', (code) => {
    expect(runtimeErrorKey(code)).not.toBeNull();
  });

  it('returns null for a code this build does not know', () => {
    // The caller then shows the backend's English message, so the user still
    // sees a sentence rather than an identifier.
    expect(runtimeErrorKey('mediamtx_invented_later')).toBeNull();
  });

  it('never throws on hostile input', () => {
    for (const code of ['', '__proto__', 'constructor', 'toString']) {
      expect(() => runtimeErrorKey(code)).not.toThrow();
      expect(runtimeErrorKey(code)).toBeNull();
    }
  });
});
