import type { CSSProperties } from 'react';

import type {
  VisualDesignAnimation,
  VisualDesignAvatarProps,
  VisualDesignBadgeListProps,
  VisualDesignFrame,
  VisualDesignMessageFragmentsProps,
  VisualDesignShapeProps,
  VisualDesignTextProps,
} from '@/api/visualdesign-schemas';

/**
 * Maps validated, bounded visual-design layer fields onto CSS -
 * exactly like components/alerts/alert-style.ts's own doc comment:
 * never an arbitrary string passed straight into `style` (Stage 13A
 * task Part 45 - no raw CSS, no class names, no HTML fragments). Every
 * value here already passed backend validation (colors are `#RRGGBB`/
 * `#RRGGBBAA`, numbers are bounded) before it could ever reach this
 * module.
 */

const FONT_FAMILY_STACKS: Record<string, string> = {
  'system-ui': 'system-ui, sans-serif',
  'sans-serif': 'sans-serif',
  serif: 'serif',
  monospace: 'monospace',
};

/** Absolute-positioned frame, converted from design units to scaled
 * pixels - the one place layer geometry becomes real CSS. */
export function layerFrameStyle(frame: VisualDesignFrame, scale: number, opacity: number): CSSProperties {
  return {
    position: 'absolute',
    left: frame.x * scale,
    top: frame.y * scale,
    width: frame.width * scale,
    height: frame.height * scale,
    opacity,
  };
}

export function shapeLayerStyle(shape: VisualDesignShapeProps, scale: number): CSSProperties {
  return {
    width: '100%',
    height: '100%',
    backgroundColor: shape.fill,
    borderRadius: shape.cornerRadius * scale,
    borderStyle: shape.borderWidth > 0 ? 'solid' : 'none',
    borderWidth: shape.borderWidth * scale,
    borderColor: shape.borderColor,
    boxSizing: 'border-box',
  };
}

export function avatarLayerStyle(avatar: VisualDesignAvatarProps, scale: number): CSSProperties {
  return {
    width: '100%',
    height: '100%',
    objectFit: 'cover',
    borderRadius: avatar.cornerRadius * scale,
    borderStyle: avatar.borderWidth > 0 ? 'solid' : 'none',
    borderWidth: avatar.borderWidth * scale,
    borderColor: avatar.borderColor,
    boxSizing: 'border-box',
    display: 'block',
  };
}

const H_ALIGN: Record<string, CSSProperties['justifyContent']> = { left: 'flex-start', center: 'center', right: 'flex-end' };
const V_ALIGN: Record<string, CSSProperties['alignItems']> = { top: 'flex-start', middle: 'center', bottom: 'flex-end' };
const TEXT_ALIGN: Record<string, CSSProperties['textAlign']> = { left: 'left', center: 'center', right: 'right' };

export function textLayerContainerStyle(text: VisualDesignTextProps): CSSProperties {
  return {
    width: '100%',
    height: '100%',
    display: 'flex',
    justifyContent: H_ALIGN[text.horizontalAlign] ?? 'center',
    alignItems: V_ALIGN[text.verticalAlign] ?? 'center',
    overflow: 'hidden',
  };
}

export function textLayerStyle(text: VisualDesignTextProps, scale: number): CSSProperties {
  const shadow = text.shadowEnabled
    ? `${text.shadowOffsetX * scale}px ${text.shadowOffsetY * scale}px ${text.shadowBlur * scale}px ${text.shadowColor}`
    : undefined;
  const outline = text.outlineWidth > 0 ? `${text.outlineColor} ${text.outlineWidth * scale}px` : undefined;
  return {
    fontFamily: FONT_FAMILY_STACKS[text.fontFamily] ?? 'sans-serif',
    fontSize: text.fontSize * scale,
    fontWeight: text.fontWeight,
    lineHeight: text.lineHeight,
    letterSpacing: text.letterSpacing * scale,
    color: text.textColor,
    textAlign: TEXT_ALIGN[text.horizontalAlign] ?? 'center',
    textShadow: shadow,
    WebkitTextStroke: outline,
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
    maxWidth: '100%',
  };
}

/** Stage 13B additions (docs/visual-designs.md §21). */
export function messageFragmentsContainerStyle(props: VisualDesignMessageFragmentsProps): CSSProperties {
  return {
    width: '100%',
    height: '100%',
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: V_ALIGN[props.verticalAlign] ?? 'center',
    justifyContent: H_ALIGN[props.horizontalAlign] ?? 'flex-start',
    overflow: 'hidden',
    columnGap: '0.25em',
    rowGap: '0.15em',
  };
}

export function messageFragmentsTextStyle(props: VisualDesignMessageFragmentsProps, scale: number): CSSProperties {
  return {
    fontFamily: FONT_FAMILY_STACKS[props.fontFamily] ?? 'sans-serif',
    fontSize: props.fontSize * scale,
    fontWeight: props.fontWeight,
    lineHeight: props.lineHeight,
    letterSpacing: props.letterSpacing * scale,
    color: props.textColor,
    textAlign: TEXT_ALIGN[props.horizontalAlign] ?? 'left',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  };
}

export function messageFragmentsEmoteStyle(props: VisualDesignMessageFragmentsProps, scale: number): CSSProperties {
  const size = props.emoteSize * scale;
  return { width: size, height: size, display: 'inline-block', verticalAlign: 'middle' };
}

export function badgeListContainerStyle(props: VisualDesignBadgeListProps): CSSProperties {
  return {
    width: '100%',
    height: '100%',
    display: 'flex',
    alignItems: 'center',
    gap: props.gap,
    overflow: 'hidden',
  };
}

export function badgeListImageStyle(props: VisualDesignBadgeListProps): CSSProperties {
  return { width: props.badgeSize, height: props.badgeSize, display: 'block', flexShrink: 0 };
}

const ENTRY_ANIMATION_CLASSES: Record<VisualDesignAnimation, string> = {
  none: '',
  fade: 'animate-chat-overlay-fade-in',
  slide_up: 'animate-chat-overlay-slide-up-in',
  slide_left: 'animate-chat-overlay-slide-left-in',
  scale: 'animate-chat-overlay-scale-in',
};

const EXIT_ANIMATION_CLASSES: Record<VisualDesignAnimation, string> = {
  none: '',
  fade: 'animate-chat-overlay-fade-out',
  slide_up: 'animate-chat-overlay-slide-up-out',
  slide_left: 'animate-chat-overlay-slide-left-out',
  scale: 'animate-chat-overlay-scale-out',
};

/** Reuses the exact same application-owned animation utility classes
 * alerts/chat-overlay already define (Stage 13A task Part 15) - never
 * new keyframes. `none` and `prefers-reduced-motion` both resolve to
 * no class, and the renderer must still reach the correct final state
 * either way. */
export function layerEntryAnimationClassName(animation: VisualDesignAnimation, prefersReducedMotion: boolean): string {
  if (prefersReducedMotion) return '';
  return ENTRY_ANIMATION_CLASSES[animation] ?? '';
}

export function layerExitAnimationClassName(animation: VisualDesignAnimation, prefersReducedMotion: boolean): string {
  if (prefersReducedMotion) return '';
  return EXIT_ANIMATION_CLASSES[animation] ?? '';
}
