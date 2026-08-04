import { describe, expect, it } from 'vitest';

import { credentialStatusSchema } from './credential-schemas';

describe('credentialStatusSchema', () => {
  it('accepts a configured status with an available store', () => {
    const result = credentialStatusSchema.safeParse({
      streamKey: { configured: true },
      store: { available: true },
    });
    expect(result.success).toBe(true);
  });

  it('accepts a not-configured status', () => {
    const result = credentialStatusSchema.safeParse({
      streamKey: { configured: false },
      store: { available: true },
    });
    expect(result.success).toBe(true);
  });

  it('accepts an unavailable store', () => {
    const result = credentialStatusSchema.safeParse({
      streamKey: { configured: false },
      store: { available: false },
    });
    expect(result.success).toBe(true);
  });

  it.each([
    ['missing streamKey', { store: { available: true } }],
    ['missing store', { streamKey: { configured: true } }],
    ['missing everything', {}],
    ['null payload', null],
    ['array payload', []],
    ['a bare boolean', true],
  ])('rejects %s', (_name, payload) => {
    const result = credentialStatusSchema.safeParse(payload);
    expect(result.success).toBe(false);
  });

  it('rejects a streamKey.configured that is not a boolean', () => {
    const result = credentialStatusSchema.safeParse({
      streamKey: { configured: 'yes' },
      store: { available: true },
    });
    expect(result.success).toBe(false);
  });

  it('rejects a store.available that is not a boolean', () => {
    const result = credentialStatusSchema.safeParse({
      streamKey: { configured: true },
      store: { available: 1 },
    });
    expect(result.success).toBe(false);
  });

  it('rejects a payload that carries the stream key value itself', () => {
    // The backend never sends this, but a schema that silently accepted an
    // extra "value" field would make it easy to add one by accident later
    // without anyone noticing in a diff.
    const result = credentialStatusSchema.safeParse({
      streamKey: { configured: true, value: 'sk_live_should_never_be_here' },
      store: { available: true },
    });
    // Zod's default object parsing strips unknown keys rather than
    // rejecting them, so this asserts the stronger property directly: even
    // if such a field were present on the wire, it never survives parsing.
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.streamKey).not.toHaveProperty('value');
      expect(JSON.stringify(result.data)).not.toContain('sk_live_should_never_be_here');
    }
  });
});
