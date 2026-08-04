import { describe, expect, it } from 'vitest';

import { validateServerUrlDraft } from './output-validation';

describe('server URL draft validation', () => {
  it('accepts an empty value as "not configured"', () => {
    const result = validateServerUrlDraft('');
    expect(result.valid).toBe(true);
    expect(result.violation).toBeNull();
    expect(result.serverUrl).toBe('');
  });

  it('accepts a whitespace-only value as empty', () => {
    const result = validateServerUrlDraft('   ');
    expect(result.valid).toBe(true);
    expect(result.serverUrl).toBe('');
  });

  it('accepts a valid rtmp:// address', () => {
    const result = validateServerUrlDraft('  rtmp://live.example.invalid/app  ');
    expect(result.valid).toBe(true);
    expect(result.serverUrl).toBe('rtmp://live.example.invalid/app');
  });

  it('accepts a valid rtmps:// address', () => {
    const result = validateServerUrlDraft('rtmps://live.example.invalid:443/app');
    expect(result.valid).toBe(true);
  });

  it('rejects an unsupported scheme', () => {
    const result = validateServerUrlDraft('https://example.invalid/app');
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('server-url-invalid-scheme');
  });

  it('rejects a value that is not a URL at all', () => {
    const result = validateServerUrlDraft('not a url');
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('server-url-invalid-scheme');
  });

  it('rejects a URL with no host', () => {
    const result = validateServerUrlDraft('rtmp:///app');
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('server-url-missing-host');
  });

  it('never confuses a server address with a stream key field', () => {
    // Purely structural: the validation result type has no field that could
    // carry a secret.
    const result = validateServerUrlDraft('rtmp://example.invalid/app');
    expect(result).not.toHaveProperty('streamKey');
  });
});
