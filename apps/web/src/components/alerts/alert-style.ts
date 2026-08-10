import type { CSSProperties } from 'react';

import type { AlertAnimation, AlertPosition, AlertTextAlign, AlertTheme } from '@/api/alerts-schemas';

/**
 * Maps a validated public alert profile config onto CSS - never an
 * arbitrary string from the backend passed straight into `style`
 * (Part 24/25: a fixed, closed presentation model, never arbitrary
 * CSS/positioning).
 *
 * Entry/exit animations deliberately reuse the exact same
 * application-owned keyframes and Tailwind `@utility` classes
 * `components/chat-overlay/overlay-style.ts` already defines in
 * index.css (`animate-chat-overlay-fade-in`/`-out` etc.) rather than
 * duplicating four more keyframe sets - Part 25 requires "only
 * application-owned animation classes," and the alert's own animation
 * duration still applies correctly because the shared CSS custom
 * property those utilities read (`--chat-overlay-animation-duration`)
 * is set fresh on this renderer's own container, not inherited from
 * any chat overlay. See docs/progress.md's Stage 12A frontend entry
 * for this decision.
 */

function clamp(value: number, min: number, max: number): number {
  if (Number.isNaN(value) || !Number.isFinite(value)) return min;
  return Math.min(max, Math.max(min, value));
}

export type AlertContainerStyle = CSSProperties & {
  '--chat-overlay-animation-duration': string;
};

const POSITION_JUSTIFY: Record<AlertPosition, CSSProperties['alignItems']> = {
  top: 'flex-start',
  center: 'center',
  bottom: 'flex-end',
};

const TEXT_ALIGN: Record<AlertTextAlign, CSSProperties['textAlign']> = {
  left: 'left',
  center: 'center',
  right: 'right',
};

/** Root container style: transparent background, the alert positioned
 * at the profile's configured edge, and the shared animation-duration
 * custom property the reused chat-overlay animation utilities read. */
export function alertContainerStyle(position: AlertPosition, animationDurationMs: number): AlertContainerStyle {
  return {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: POSITION_JUSTIFY[position] ?? 'flex-end',
    height: '100%',
    width: '100%',
    '--chat-overlay-animation-duration': `${clamp(animationDurationMs, 0, 5000)}ms`,
  };
}

const THEME_CLASSES: Record<AlertTheme, string> = {
  minimal: 'px-4 py-2 text-lg rounded-md bg-black/60 text-white',
  compact: 'px-3 py-1.5 text-sm rounded bg-black/70 text-white',
  large: 'px-8 py-4 text-3xl rounded-lg bg-black/60 text-white font-semibold',
};

export function alertThemeClassName(theme: AlertTheme): string {
  return THEME_CLASSES[theme] ?? THEME_CLASSES.minimal;
}

export function alertTextAlignStyle(align: AlertTextAlign): CSSProperties {
  return { textAlign: TEXT_ALIGN[align] ?? 'center' };
}

const ENTRY_ANIMATION_CLASSES: Record<AlertAnimation, string> = {
  none: '',
  fade: 'animate-chat-overlay-fade-in',
  slide_up: 'animate-chat-overlay-slide-up-in',
  slide_left: 'animate-chat-overlay-slide-left-in',
  scale: 'animate-chat-overlay-scale-in',
};

/** `none` and `prefers-reduced-motion` both resolve to no class. */
export function alertEntryAnimationClassName(animation: AlertAnimation, prefersReducedMotion: boolean): string {
  if (prefersReducedMotion) return '';
  return ENTRY_ANIMATION_CLASSES[animation] ?? '';
}

const EXIT_ANIMATION_CLASSES: Record<AlertAnimation, string> = {
  none: '',
  fade: 'animate-chat-overlay-fade-out',
  slide_up: 'animate-chat-overlay-slide-up-out',
  slide_left: 'animate-chat-overlay-slide-left-out',
  scale: 'animate-chat-overlay-scale-out',
};

export function alertExitAnimationClassName(animation: AlertAnimation, prefersReducedMotion: boolean): string {
  if (prefersReducedMotion) return '';
  return EXIT_ANIMATION_CLASSES[animation] ?? '';
}

/** How long to wait, at most, for an exit animation before hiding the
 * alert regardless - never rely on `animationend` alone (Part 25: "hide
 * always completes using a hard fallback timer"). Mirrors
 * chat-overlay's own identical reasoning and buffer. */
export function alertExitAnimationFallbackMs(animationDurationMs: number): number {
  return clamp(animationDurationMs, 0, 5000) + 150;
}
