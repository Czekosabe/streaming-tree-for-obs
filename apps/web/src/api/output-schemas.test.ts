import { describe, expect, it } from 'vitest';

import { outputSettingsSchema } from './output-schemas';

describe('outputSettingsSchema', () => {
  it('accepts a configured server address', () => {
    const result = outputSettingsSchema.safeParse({
      serverUrl: 'rtmps://live.example.invalid/app',
      autoRestart: true,
      updatedAt: '2026-08-04T12:00:00Z',
    });
    expect(result.success).toBe(true);
  });

  it('accepts an empty (not configured) server address', () => {
    const result = outputSettingsSchema.safeParse({
      serverUrl: '',
      autoRestart: true,
      updatedAt: '',
    });
    expect(result.success).toBe(true);
  });

  it('rejects a missing autoRestart field', () => {
    const result = outputSettingsSchema.safeParse({ serverUrl: '', updatedAt: '' });
    expect(result.success).toBe(false);
  });

  it('rejects malformed input', () => {
    expect(outputSettingsSchema.safeParse(null).success).toBe(false);
    expect(outputSettingsSchema.safeParse('rtmp://x').success).toBe(false);
  });

  it('never has a field for the stream key', () => {
    const parsed = outputSettingsSchema.parse({
      serverUrl: 'rtmp://example.invalid/app',
      autoRestart: true,
      updatedAt: '',
    });
    expect(parsed).not.toHaveProperty('streamKey');
  });
});
