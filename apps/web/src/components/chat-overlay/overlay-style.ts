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

/** `none` and `prefers-reduced-motion` both resolve to no class, so an
 * item never waits on an animation that was never going to play. */
export function entryAnimationClassName(
  animation: ChatOverlayAnimation,
  prefersReducedMotion: boolean,
): string {
  if (prefersReducedMotion) return '';
  return ENTRY_ANIMATION_CLASSES[animation] ?? '';
}

const EXIT_ANIMATION_CLASSES: Record<ChatOverlayAnimation, string> = {
  none: '',
  fade: 'animate-chat-overlay-fade-out',
  slide_up: 'animate-chat-overlay-slide-up-out',
  slide_left: 'animate-chat-overlay-slide-left-out',
  scale: 'animate-chat-overlay-scale-out',
};

/** Exit-animation class - used only by a cosmetic "leaving" item (see
 * models/chat-overlay-reducer.ts), never by an immediate removal, which
 * skips this entirely. `none` and `prefers-reduced-motion` both resolve
 * to no class, in which case the caller must remove the item
 * immediately rather than waiting on an animation that will never
 * fire - see OverlayLeavingItem.tsx's own fallback-timeout handling. */
export function exitAnimationClassName(
  animation: ChatOverlayAnimation,
  prefersReducedMotion: boolean,
): string {
  if (prefersReducedMotion) return '';
  return EXIT_ANIMATION_CLASSES[animation] ?? '';
}

/** How long to wait, at most, for a leaving item's exit animation
 * before removing it regardless - the configured duration plus a fixed
 * buffer, since `animationend` must never be the only removal path
 * (a CSS bug, a hidden tab throttling rAF, or `animation: none`
 * resolving to an instant no-op animation could all otherwise leave a
 * "leaving" item stuck forever). Mirrors the same clamped 0-5000ms
 * range `overlayContainerStyle` already enforces. */
export function exitAnimationFallbackMs(animationDurationMs: number): number {
  return clamp(animationDurationMs, 0, 5000) + 150;
}

const STACK_DIRECTION_JUSTIFY: Record<PublicChatOverlayConfig['stackDirection'], string> = {
  top_down: 'justify-start',
  bottom_up: 'justify-end',
};

const HORIZONTAL_ALIGNMENT_ITEMS: Record<PublicChatOverlayConfig['horizontalAlignment'], string> = {
  left: 'items-start text-left',
  center: 'items-center text-center',
  right: 'items-end text-right',
};

const LAYOUT_MODE_MAX_WIDTH: Record<PublicChatOverlayConfig['layoutMode'], string> = {
  vertical: 'max-w-full',
  horizontal: 'mx-auto max-w-[720px]',
};

/**
 * The overlay root's own stack-direction/alignment/max-width classes,
 * resolved here as one already-computed string per config, rather than as
 * ternaries/record lookups written directly inside the root `cn(...)` call
 * in ChatOverlayRenderer.tsx. Each of the three selections above is
 * genuinely mutually exclusive at runtime (exactly one key ever applies),
 * but Tailwind CSS IntelliSense's conflict checker has no control-flow
 * awareness - written as ternaries inside a recognized class-merging call,
 * it sees both branches' literal classes at once and flags a false
 * "conflicting classnames" warning (e.g. `justify-start` next to
 * `justify-end`). Passing a single plain string built outside that call
 * removes the ambiguity for the editor exactly as it already has none for
 * the renderer - the CSS output is unchanged either way.
 */
export function overlayRootLayoutClassName(config: PublicChatOverlayConfig): string {
  return [
    STACK_DIRECTION_JUSTIFY[config.stackDirection],
    HORIZONTAL_ALIGNMENT_ITEMS[config.horizontalAlignment],
    LAYOUT_MODE_MAX_WIDTH[config.layoutMode],
  ].join(' ');
}
