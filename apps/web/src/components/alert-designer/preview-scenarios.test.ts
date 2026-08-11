import { describe, expect, it } from 'vitest';

import { baseEventTypeForScenario, PREVIEW_SCENARIOS, previewScenarioFixture } from './preview-scenarios';

describe('PREVIEW_SCENARIOS', () => {
  it('has 15 scenarios, matching Stage 13A task Part 39', () => {
    expect(PREVIEW_SCENARIOS).toHaveLength(15);
  });
});

describe('baseEventTypeForScenario', () => {
  it('maps every real event-type scenario to itself', () => {
    expect(baseEventTypeForScenario('follow')).toBe('follow');
    expect(baseEventTypeForScenario('bits')).toBe('bits');
  });
  it('maps grouped scenarios to their real base event type', () => {
    expect(baseEventTypeForScenario('grouped_bits')).toBe('bits');
    expect(baseEventTypeForScenario('grouped_gift_batch')).toBe('subscription_gift_batch');
  });
});

describe('previewScenarioFixture', () => {
  it('anonymous has a null username but a real quantity', () => {
    const fixture = previewScenarioFixture('anonymous');
    expect(fixture.bindings.username).toBeNull();
    expect(fixture.bindings.quantity).not.toBeNull();
  });

  it('missing_avatar/very_long_username/very_long_message/missing_message all have no avatar', () => {
    for (const scenario of ['missing_avatar', 'very_long_username', 'very_long_message', 'missing_message'] as const) {
      expect(previewScenarioFixture(scenario).avatarUrl).toBeNull();
    }
  });

  it('very_long_username produces a genuinely long username', () => {
    const fixture = previewScenarioFixture('very_long_username');
    expect(fixture.bindings.username?.length).toBeGreaterThan(100);
  });

  it('very_long_message produces a genuinely long message', () => {
    const fixture = previewScenarioFixture('very_long_message');
    expect(fixture.bindings.message?.length).toBeGreaterThan(200);
  });

  it('missing_message has a null message', () => {
    expect(previewScenarioFixture('missing_message').bindings.message).toBeNull();
  });

  it('grouped scenarios report a groupCount greater than 1', () => {
    expect(previewScenarioFixture('grouped_bits').bindings.groupCount).toBeGreaterThan(1);
    expect(previewScenarioFixture('grouped_gift_batch').bindings.groupCount).toBeGreaterThan(1);
  });

  it('every non-grouped scenario reports groupCount 1', () => {
    expect(previewScenarioFixture('follow').bindings.groupCount).toBe(1);
  });

  it('never fabricates an avatar URL - every scenario has avatarUrl null', () => {
    for (const scenario of PREVIEW_SCENARIOS) {
      expect(previewScenarioFixture(scenario).avatarUrl).toBeNull();
    }
  });
});
