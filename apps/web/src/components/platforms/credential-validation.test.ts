import { describe, expect, it } from 'vitest';

import { STREAM_KEY_MAX_LENGTH } from '@/models/platform-constraints';

import { validateStreamKeyDraft } from './credential-validation';

describe('stream-key draft validation', () => {
  it('accepts a plausible value and returns it trimmed', () => {
    const result = validateStreamKeyDraft('  sk_live_abc123  ');

    expect(result.valid).toBe(true);
    expect(result.violation).toBeNull();
    expect(result.streamKey).toBe('sk_live_abc123');
  });

  it('preserves internal whitespace - only surrounding whitespace is trimmed', () => {
    const result = validateStreamKeyDraft('sk live key');
    expect(result.valid).toBe(true);
    expect(result.streamKey).toBe('sk live key');
  });

  it.each(['', '   ', '\t\n'])('rejects the blank value %j', (raw) => {
    const result = validateStreamKeyDraft(raw);
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('stream-key-required');
  });

  it('rejects a value over the limit', () => {
    const result = validateStreamKeyDraft('a'.repeat(STREAM_KEY_MAX_LENGTH + 1));
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('stream-key-too-long');
  });

  it('accepts a value exactly at the limit', () => {
    const result = validateStreamKeyDraft('a'.repeat(STREAM_KEY_MAX_LENGTH));
    expect(result.valid).toBe(true);
  });

  it('rejects an embedded newline', () => {
    const result = validateStreamKeyDraft('sk_live_abc\ndef');
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('stream-key-invalid');
  });

  it('rejects an embedded carriage return', () => {
    const result = validateStreamKeyDraft('sk_live_abc\rdef');
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('stream-key-invalid');
  });

  it('rejects an embedded control character', () => {
    const result = validateStreamKeyDraft('sk_live_\x07abc');
    expect(result.valid).toBe(false);
    expect(result.violation).toBe('stream-key-invalid');
  });

  it('accepts printable Unicode', () => {
    const result = validateStreamKeyDraft('sk_ключ_密钥_🔑');
    expect(result.valid).toBe(true);
    expect(result.streamKey).toBe('sk_ключ_密钥_🔑');
  });

  it('measures the trimmed length, not the raw one', () => {
    const result = validateStreamKeyDraft(`  ${'a'.repeat(STREAM_KEY_MAX_LENGTH)}  `);
    expect(result.valid).toBe(true);
  });
});
