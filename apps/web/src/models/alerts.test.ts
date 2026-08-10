import { describe, expect, it } from 'vitest';

import type { AlertEventTypeCapability } from '@/api/alerts-schemas';

import {
  ALERT_INTERRUPT_MODES,
  codePointLength,
  extractPlaceholderNames,
  insertPlaceholder,
  isValidAlertName,
  isValidAlertTemplate,
  isValidAnimationDurationMs,
  isValidDurationMs,
  isValidGroupWindowMs,
  isValidMaxQueueItems,
  isValidMaximumQueueAgeSeconds,
  isValidPriority,
  isValidThresholdRange,
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
    hasAnonymity: false, hasRewardTitle: false, hasRoles: false,
    availablePlaceholders: ['platform', 'eventType', 'username'],
    groupable: false, groupingRequiresHiddenMessage: false,
  };
  const bitsCapability: AlertEventTypeCapability = {
    eventType: 'bits', hasUser: true, hasMessage: true, hasQuantity: true,
    hasAnonymity: true, hasRewardTitle: false, hasRoles: false,
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
