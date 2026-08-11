import { describe, expect, it } from 'vitest';

import { CHAT_PREVIEW_SCENARIOS, chatPreviewScenarioItem } from './preview-scenarios';

describe('CHAT_PREVIEW_SCENARIOS', () => {
  it('has 21 scenarios, matching Stage 13B task Part 27', () => {
    expect(CHAT_PREVIEW_SCENARIOS).toHaveLength(21);
  });
});

describe('chatPreviewScenarioItem', () => {
  it('missing_avatar has no avatar url', () => {
    expect(chatPreviewScenarioItem('missing_avatar').user?.avatarUrl).toBeUndefined();
  });

  it('very_long_username produces a genuinely long username', () => {
    expect(chatPreviewScenarioItem('very_long_username').user?.displayName?.length).toBeGreaterThan(100);
  });

  it('long_message produces a genuinely long message', () => {
    expect(chatPreviewScenarioItem('long_message').message?.plainText.length).toBeGreaterThan(200);
  });

  it('no_badges has an empty badge list', () => {
    expect(chatPreviewScenarioItem('no_badges').user?.badges).toHaveLength(0);
  });

  it('multiple_badges has more than one badge', () => {
    expect(chatPreviewScenarioItem('multiple_badges').user?.badges?.length).toBeGreaterThan(1);
  });

  it('message_with_emote contains an emote fragment with a resolved image URL', () => {
    const fragments = chatPreviewScenarioItem('message_with_emote').message?.fragments ?? [];
    const emote = fragments.find((f) => f.type === 'emote');
    expect(emote?.emoteImageUrl).toMatch(/^https:\/\//);
  });

  it('message_with_mention contains a mention fragment', () => {
    const fragments = chatPreviewScenarioItem('message_with_mention').message?.fragments ?? [];
    expect(fragments.some((f) => f.type === 'mention')).toBe(true);
  });

  it('account_label_present/absent differ only in accountLabel', () => {
    expect(chatPreviewScenarioItem('account_label_present').accountLabel).toBe('Main Channel');
    expect(chatPreviewScenarioItem('account_label_absent').accountLabel).toBeUndefined();
  });

  it('activity scenarios produce activity-kind items with the matching activityType', () => {
    for (const scenario of ['follow', 'subscription', 'bits', 'raid', 'channel_point_redemption'] as const) {
      const item = chatPreviewScenarioItem(scenario);
      expect(item.kind).toBe('activity');
      expect(item.activity?.activityType).toBe(scenario);
    }
  });

  it('bits/raid/subscription_gift_batch activities carry a real quantity', () => {
    expect(chatPreviewScenarioItem('bits').activity?.quantity).toBeGreaterThan(0);
    expect(chatPreviewScenarioItem('raid').activity?.quantity).toBeGreaterThan(0);
    expect(chatPreviewScenarioItem('subscription_gift_batch').activity?.quantity).toBeGreaterThan(0);
  });

  it('deleted_placeholder is marked deleted with no message content', () => {
    const item = chatPreviewScenarioItem('deleted_placeholder');
    expect(item.deleted).toBe(true);
    expect(item.message).toBeUndefined();
  });

  it('every scenario is marked synthetic', () => {
    for (const scenario of CHAT_PREVIEW_SCENARIOS) {
      expect(chatPreviewScenarioItem(scenario).synthetic).toBe(true);
    }
  });
});
