import { describe, expect, it } from 'vitest';

import {
  INGEST_STATES,
  MEDIAMTX_STATES,
  RUNTIME_SCHEMA_VERSION,
  runtimeSnapshotSchema,
} from './runtime-schemas';

/** A snapshot shaped exactly like the backend sends one. */
function snapshot(overrides: Record<string, unknown> = {}) {
  return {
    version: RUNTIME_SCHEMA_VERSION,
    mediaMtx: {
      supportedVersion: 'v1.19.3',
      installedVersion: 'v1.19.3',
      source: 'managed',
      state: 'ready',
      autoStart: true,
      autoRestart: true,
      startedAt: '2026-08-03T12:00:00Z',
      restartCount: 0,
      lastError: null,
    },
    ingest: {
      state: 'waiting',
      path: 'live',
      trackCount: null,
      tracks: [],
    },
    connection: {
      serverUrl: 'rtmp://127.0.0.1:1935',
      streamKey: 'live',
      publishUrl: 'rtmp://127.0.0.1:1935/live',
    },
    ...overrides,
  };
}

describe('runtime snapshot parsing', () => {
  it('accepts a well formed snapshot', () => {
    const result = runtimeSnapshotSchema.safeParse(snapshot());

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.mediaMtx.state).toBe('ready');
      expect(result.data.connection.publishUrl).toBe('rtmp://127.0.0.1:1935/live');
    }
  });

  it.each(MEDIAMTX_STATES)('accepts the MediaMTX state %s', (state) => {
    const result = runtimeSnapshotSchema.safeParse(
      snapshot({ mediaMtx: { ...snapshot().mediaMtx, state } }),
    );

    expect(result.success).toBe(true);
  });

  it.each(INGEST_STATES)('accepts the ingest state %s', (state) => {
    const result = runtimeSnapshotSchema.safeParse(
      snapshot({ ingest: { ...snapshot().ingest, state } }),
    );

    expect(result.success).toBe(true);
  });

  it('accepts a snapshot with no installed version, as when MediaMTX is missing', () => {
    const result = runtimeSnapshotSchema.safeParse(
      snapshot({
        mediaMtx: {
          supportedVersion: 'v1.19.3',
          source: 'missing',
          state: 'missing',
          autoStart: true,
          autoRestart: true,
          restartCount: 0,
          lastError: { code: 'mediamtx_not_installed', message: 'Not installed yet.' },
        },
      }),
    );

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.mediaMtx.installedVersion).toBeUndefined();
      expect(result.data.mediaMtx.lastError?.code).toBe('mediamtx_not_installed');
    }
  });

  it('accepts a receiving snapshot with track details', () => {
    const result = runtimeSnapshotSchema.safeParse(
      snapshot({
        ingest: {
          state: 'receiving',
          path: 'live',
          sourceType: 'rtmpConn',
          connectedAt: '2026-08-03T12:00:00Z',
          trackCount: 2,
          tracks: ['H264', 'MPEG-4 Audio'],
        },
      }),
    );

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.ingest.trackCount).toBe(2);
      expect(result.data.ingest.tracks).toEqual(['H264', 'MPEG-4 Audio']);
    }
  });

  it('tolerates unknown future fields', () => {
    // A newer backend adding a field must not blank the panel.
    const result = runtimeSnapshotSchema.safeParse(
      snapshot({
        somethingNew: { nested: true },
        mediaMtx: { ...snapshot().mediaMtx, futureField: 'x' },
      }),
    );

    expect(result.success).toBe(true);
  });

  it('rejects an unknown MediaMTX state', () => {
    // Every state drives the interface, so an unrecognised one is a contract
    // violation rather than something to degrade past.
    const result = runtimeSnapshotSchema.safeParse(
      snapshot({ mediaMtx: { ...snapshot().mediaMtx, state: 'teleporting' } }),
    );

    expect(result.success).toBe(false);
  });

  it('rejects a snapshot missing the lastError field', () => {
    const { lastError: _omitted, ...withoutLastError } = snapshot().mediaMtx;
    const result = runtimeSnapshotSchema.safeParse(snapshot({ mediaMtx: withoutLastError }));

    // Explicitly nullable, so "no error" is distinguishable from "field absent".
    expect(result.success).toBe(false);
  });

  it('rejects a malformed snapshot', () => {
    for (const broken of [
      snapshot({ connection: { serverUrl: '', streamKey: 'live', publishUrl: 'x' } }),
      snapshot({ ingest: { ...snapshot().ingest, tracks: 'H264' } }),
      snapshot({ mediaMtx: { ...snapshot().mediaMtx, restartCount: -1 } }),
      snapshot({ version: 'one' }),
      {},
      null,
    ]) {
      expect(runtimeSnapshotSchema.safeParse(broken).success).toBe(false);
    }
  });
});
