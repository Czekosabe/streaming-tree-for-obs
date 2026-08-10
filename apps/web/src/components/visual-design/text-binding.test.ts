import { describe, expect, it } from 'vitest';

import type { VisualDesignTextProps } from '@/api/visualdesign-schemas';

import { platformDisplayName, resolveTextBindingValue, type AlertBindingContext } from './text-binding';

function textProps(overrides: Partial<VisualDesignTextProps> = {}): VisualDesignTextProps {
  return {
    binding: 'alert_rendered_text', missingValueBehavior: 'hide',
    fontFamily: 'system-ui', fontSize: 32, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
    textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
    outlineWidth: 0, outlineColor: '#000000',
    shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
    ...overrides,
  };
}

const baseContext: AlertBindingContext = {
  renderedText: 'Ann followed!',
  username: 'Ann',
  platformLabel: 'Twitch',
  eventTypeLabel: 'Follow',
  message: null,
  quantity: null,
  groupCount: 1,
};

describe('resolveTextBindingValue', () => {
  it('resolves alert_rendered_text', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'alert_rendered_text' }), baseContext)).toBe('Ann followed!');
  });
  it('resolves static text from staticText', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'static', staticText: 'Hello!' }), baseContext)).toBe('Hello!');
  });
  it('resolves username when present', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'username' }), baseContext)).toBe('Ann');
  });
  it('returns null for username when absent (anonymous)', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'username' }), { ...baseContext, username: null })).toBeNull();
  });
  it('resolves platform and event_type from the pre-resolved labels', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'platform' }), baseContext)).toBe('Twitch');
    expect(resolveTextBindingValue(textProps({ binding: 'event_type' }), baseContext)).toBe('Follow');
  });
  it('returns null for message when absent', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'message' }), baseContext)).toBeNull();
  });
  it('resolves message when present', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'message' }), { ...baseContext, message: 'gg' })).toBe('gg');
  });
  it('returns null for quantity when absent (e.g. follow)', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'quantity' }), baseContext)).toBeNull();
  });
  it('resolves quantity as a string when present', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'quantity' }), { ...baseContext, quantity: 250 })).toBe('250');
  });
  it('always resolves group_count, even when 1', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'group_count' }), baseContext)).toBe('1');
    expect(resolveTextBindingValue(textProps({ binding: 'group_count' }), { ...baseContext, groupCount: 3 })).toBe('3');
  });
  it('never fabricates a value - an empty static text resolves to null', () => {
    expect(resolveTextBindingValue(textProps({ binding: 'static', staticText: '' }), baseContext)).toBeNull();
  });
});

describe('platformDisplayName', () => {
  it('maps twitch to its fixed brand name', () => {
    expect(platformDisplayName('twitch')).toBe('Twitch');
  });
  it('falls back to the raw id for an unrecognized provider', () => {
    expect(platformDisplayName('mystery')).toBe('mystery');
  });
});
