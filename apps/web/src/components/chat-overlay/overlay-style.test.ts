import { describe, expect, it } from 'vitest';

import type { PublicChatOverlayConfig } from '@/api/chat-overlay-schemas';

import { entryAnimationClassName, overlayContainerStyle, overlayItemStyle } from './overlay-style';

function baseConfig(overrides: Partial<PublicChatOverlayConfig> = {}): PublicChatOverlayConfig {
  return {
    schemaVersion: 1,
    layoutMode: 'horizontal',
    stackDirection: 'bottom_up',
    horizontalAlignment: 'left',
    showPlatformIcon: true,
    showPlatformName: false,
    showTimestamp: false,
    maxVisibleItems: 30,
    messageLifetimeSeconds: 0,
    fontFamily: 'sans_serif',
    fontSize: 16,
    fontWeight: 400,
    lineHeight: 1.4,
    textColor: '#FFFFFF',
    usernameColorMode: 'provider',
    bubbleColor: '#000000',
    bubbleOpacity: 0.45,
    borderRadius: 8,
    itemSpacing: 6,
    textOutline: true,
    textShadow: false,
    entryAnimation: 'fade',
    exitAnimation: 'fade',
    animationDurationMs: 250,
    highlightBroadcaster: true,
    highlightModerators: true,
    highlightSubscribers: false,
    highlightVips: false,
    language: 'en',
    ...overrides,
  };
}

describe('overlayContainerStyle', () => {
  it('maps a valid hex color through unchanged', () => {
    const style = overlayContainerStyle(baseConfig({ textColor: '#112233' }));
    expect(style.color).toBe('#112233');
  });

  it('falls back to a safe color for an invalid textColor value', () => {
    const style = overlayContainerStyle(baseConfig({ textColor: 'javascript:alert(1)' }));
    expect(style.color).toBe('#FFFFFF');
  });

  it('clamps an out-of-range font size', () => {
    const style = overlayContainerStyle(baseConfig({ fontSize: 999 }));
    expect(style.fontSize).toBe('64px');
  });

  it('clamps an out-of-range font weight', () => {
    const style = overlayContainerStyle(baseConfig({ fontWeight: 10000 }));
    expect(style.fontWeight).toBe(900);
  });

  it('sets the animation-duration custom property from a clamped value', () => {
    const style = overlayContainerStyle(baseConfig({ animationDurationMs: 999999 }));
    expect(style['--chat-overlay-animation-duration']).toBe('5000ms');
  });

  it('falls back to a safe font stack for every known font family', () => {
    for (const family of ['sans_serif', 'serif', 'monospace', 'rounded'] as const) {
      const style = overlayContainerStyle(baseConfig({ fontFamily: family }));
      expect(typeof style.fontFamily).toBe('string');
      expect(style.fontFamily).not.toContain('<');
    }
  });
});

describe('overlayItemStyle', () => {
  it('never produces an unbounded border radius or spacing', () => {
    const style = overlayItemStyle(baseConfig({ borderRadius: -50, itemSpacing: 99999 }));
    expect(style.borderRadius).toBe('0px');
    expect(style.marginBottom).toBe('64px');
  });

  it('mixes bubbleColor and bubbleOpacity into an rgba() background', () => {
    const style = overlayItemStyle(baseConfig({ bubbleColor: '#000000', bubbleOpacity: 0.5 }));
    expect(style.backgroundColor).toBe('rgba(0, 0, 0, 0.5)');
  });

  it('falls back to black for an invalid bubbleColor', () => {
    const style = overlayItemStyle(baseConfig({ bubbleColor: 'not-a-color', bubbleOpacity: 1 }));
    expect(style.backgroundColor).toBe('rgba(0, 0, 0, 1)');
  });
});

describe('entryAnimationClassName', () => {
  it('returns a class for a known animation', () => {
    expect(entryAnimationClassName('fade', false)).toContain('animate-chat-overlay-fade-in');
  });

  it('returns an empty string for "none"', () => {
    expect(entryAnimationClassName('none', false)).toBe('');
  });

  it('returns an empty string when reduced motion is preferred, regardless of the configured animation', () => {
    expect(entryAnimationClassName('slide_up', true)).toBe('');
    expect(entryAnimationClassName('scale', true)).toBe('');
  });
});
