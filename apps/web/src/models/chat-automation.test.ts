import { describe, expect, it } from 'vitest';

import {
  MAX_COMMAND_NAME_LENGTH,
  MAX_NAME_CODE_POINTS,
  MAX_TEMPLATE_CODE_POINTS,
  codePointLength,
  extractPlaceholderNames,
  insertPlaceholder,
  isValidCommandName,
  isValidCooldownSeconds,
  isValidFirstDelaySeconds,
  isValidIntervalSeconds,
  isValidJitterSeconds,
  isValidMaximumSendsPerHour,
  isValidMinimumChatMessages,
  isValidScheduleName,
  isValidTemplate,
  normalizeCommandName,
  unknownPlaceholderNames,
} from './chat-automation';

describe('codePointLength', () => {
  it('counts a surrogate-pair emoji as one code point, matching the backend', () => {
    expect('🎉'.length).toBe(2);
    expect(codePointLength('🎉')).toBe(1);
  });
});

describe('isValidScheduleName', () => {
  it('rejects empty and over-long names', () => {
    expect(isValidScheduleName('')).toBe(false);
    expect(isValidScheduleName('  ')).toBe(false);
    expect(isValidScheduleName('a'.repeat(MAX_NAME_CODE_POINTS + 1))).toBe(false);
  });
  it('accepts a normal name', () => {
    expect(isValidScheduleName('Hourly reminder')).toBe(true);
  });
});

describe('isValidTemplate', () => {
  it('rejects blank templates and templates over the limit', () => {
    expect(isValidTemplate('')).toBe(false);
    expect(isValidTemplate('   ')).toBe(false);
    expect(isValidTemplate('a'.repeat(MAX_TEMPLATE_CODE_POINTS + 1))).toBe(false);
  });
  it('accepts a template at exactly the boundary', () => {
    expect(isValidTemplate('a'.repeat(MAX_TEMPLATE_CODE_POINTS))).toBe(true);
  });
});

describe('schedule timing validators', () => {
  it('interval must be 60s-24h', () => {
    expect(isValidIntervalSeconds(59)).toBe(false);
    expect(isValidIntervalSeconds(60)).toBe(true);
    expect(isValidIntervalSeconds(86400)).toBe(true);
    expect(isValidIntervalSeconds(86401)).toBe(false);
  });
  it('first delay must be 0-24h', () => {
    expect(isValidFirstDelaySeconds(-1)).toBe(false);
    expect(isValidFirstDelaySeconds(0)).toBe(true);
    expect(isValidFirstDelaySeconds(86401)).toBe(false);
  });
  it('jitter must be 0-15min', () => {
    expect(isValidJitterSeconds(-1)).toBe(false);
    expect(isValidJitterSeconds(900)).toBe(true);
    expect(isValidJitterSeconds(901)).toBe(false);
  });
  it('minimum chat messages must be 0-1000', () => {
    expect(isValidMinimumChatMessages(-1)).toBe(false);
    expect(isValidMinimumChatMessages(1000)).toBe(true);
    expect(isValidMinimumChatMessages(1001)).toBe(false);
  });
  it('maximum sends per hour must be 1-60', () => {
    expect(isValidMaximumSendsPerHour(0)).toBe(false);
    expect(isValidMaximumSendsPerHour(1)).toBe(true);
    expect(isValidMaximumSendsPerHour(60)).toBe(true);
    expect(isValidMaximumSendsPerHour(61)).toBe(false);
  });
});

describe('isValidCommandName', () => {
  it('accepts lowercase ascii names', () => {
    expect(isValidCommandName('discord')).toBe(true);
    expect(isValidCommandName('a-b_c9')).toBe(true);
  });
  it('rejects uppercase, punctuation, whitespace and over-long names', () => {
    expect(isValidCommandName('Discord')).toBe(false);
    expect(isValidCommandName('!discord')).toBe(false);
    expect(isValidCommandName('dis cord')).toBe(false);
    expect(isValidCommandName('')).toBe(false);
    expect(isValidCommandName('a'.repeat(MAX_COMMAND_NAME_LENGTH + 1))).toBe(false);
  });
});

describe('normalizeCommandName', () => {
  it('lowercases and trims', () => {
    expect(normalizeCommandName('  Discord  ')).toBe('discord');
  });
});

describe('isValidCooldownSeconds', () => {
  it('accepts zero and rejects negative/over-limit values', () => {
    expect(isValidCooldownSeconds(0, 0)).toBe(true);
    expect(isValidCooldownSeconds(-1, 0)).toBe(false);
    expect(isValidCooldownSeconds(0, -1)).toBe(false);
    expect(isValidCooldownSeconds(3601, 0)).toBe(false);
    expect(isValidCooldownSeconds(0, 86401)).toBe(false);
  });
});

describe('extractPlaceholderNames', () => {
  it('extracts a simple placeholder', () => {
    expect(extractPlaceholderNames('Hi {channelName}!')).toEqual(['channelName']);
  });
  it('extracts multiple adjacent placeholders', () => {
    expect(extractPlaceholderNames('{channelName}{platform}')).toEqual(['channelName', 'platform']);
  });
  it('ignores escaped braces', () => {
    expect(extractPlaceholderNames('Visit {{example}}')).toEqual([]);
  });
  it('handles unmatched braces without throwing', () => {
    expect(() => extractPlaceholderNames('hi {name')).not.toThrow();
  });
});

describe('unknownPlaceholderNames', () => {
  it('reports a name outside the known set', () => {
    expect(unknownPlaceholderNames('hi {viewerCount}')).toEqual(['viewerCount']);
  });
  it('reports nothing for every known placeholder', () => {
    expect(
      unknownPlaceholderNames('{channelName}{platform}{channelUrl}{streamTitle}{streamUptime}'),
    ).toEqual([]);
  });
});

describe('insertPlaceholder', () => {
  it('inserts at the cursor and advances it past the inserted token', () => {
    const result = insertPlaceholder('Hello !', 6, 'channelName');
    expect(result.text).toBe('Hello {channelName}!');
    expect(result.cursor).toBe(6 + '{channelName}'.length);
  });
});
