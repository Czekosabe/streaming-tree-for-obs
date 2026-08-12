import { describe, expect, it } from 'vitest';

import type { AlertEventTypeCapability } from '@/api/alerts-schemas';

import {
  ALERT_INTERRUPT_MODES,
  MAX_AMOUNT_MICROS,
  codePointLength,
  extractPlaceholderNames,
  formatAmountMicros,
  insertPlaceholder,
  isValidAlertName,
  isValidAlertTemplate,
  isValidAmountRange,
  isValidAnimationDurationMs,
  isValidDurationMs,
  isValidGroupWindowMs,
  isValidMaxQueueItems,
  isValidMaximumQueueAgeSeconds,
  isValidPriority,
  isValidThresholdRange,
  normalizeCurrencyCode,
  parseAmountMicros,
  unknownPlaceholderNames,
  unsupportedPlaceholderNames,
} from './alerts';

describe('codePointLength', () => {
  it('counts a single emoji as one code point', () => {
    expect(codePointLength('🎉')).toBe(1);
  });
});

describe('isValidAlertName', () => {
  it('accepts a normal name', () => expect(isValidAlertName('Main')).toBe(true));
  it('rejects an empty name', () => expect(isValidAlertName('   ')).toBe(false));
  it('rejects a name over 80 code points', () => expect(isValidAlertName('a'.repeat(81))).toBe(false));
});

describe('isValidAlertTemplate', () => {
  it('accepts a normal template', () => expect(isValidAlertTemplate('{username} followed!')).toBe(true));
  it('rejects an empty template', () => expect(isValidAlertTemplate('   ')).toBe(false));
  it('rejects a template over 500 code points', () => expect(isValidAlertTemplate('a'.repeat(501))).toBe(false));
});

describe('isValidPriority', () => {
  it('accepts the boundary values', () => {
    expect(isValidPriority(0)).toBe(true);
    expect(isValidPriority(100)).toBe(true);
  });
  it('rejects out of range', () => {
    expect(isValidPriority(-1)).toBe(false);
    expect(isValidPriority(101)).toBe(false);
  });
});

describe('isValidDurationMs', () => {
  it('accepts the boundary values', () => {
    expect(isValidDurationMs(1000)).toBe(true);
    expect(isValidDurationMs(30000)).toBe(true);
  });
  it('rejects out of range', () => {
    expect(isValidDurationMs(999)).toBe(false);
    expect(isValidDurationMs(30001)).toBe(false);
  });
});

describe('isValidGroupWindowMs', () => {
  it('accepts the boundary values', () => {
    expect(isValidGroupWindowMs(1000)).toBe(true);
    expect(isValidGroupWindowMs(30000)).toBe(true);
  });
  it('rejects out of range', () => {
    expect(isValidGroupWindowMs(999)).toBe(false);
    expect(isValidGroupWindowMs(30001)).toBe(false);
  });
  it('rejects a non-integer', () => {
    expect(isValidGroupWindowMs(5000.5)).toBe(false);
  });
});

describe('ALERT_INTERRUPT_MODES', () => {
  it('is the closed never/lower_priority pair, mirroring the backend enum', () => {
    expect(ALERT_INTERRUPT_MODES).toEqual(['never', 'lower_priority']);
  });
});

describe('isValidAnimationDurationMs', () => {
  it('accepts the boundary values', () => {
    expect(isValidAnimationDurationMs(0)).toBe(true);
    expect(isValidAnimationDurationMs(2000)).toBe(true);
  });
  it('rejects negative and over-max', () => {
    expect(isValidAnimationDurationMs(-1)).toBe(false);
    expect(isValidAnimationDurationMs(2001)).toBe(false);
  });
});

describe('isValidMaxQueueItems / isValidMaximumQueueAgeSeconds', () => {
  it('accepts documented defaults', () => {
    expect(isValidMaxQueueItems(100)).toBe(true);
    expect(isValidMaximumQueueAgeSeconds(120)).toBe(true);
  });
  it('rejects out of range', () => {
    expect(isValidMaxQueueItems(0)).toBe(false);
    expect(isValidMaxQueueItems(501)).toBe(false);
    expect(isValidMaximumQueueAgeSeconds(4)).toBe(false);
    expect(isValidMaximumQueueAgeSeconds(3601)).toBe(false);
  });
});

describe('isValidThresholdRange', () => {
  it('accepts both null (unbounded)', () => expect(isValidThresholdRange(null, null)).toBe(true));
  it('accepts one-sided bounds', () => {
    expect(isValidThresholdRange(1, null)).toBe(true);
    expect(isValidThresholdRange(null, 100)).toBe(true);
  });
  it('accepts equal bounds', () => expect(isValidThresholdRange(50, 50)).toBe(true));
  it('rejects minimum greater than maximum', () => expect(isValidThresholdRange(100, 1)).toBe(false));
  it('rejects a negative bound', () => expect(isValidThresholdRange(-1, null)).toBe(false));
});

describe('extractPlaceholderNames', () => {
  it('extracts multiple placeholders', () => {
    expect(extractPlaceholderNames('{username} gave {quantity} bits')).toEqual(['username', 'quantity']);
  });
  it('ignores escaped braces', () => {
    expect(extractPlaceholderNames('{{literal}} {username}')).toEqual(['username']);
  });
  it('handles adjacent placeholders', () => {
    expect(extractPlaceholderNames('{username}{platform}')).toEqual(['username', 'platform']);
  });
});

describe('unknownPlaceholderNames', () => {
  it('flags a name outside the closed vocabulary', () => {
    expect(unknownPlaceholderNames('{bogus}')).toEqual(['bogus']);
  });
  it('accepts every known name', () => {
    expect(
      unknownPlaceholderNames('{username}{platform}{eventType}{quantity}{message}{rewardTitle}{groupCount}'),
    ).toEqual([]);
  });
});

describe('unsupportedPlaceholderNames', () => {
  const followCapability: AlertEventTypeCapability = {
    eventType: 'follow', hasUser: true, hasMessage: false, hasQuantity: false,
    hasAnonymity: false, hasRewardTitle: false, hasRoles: false, hasAmount: false, hasMembershipLevel: false,
    availablePlaceholders: ['platform', 'eventType', 'username'],
    groupable: false, groupingRequiresHiddenMessage: false,
  };
  const bitsCapability: AlertEventTypeCapability = {
    eventType: 'bits', hasUser: true, hasMessage: true, hasQuantity: true,
    hasAnonymity: true, hasRewardTitle: false, hasRoles: false, hasAmount: false, hasMembershipLevel: false,
    availablePlaceholders: ['platform', 'eventType', 'username', 'quantity', 'message', 'groupCount'],
    groupable: true, groupingRequiresHiddenMessage: true,
  };

  it('flags {quantity} on a follow rule as unsupported (known but unavailable)', () => {
    expect(unsupportedPlaceholderNames('{quantity}', followCapability)).toEqual(['quantity']);
  });
  it('accepts {quantity} on a bits rule', () => {
    expect(unsupportedPlaceholderNames('{quantity}', bitsCapability)).toEqual([]);
  });
  it('never flags a genuinely unknown name (that is unknownPlaceholderNames own job)', () => {
    expect(unsupportedPlaceholderNames('{bogus}', followCapability)).toEqual([]);
  });
  it('returns nothing when capability is not yet loaded', () => {
    expect(unsupportedPlaceholderNames('{quantity}', undefined)).toEqual([]);
  });
});

describe('insertPlaceholder', () => {
  it('inserts at the cursor and advances it past the token', () => {
    const result = insertPlaceholder('Hello !', 6, 'username');
    expect(result.text).toBe('Hello {username}!');
    expect(result.cursor).toBe(16);
  });
});

describe('isValidAmountRange', () => {
  it('accepts both bounds unset regardless of currency', () => {
    expect(isValidAmountRange('', null, null)).toBe(true);
  });
  it('rejects a bound set with no currency', () => {
    expect(isValidAmountRange('', 1_000_000, null)).toBe(false);
    expect(isValidAmountRange('  ', null, 1_000_000)).toBe(false);
  });
  it('accepts a bound set with a currency', () => {
    expect(isValidAmountRange('USD', 1_000_000, null)).toBe(true);
  });
  it('rejects a negative bound', () => {
    expect(isValidAmountRange('USD', -1, null)).toBe(false);
  });
  it('rejects a bound over the max', () => {
    expect(isValidAmountRange('USD', null, MAX_AMOUNT_MICROS + 1)).toBe(false);
  });
  it('accepts a bound exactly at the max', () => {
    expect(isValidAmountRange('USD', MAX_AMOUNT_MICROS, null)).toBe(true);
  });
  it('rejects minimum greater than maximum', () => {
    expect(isValidAmountRange('USD', 100, 1)).toBe(false);
  });
  it('accepts minimum equal to maximum', () => {
    expect(isValidAmountRange('USD', 50, 50)).toBe(true);
  });
});

describe('normalizeCurrencyCode', () => {
  it('uppercases a lowercase code', () => expect(normalizeCurrencyCode('usd')).toBe('USD'));
  it('leaves an uppercase code unchanged', () => expect(normalizeCurrencyCode('EUR')).toBe('EUR'));
});

describe('parseAmountMicros', () => {
  it('parses a whole-major-unit string', () => expect(parseAmountMicros('5')).toBe(5_000_000));
  it('parses a two-decimal string', () => expect(parseAmountMicros('5.00')).toBe(5_000_000));
  it('parses a partial-decimal string, zero-padding the rest', () => expect(parseAmountMicros('1.5')).toBe(1_500_000));
  it('parses a full six-digit fraction', () => expect(parseAmountMicros('0.000999')).toBe(999));
  it('returns null for an empty string', () => expect(parseAmountMicros('')).toBeNull());
  it('returns null for a non-numeric string', () => expect(parseAmountMicros('abc')).toBeNull());
  it('returns null for a negative string (never a valid amount)', () => expect(parseAmountMicros('-5')).toBeNull());
  it('returns null for more than 6 fraction digits', () => expect(parseAmountMicros('1.1234567')).toBeNull());
  it('is the exact inverse of formatAmountMicros for round-trippable values', () => {
    for (const micros of [0, 999, 5_000_000, 1_500_000, 1_000_001]) {
      expect(parseAmountMicros(formatAmountMicros(micros))).toBe(micros);
    }
  });
});

describe('formatAmountMicros', () => {
  it('formats a whole amount with a trailing .00', () => expect(formatAmountMicros(5_000_000)).toBe('5.00'));
  it('formats a half-unit amount', () => expect(formatAmountMicros(1_500_000)).toBe('1.50'));
  it('formats zero', () => expect(formatAmountMicros(0)).toBe('0.00'));
  it('keeps a sub-cent fraction fully visible (never silently rounded)', () => {
    expect(formatAmountMicros(999)).toBe('0.000999');
  });
  it('keeps a non-round fraction fully visible', () => {
    expect(formatAmountMicros(1_000_001)).toBe('1.000001');
  });
});
