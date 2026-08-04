import { describe, expect, it } from 'vitest';

import type { CredentialStatus } from '@/api/credential-schemas';

import { presentCredentialStatus } from './credential-presentation';

const configured: CredentialStatus = {
  streamKey: { configured: true },
  store: { available: true },
};

const missing: CredentialStatus = {
  streamKey: { configured: false },
  store: { available: true },
};

const storeUnavailable: CredentialStatus = {
  streamKey: { configured: false },
  store: { available: false },
};

describe('presentCredentialStatus', () => {
  it('shows a checking state while loading, regardless of stale data', () => {
    const result = presentCredentialStatus(configured, true);
    expect(result).toEqual({ labelKey: 'credentials.checking', tone: 'neutral' });
  });

  it('shows an unknown state when there is no data and nothing is loading', () => {
    const result = presentCredentialStatus(undefined, false);
    expect(result.labelKey).toBe('credentials.unknown');
    expect(result.tone).toBe('warning');
  });

  it('reports the store as unavailable even when a stream key could be configured', () => {
    const result = presentCredentialStatus(storeUnavailable, false);
    expect(result.labelKey).toBe('credentials.storeUnavailable');
    expect(result.tone).toBe('warning');
  });

  it('reports "stored" only once the store is available and the key is configured', () => {
    const result = presentCredentialStatus(configured, false);
    expect(result.labelKey).toBe('credentials.stored');
    expect(result.tone).toBe('positive');
  });

  it('reports "missing" when the store is available but nothing is configured', () => {
    const result = presentCredentialStatus(missing, false);
    expect(result.labelKey).toBe('credentials.missing');
    expect(result.tone).toBe('neutral');
  });

  it('never uses wording that implies provider verification', () => {
    // "Stored" must never become "Valid", "Connected" or "Authenticated":
    // this application never checks a key against the real platform.
    for (const status of [configured, missing, storeUnavailable]) {
      for (const loading of [true, false]) {
        const result = presentCredentialStatus(status, loading);
        expect(result.labelKey.toLowerCase()).not.toMatch(/valid|connected|authenticated/);
      }
    }
  });
});
