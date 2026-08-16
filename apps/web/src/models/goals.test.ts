import { describe, expect, it } from 'vitest';

import {
  MAX_GOAL_AMOUNT_MICROS,
  MAX_GOAL_COUNT_VALUE,
  emptyGoalDraft,
  isValidCurrencyCode,
  isValidGoalCurrency,
  isValidGoalName,
  isValidGoalTarget,
  isValidGoalValue,
  isValidHexColor,
  isValidWidgetProfileFields,
  defaultWidgetProfileDraft,
} from './goals';

describe('isValidGoalName', () => {
  it('rejects an empty name', () => {
    expect(isValidGoalName('')).toBe(false);
  });
  it('accepts a normal name', () => {
    expect(isValidGoalName('Followers')).toBe(true);
  });
  it('rejects a name over 80 code points', () => {
    expect(isValidGoalName('a'.repeat(81))).toBe(false);
  });
});

describe('isValidGoalTarget', () => {
  it('rejects zero', () => {
    expect(isValidGoalTarget('followers', 0)).toBe(false);
  });
  it('rejects negative', () => {
    expect(isValidGoalTarget('followers', -1)).toBe(false);
  });
  it('accepts a normal count target', () => {
    expect(isValidGoalTarget('followers', 1000)).toBe(true);
  });
  it('rejects a count target above the bound', () => {
    expect(isValidGoalTarget('followers', MAX_GOAL_COUNT_VALUE + 1)).toBe(false);
  });
  it('accepts a donation target up to the money bound', () => {
    expect(isValidGoalTarget('donations', MAX_GOAL_AMOUNT_MICROS)).toBe(true);
  });
});

describe('isValidGoalValue', () => {
  it('accepts zero', () => {
    expect(isValidGoalValue('followers', 0)).toBe(true);
  });
  it('rejects a negative value', () => {
    expect(isValidGoalValue('followers', -1)).toBe(false);
  });
});

describe('isValidCurrencyCode', () => {
  it('accepts USD', () => {
    expect(isValidCurrencyCode('USD')).toBe(true);
  });
  it('rejects lowercase', () => {
    expect(isValidCurrencyCode('usd')).toBe(false);
  });
  it('rejects a too-short code', () => {
    expect(isValidCurrencyCode('US')).toBe(false);
  });
});

describe('isValidGoalCurrency', () => {
  it('requires a currency for donations', () => {
    expect(isValidGoalCurrency('donations', undefined)).toBe(false);
    expect(isValidGoalCurrency('donations', 'USD')).toBe(true);
  });
  it('rejects a currency on a non-monetary kind', () => {
    expect(isValidGoalCurrency('followers', 'USD')).toBe(false);
    expect(isValidGoalCurrency('followers', undefined)).toBe(true);
  });
});

describe('isValidHexColor', () => {
  it('accepts a 6-digit hex color', () => {
    expect(isValidHexColor('#ffffff')).toBe(true);
  });
  it('accepts an 8-digit hex color with alpha', () => {
    expect(isValidHexColor('#ffffff80')).toBe(true);
  });
  it('rejects a named color', () => {
    expect(isValidHexColor('purple')).toBe(false);
  });
});

describe('emptyGoalDraft', () => {
  it('defaults donations with a currency and followers without one', () => {
    expect(emptyGoalDraft('donations').currency).toBe('USD');
    expect(emptyGoalDraft('followers').currency).toBeUndefined();
  });
});

describe('isValidWidgetProfileFields', () => {
  it('accepts the default draft once named', () => {
    expect(isValidWidgetProfileFields({ ...defaultWidgetProfileDraft('goal_1'), name: 'Widget' })).toBe(true);
  });
  it('rejects the default draft while its name is still empty', () => {
    expect(isValidWidgetProfileFields(defaultWidgetProfileDraft('goal_1'))).toBe(false);
  });
  it('rejects an invalid color', () => {
    const draft = { ...defaultWidgetProfileDraft('goal_1'), name: 'Widget', backgroundColor: 'purple' };
    expect(isValidWidgetProfileFields(draft)).toBe(false);
  });
  it('rejects opacity above 1', () => {
    const draft = { ...defaultWidgetProfileDraft('goal_1'), name: 'Widget', opacity: 1.5 };
    expect(isValidWidgetProfileFields(draft)).toBe(false);
  });
});
