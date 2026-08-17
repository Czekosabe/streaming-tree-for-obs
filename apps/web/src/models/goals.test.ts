import { describe, expect, it } from 'vitest';

import {
  MAX_DASHBOARD_CHILDREN,
  MAX_EVENT_TICKER_ITEMS,
  MAX_GOAL_AMOUNT_MICROS,
  MAX_GOAL_COUNT_VALUE,
  MAX_RECENT_SUPPORTERS,
  emptyDashboardChild,
  emptyGoalDraft,
  isValidCurrencyCode,
  isValidGoalCurrency,
  isValidGoalName,
  isValidGoalTarget,
  isValidGoalValue,
  isValidHexColor,
  isValidWidgetProfileFields,
  defaultWidgetProfileDraft,
  defaultWidgetProfileDraftOfKind,
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

describe('isValidWidgetProfileFields - Stage 18B kinds', () => {
  it('accepts every kind\'s own default draft once named (and, where needed, given a valid currency/metric/children)', () => {
    for (const kind of [
      'latest_follower', 'latest_subscriber', 'latest_donation', 'recent_supporters', 'event_ticker',
    ] as const) {
      const draft = { ...defaultWidgetProfileDraftOfKind(kind), name: 'Widget' };
      expect(isValidWidgetProfileFields(draft), `kind ${kind}`).toBe(true);
    }
  });

  it('rejects a goalId on a non-goal kind', () => {
    const draft = { ...defaultWidgetProfileDraftOfKind('latest_follower'), name: 'Widget', goalId: 'goal_1' };
    expect(isValidWidgetProfileFields(draft)).toBe(false);
  });

  it('requires goalId for the goal kind', () => {
    const draft = { ...defaultWidgetProfileDraftOfKind('goal'), name: 'Widget', goalId: undefined };
    expect(isValidWidgetProfileFields(draft)).toBe(false);
  });

  it('rejects provider/account filters on the goal kind', () => {
    const draft = { ...defaultWidgetProfileDraft('goal_1'), name: 'Widget', providers: ['twitch' as const] };
    expect(isValidWidgetProfileFields(draft)).toBe(false);
  });

  it('rejects maxItems out of range for recent_supporters', () => {
    const draft = { ...defaultWidgetProfileDraftOfKind('recent_supporters'), name: 'Widget', maxItems: MAX_RECENT_SUPPORTERS + 1 };
    expect(isValidWidgetProfileFields(draft)).toBe(false);
  });

  it('allows event_ticker its own higher maxItems bound', () => {
    const draft = { ...defaultWidgetProfileDraftOfKind('event_ticker'), name: 'Widget', maxItems: MAX_EVENT_TICKER_ITEMS };
    expect(isValidWidgetProfileFields(draft)).toBe(true);
  });

  it('requires a currency for largest_donation', () => {
    const missing = { ...defaultWidgetProfileDraftOfKind('largest_donation'), name: 'Widget', currency: undefined };
    expect(isValidWidgetProfileFields(missing)).toBe(false);
    const present = { ...defaultWidgetProfileDraftOfKind('largest_donation'), name: 'Widget', currency: 'USD' };
    expect(isValidWidgetProfileFields(present)).toBe(true);
  });

  it('requires a metric for session_counter, and a currency only for support_amount', () => {
    const noMetric = { ...defaultWidgetProfileDraftOfKind('session_counter'), name: 'Widget', metric: undefined };
    expect(isValidWidgetProfileFields(noMetric)).toBe(false);
    const followsMetric = { ...defaultWidgetProfileDraftOfKind('session_counter'), name: 'Widget', metric: 'follows' as const };
    expect(isValidWidgetProfileFields(followsMetric)).toBe(true);
    const supportAmountNoCurrency = { ...defaultWidgetProfileDraftOfKind('session_counter'), name: 'Widget', metric: 'support_amount' as const, currency: undefined };
    expect(isValidWidgetProfileFields(supportAmountNoCurrency)).toBe(false);
    const supportAmountWithCurrency = { ...defaultWidgetProfileDraftOfKind('session_counter'), name: 'Widget', metric: 'support_amount' as const, currency: 'EUR' };
    expect(isValidWidgetProfileFields(supportAmountWithCurrency)).toBe(true);
  });

  it('requires 1-8 children for a dashboard, and rejects duplicates', () => {
    const empty = { ...defaultWidgetProfileDraftOfKind('dashboard'), name: 'Dashboard' };
    expect(isValidWidgetProfileFields(empty)).toBe(false);

    const oneChild = { ...empty, children: [emptyDashboardChild('widget_1')] };
    expect(isValidWidgetProfileFields(oneChild)).toBe(true);

    const duplicate = { ...empty, children: [emptyDashboardChild('widget_1'), emptyDashboardChild('widget_1')] };
    expect(isValidWidgetProfileFields(duplicate)).toBe(false);

    const tooMany = { ...empty, children: Array.from({ length: MAX_DASHBOARD_CHILDREN + 1 }, (_, i) => emptyDashboardChild(`widget_${i}`)) };
    expect(isValidWidgetProfileFields(tooMany)).toBe(false);
  });

  it('rejects a dashboard child spanning past the dashboard\'s own column count', () => {
    const draft = {
      ...defaultWidgetProfileDraftOfKind('dashboard'), name: 'Dashboard', columns: 2,
      children: [{ ...emptyDashboardChild('widget_1'), column: 2, columnSpan: 2 }],
    };
    expect(isValidWidgetProfileFields(draft)).toBe(false);
  });
});
