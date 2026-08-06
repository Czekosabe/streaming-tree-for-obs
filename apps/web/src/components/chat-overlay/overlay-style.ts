import type { CSSProperties } from 'react';

import type {
  ChatOverlayAnimation,
  ChatOverlayFontFamily,
  PublicChatOverlayConfig,
} from '@/api/chat-overlay-schemas';

/**
 * Maps a validated public overlay config onto CSS - never an arbitrary
 * string from the backend passed straight into `style`. Every value here
 * either comes from a fixed enum (font family, animation) or a number/
 * hex-color the backend has already bounded (internal/domain/chatoverlay's
 * own validation.go) - this module adds its own defensive clamping too, so
 * a future backend regression can never hand the renderer an unbounded
 * value.
 */

const FONT_STACKS: Record<ChatOverlayFontFamily, string> = {
  sans_serif: 'ui-sans-serif, system-ui, "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
  serif: 'ui-serif, Georgia, Cambria, "Times New Roman", Times, serif',
  monospace: 'ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, monospace',
  rounded: 'ui-rounded, "SF Pro Rounded", "Segoe UI", system-ui, sans-serif',
};

function clamp(value: number, min: number, max: number): number {
  if (Number.isNaN(value) || !Number.isFinite(value)) return min;
  return Math.min(max, Math.max(min, value));
}

const HEX_COLOR_PATTERN = /^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$/;

/** Falls back to a safe opaque black for anything that isn't a validated
 * `#RRGGBB`/`#RRGGBBAA` string - defense in depth, never trusts the
 * backend alone for something that ends up in inline CSS. */
function safeColor(value: string, fallback: string): string {
  return HEX_COLOR_PATTERN.test(value) ? value : fallback;
}

function hexToRgba(hex: string, opacity: number): string {
  const safe = safeColor(hex, '#000000');
  const r = Number.parseInt(safe.slice(1, 3), 16);
  const g = Number.parseInt(safe.slice(3, 5), 16);
  const b = Number.parseInt(safe.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, ${clamp(opacity, 0, 1)})`;
}

export type OverlayContainerStyle = CSSProperties & {
  '--chat-overlay-animation-duration': string;
};

/** Root container style: transparent background, the configured font
 * stack/size/weight/line-height/text color, and a CSS custom property the
 * animation classes read for their duration - see overlay-animations.css
 * equivalents inlined as Tailwind utility classes on each item instead
 * (see OverlayMessage.tsx/OverlayActivity.tsx). */
export function overlayContainerStyle(config: PublicChatOverlayConfig): OverlayContainerStyle {
  return {
    fontFamily: FONT_STACKS[config.fontFamily] ?? FONT_STACKS.sans_serif,
    fontSize: `${clamp(config.fontSize, 8, 64)}px`,
    fontWeight: clamp(config.fontWeight, 100, 900),
    lineHeight: clamp(config.lineHeight, 1, 3),
    color: safeColor(config.textColor, '#FFFFFF'),
    '--chat-overlay-animation-duration': `${clamp(config.animationDurationMs, 0, 5000)}ms`,
  };
}

export function overlayItemStyle(config: PublicChatOverlayConfig): CSSProperties {
  return {
    backgroundColor: hexToRgba(config.bubbleColor, config.bubbleOpacity),
    borderRadius: `${clamp(config.borderRadius, 0, 64)}px`,
    padding: '0.5em 0.75em',
    marginBottom: `${clamp(config.itemSpacing, 0, 64)}px`,
    textShadow: config.textOutline
      ? '0 0 3px rgba(0,0,0,0.9), 0 0 3px rgba(0,0,0,0.9)'
      : config.textShadow
        ? '0 1px 2px rgba(0,0,0,0.8)'
        : undefined,
  };
}

const ENTRY_ANIMATION_CLASSES: Record<ChatOverlayAnimation, string> = {
  none: '',
  fade: 'animate-chat-overlay-fade-in',
  slide_up: 'animate-chat-overlay-slide-up-in',
  slide_left: 'animate-chat-overlay-slide-left-in',
  scale: 'animate-chat-overlay-scale-in',
};

/** Entry-animation class only - exit animation has no visible effect for
 * this stage's renderer since a removed item is simply not rendered on
 * the next frame (no exit transition state is tracked). `none` and
 * `prefers-reduced-motion` both resolve to no class, so a moderation
 * removal is never delayed by an animation - see this module's own doc
 * comment and the Stage 10 task's Part 11. */
export function entryAnimationClassName(
  animation: ChatOverlayAnimation,
  prefersReducedMotion: boolean,
): string {
  if (prefersReducedMotion) return '';
  return ENTRY_ANIMATION_CLASSES[animation] ?? '';
}
