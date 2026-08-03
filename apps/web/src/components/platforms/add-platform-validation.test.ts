import { describe, expect, it } from 'vitest';

import { DISPLAY_NAME_MAX_LENGTH } from '@/models/platform-constraints';

import { validateAddPlatform } from './add-platform-validation';

describe('Add Platform form validation', () => {
  it('accepts a valid draft and returns the trimmed name', () => {
    const result = validateAddPlatform({
      providerId: 'twitch',
      displayName: '  Main Twitch channel  ',
    });

    expect(result.valid).toBe(true);
    expect(result.violation).toBeNull();
    expect(result.displayName).toBe('Main Twitch channel');
  });

  it.each(['', '   ', '\t\n'])('rejects the blank display name %j', (displayName) => {
    const result = validateAddPlatform({ providerId: 'twitch', displayName });

    expect(result.valid).toBe(false);
    expect(result.violation).toBe('display-name-required');
  });

  it('rejects a display name over the limit', () => {
    const result = validateAddPlatform({
      providerId: 'twitch',
      displayName: 'a'.repeat(DISPLAY_NAME_MAX_LENGTH + 1),
    });

    expect(result.valid).toBe(false);
    expect(result.violation).toBe('display-name-too-long');
  });

  it('accepts a display name exactly at the limit', () => {
    const result = validateAddPlatform({
      providerId: 'twitch',
      displayName: 'a'.repeat(DISPLAY_NAME_MAX_LENGTH),
    });

    expect(result.valid).toBe(true);
  });

  it('measures the trimmed length, not the raw one', () => {
    const result = validateAddPlatform({
      providerId: 'twitch',
      displayName: `  ${'a'.repeat(DISPLAY_NAME_MAX_LENGTH)}  `,
    });

    expect(result.valid).toBe(true);
  });

  it('requires a provider', () => {
    const result = validateAddPlatform({ providerId: '', displayName: 'Fine' });

    expect(result.valid).toBe(false);
    expect(result.violation).toBe('provider-required');
  });

  it('allows the same provider more than once', () => {
    // Several destinations may share a provider, so nothing here inspects
    // what is already configured.
    for (const displayName of ['Main channel', 'Backup channel']) {
      expect(validateAddPlatform({ providerId: 'twitch', displayName }).valid).toBe(true);
    }
  });

  it('accepts Unicode display names', () => {
    const result = validateAddPlatform({
      providerId: 'twitch',
      displayName: 'Zażółć gęślą jaźń',
    });

    expect(result.valid).toBe(true);
    expect(result.displayName).toBe('Zażółć gęślą jaźń');
  });
});
