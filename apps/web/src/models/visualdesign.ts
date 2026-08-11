/**
 * Pure, backend-mirroring helpers for Stage 13A visual designs: bounds
 * (mirroring internal/domain/visualdesign's own constants), layer
 * factories, canvas/coordinate math, snapping, and text-binding
 * availability. The backend is the real authority for every bound
 * below - these exist for live client-side feedback and for the
 * editor's own geometry math, which has no backend equivalent at all.
 */

import type { AlertEventTypeCapability } from '@/api/alerts-schemas';
import type {
  VisualDesignCanvas,
  VisualDesignFrame,
  VisualDesignLayer,
  VisualDesignLayerKind,
  VisualDesignTextBinding,
} from '@/api/visualdesign-schemas';

export function codePointLength(text: string): number {
  return Array.from(text).length;
}

// --- bounds (mirror internal/domain/visualdesign's own constants) -------

export const MIN_CANVAS_WIDTH = 320;
export const MAX_CANVAS_WIDTH = 3840;
export const MIN_CANVAS_HEIGHT = 240;
export const MAX_CANVAS_HEIGHT = 3840;

export const MIN_LAYER_SIZE = 8;
export const MAX_LAYERS = 50;
export const MAX_LAYER_NAME_CODE_POINTS = 80;
export const MAX_STATIC_TEXT_CODE_POINTS = 500;

export const MIN_OPACITY = 0;
export const MAX_OPACITY = 1;

export const MIN_BORDER_WIDTH = 0;
export const MAX_BORDER_WIDTH = 32;
export const MIN_CORNER_RADIUS = 0;
export const MAX_CORNER_RADIUS = 500;

export const MIN_FONT_SIZE = 8;
export const MAX_FONT_SIZE = 300;
export const MIN_FONT_WEIGHT = 100;
export const MAX_FONT_WEIGHT = 900;
export const FONT_WEIGHT_STEP = 100;
export const MIN_LINE_HEIGHT = 0.8;
export const MAX_LINE_HEIGHT = 3.0;
export const MIN_LETTER_SPACING = -2;
export const MAX_LETTER_SPACING = 20;

export const MIN_OUTLINE_WIDTH = 0;
export const MAX_OUTLINE_WIDTH = 16;
export const MIN_SHADOW_OFFSET = -32;
export const MAX_SHADOW_OFFSET = 32;
export const MIN_SHADOW_BLUR = 0;
export const MAX_SHADOW_BLUR = 64;

export const MIN_LAYER_ANIMATION_DURATION_MS = 0;
export const MAX_LAYER_ANIMATION_DURATION_MS = 2000;

export const MAX_UNDO_HISTORY = 50;

/** Stage 13B additions (docs/visual-designs.md §21). */
export const MIN_EMOTE_SIZE = 8;
export const MAX_EMOTE_SIZE = 128;
export const MIN_BADGE_COUNT = 1;
export const MAX_BADGE_COUNT = 20;
export const MIN_BADGE_SIZE = 8;
export const MAX_BADGE_SIZE = 128;
export const MIN_BADGE_GAP = 0;
export const MAX_BADGE_GAP = 32;

export const VISUAL_DESIGN_LAYER_KINDS = ['shape', 'text', 'platform_icon', 'avatar'] as const;
/** The two Stage 13B layer kinds, offered by the Chat Overlay Designer's
 * own add-layer menu in addition to the four shared ones above. */
export const CHAT_VISUAL_DESIGN_LAYER_KINDS = ['message_fragments', 'badge_list'] as const;

export const VISUAL_DESIGN_TEXT_BINDINGS = [
  'static',
  'alert_rendered_text',
  'username',
  'platform',
  'event_type',
  'message',
  'quantity',
  'group_count',
  'timestamp',
  'account_label',
] as const;
export const VISUAL_DESIGN_FONT_FAMILIES = ['system-ui', 'sans-serif', 'serif', 'monospace'] as const;

export const CANVAS_LANDSCAPE: VisualDesignCanvas = { width: 1920, height: 1080, transparent: true };
export const CANVAS_VERTICAL: VisualDesignCanvas = { width: 1080, height: 1920, transparent: true };
/** The Chat Overlay Designer's own canvas preset - a chat visual design
 * describes one repeated overlay item/card, not a full-screen
 * presentation (docs/visual-designs.md §17), mirroring
 * visualdesign.CanvasChatItem on the backend exactly. */
export const CANVAS_CHAT_ITEM: VisualDesignCanvas = { width: 960, height: 280, transparent: true };

const ZOOM_LEVELS = [0.25, 0.5, 0.75, 1, 1.5, 2] as const;
export const ZOOM_LEVELS_ARRAY: readonly number[] = ZOOM_LEVELS;
export const MIN_ZOOM = ZOOM_LEVELS[0];
export const MAX_ZOOM = ZOOM_LEVELS[ZOOM_LEVELS.length - 1];

// --- validation predicates ------------------------------------------------

export function isValidCanvasSize(width: number, height: number): boolean {
  return (
    Number.isFinite(width) &&
    Number.isFinite(height) &&
    width >= MIN_CANVAS_WIDTH &&
    width <= MAX_CANVAS_WIDTH &&
    height >= MIN_CANVAS_HEIGHT &&
    height <= MAX_CANVAS_HEIGHT
  );
}

export function isValidOpacity(value: number): boolean {
  return Number.isFinite(value) && value >= MIN_OPACITY && value <= MAX_OPACITY;
}

export function isValidFontSize(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_FONT_SIZE && value <= MAX_FONT_SIZE;
}

export function isValidFontWeight(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_FONT_WEIGHT && value <= MAX_FONT_WEIGHT && value % FONT_WEIGHT_STEP === 0;
}

const HEX_COLOR_PATTERN = /^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$/;

export function isValidColor(value: string): boolean {
  return HEX_COLOR_PATTERN.test(value);
}

export function isValidStaticText(value: string): boolean {
  const n = codePointLength(value);
  return n > 0 && n <= MAX_STATIC_TEXT_CODE_POINTS;
}

export function isValidLayerName(value: string): boolean {
  return codePointLength(value) <= MAX_LAYER_NAME_CODE_POINTS;
}

// --- text-binding availability (mirrors internal/domain/alerts's own
// AvailableTextBindings, driven by the same capability table) -----------

export function availableTextBindings(capability: AlertEventTypeCapability | undefined): VisualDesignTextBinding[] {
  const out: VisualDesignTextBinding[] = ['static', 'alert_rendered_text', 'platform', 'event_type'];
  if (capability === undefined) return out;
  if (capability.hasUser) out.push('username');
  if (capability.hasQuantity) out.push('quantity');
  if (capability.hasMessage) out.push('message');
  if (capability.groupable) out.push('group_count');
  return out;
}

/** The bindings available to a chat-overlay design (mirrors
 * internal/domain/chatoverlay's own AvailableTextBindings union across
 * both item kinds - docs/visual-designs.md §20.1: "Stage 13B does not
 * create a separate persisted design per item kind"). Never includes
 * `alert_rendered_text` (alert-only) or `group_count` (chat items are
 * never grouped). */
export const CHAT_VISUAL_DESIGN_TEXT_BINDINGS: readonly VisualDesignTextBinding[] = [
  'static',
  'username',
  'platform',
  'message',
  'event_type',
  'quantity',
  'timestamp',
  'account_label',
];

// --- layer id / factories -------------------------------------------------

export function newLayerId(): string {
  return `layer_${crypto.randomUUID().replace(/-/g, '')}`;
}

function baseLayer(kind: VisualDesignLayerKind, name: string, frame: VisualDesignFrame, order: number): Omit<
  VisualDesignLayer,
  'shape' | 'text' | 'platformIcon' | 'avatar' | 'messageFragments' | 'badgeList'
> {
  return {
    id: newLayerId(), name, kind, visible: true, locked: false, order, frame, opacity: 1,
    entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
  };
}

export function createShapeLayer(frame: VisualDesignFrame, order: number): VisualDesignLayer {
  return {
    ...baseLayer('shape', 'Rectangle', frame, order),
    shape: { kind: 'rectangle', fill: '#1F2937', borderColor: '#000000', borderWidth: 0, cornerRadius: 8 },
  };
}

export function createTextLayer(frame: VisualDesignFrame, order: number, binding: VisualDesignTextBinding = 'alert_rendered_text'): VisualDesignLayer {
  return {
    ...baseLayer('text', 'Text', frame, order),
    text: {
      binding, staticText: binding === 'static' ? 'New text' : '', missingValueBehavior: 'hide',
      fontFamily: 'system-ui', fontSize: 32, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
      textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
      outlineWidth: 0, outlineColor: '#000000',
      shadowEnabled: true, shadowOffsetX: 0, shadowOffsetY: 2, shadowBlur: 8, shadowColor: '#000000CC',
    },
  };
}

export function createPlatformIconLayer(frame: VisualDesignFrame, order: number): VisualDesignLayer {
  return { ...baseLayer('platform_icon', 'Platform icon', frame, order), platformIcon: {} };
}

export function createAvatarLayer(frame: VisualDesignFrame, order: number): VisualDesignLayer {
  return {
    ...baseLayer('avatar', 'Avatar', frame, order),
    avatar: { cornerRadius: 500, borderColor: '#FFFFFF', borderWidth: 0 },
  };
}

export function createMessageFragmentsLayer(frame: VisualDesignFrame, order: number): VisualDesignLayer {
  return {
    ...baseLayer('message_fragments', 'Message', frame, order),
    messageFragments: {
      fontFamily: 'system-ui', fontSize: 16, fontWeight: 400, lineHeight: 1.3, letterSpacing: 0,
      textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'top', emoteSize: 24,
    },
  };
}

export function createBadgeListLayer(frame: VisualDesignFrame, order: number): VisualDesignLayer {
  return {
    ...baseLayer('badge_list', 'Badges', frame, order),
    badgeList: { maxCount: 5, badgeSize: 18, gap: 4 },
  };
}

/** Deep-copies layer with a new id and a small, bounded position offset
 * (Stage 13A task Part 34), clamped back onto the canvas. */
export function duplicateLayer(layer: VisualDesignLayer, canvas: VisualDesignCanvas, order: number): VisualDesignLayer {
  const offset = 16;
  const frame = clampFrameToCanvas(
    { x: layer.frame.x + offset, y: layer.frame.y + offset, width: layer.frame.width, height: layer.frame.height },
    canvas,
  );
  return { ...structuredClone(layer), id: newLayerId(), name: `${layer.name} copy`, order, frame };
}

// --- frame / canvas geometry ----------------------------------------------

export function clampFrameToCanvas(frame: VisualDesignFrame, canvas: VisualDesignCanvas): VisualDesignFrame {
  const width = Math.min(Math.max(frame.width, MIN_LAYER_SIZE), canvas.width);
  const height = Math.min(Math.max(frame.height, MIN_LAYER_SIZE), canvas.height);
  const x = Math.min(Math.max(frame.x, 0), canvas.width - width);
  const y = Math.min(Math.max(frame.y, 0), canvas.height - height);
  return { x: Math.round(x), y: Math.round(y), width: Math.round(width), height: Math.round(height) };
}

export function isFrameWithinCanvas(frame: VisualDesignFrame, canvas: VisualDesignCanvas): boolean {
  return (
    frame.width >= MIN_LAYER_SIZE &&
    frame.height >= MIN_LAYER_SIZE &&
    frame.x >= 0 &&
    frame.y >= 0 &&
    frame.x + frame.width <= canvas.width &&
    frame.y + frame.height <= canvas.height
  );
}

/** contain-style scale: the same policy the shared renderer applies at
 * runtime (docs/visual-designs.md §3) - never stretched independently,
 * always centered. */
export function containScale(canvas: VisualDesignCanvas, viewportWidth: number, viewportHeight: number): number {
  if (canvas.width <= 0 || canvas.height <= 0 || viewportWidth <= 0 || viewportHeight <= 0) return 1;
  return Math.min(viewportWidth / canvas.width, viewportHeight / canvas.height);
}

export type SnapResult = { x: number; y: number; snappedX: boolean; snappedY: boolean };

/** Snaps frame's position to the canvas's own horizontal/vertical
 * center and edges when within thresholdDesignUnits of them (Stage 13A
 * task Part 31) - purely an editor convenience; the persisted
 * coordinates are always ordinary, unsnapped-unless-actually-within-
 * threshold integers. */
export function snapFramePosition(
  frame: VisualDesignFrame,
  canvas: VisualDesignCanvas,
  thresholdDesignUnits: number,
): SnapResult {
  let x = frame.x;
  let y = frame.y;
  let snappedX = false;
  let snappedY = false;

  const centerX = (canvas.width - frame.width) / 2;
  const centerY = (canvas.height - frame.height) / 2;
  const rightEdge = canvas.width - frame.width;
  const bottomEdge = canvas.height - frame.height;

  const xCandidates: number[] = [0, centerX, rightEdge];
  const yCandidates: number[] = [0, centerY, bottomEdge];

  for (const candidate of xCandidates) {
    if (Math.abs(frame.x - candidate) <= thresholdDesignUnits) {
      x = candidate;
      snappedX = true;
      break;
    }
  }
  for (const candidate of yCandidates) {
    if (Math.abs(frame.y - candidate) <= thresholdDesignUnits) {
      y = candidate;
      snappedY = true;
      break;
    }
  }

  return { x: Math.round(x), y: Math.round(y), snappedX, snappedY };
}

// --- layer ordering ---------------------------------------------------

/** Returns a new layer array with orders normalized to a dense
 * 0..N-1 sequence, stable-sorted by current order - mirrors the
 * backend's own Service.Save normalization exactly, so the editor's
 * own display order always matches what a save will persist. */
export function normalizeLayerOrder(layers: VisualDesignLayer[]): VisualDesignLayer[] {
  const sorted = [...layers].sort((a, b) => a.order - b.order);
  return sorted.map((layer, index) => ({ ...layer, order: index }));
}

export function moveLayerOrder(layers: VisualDesignLayer[], layerId: string, direction: 'up' | 'down' | 'front' | 'back'): VisualDesignLayer[] {
  const sorted = normalizeLayerOrder(layers);
  const index = sorted.findIndex((l) => l.id === layerId);
  if (index === -1) return layers;

  const reordered = [...sorted];
  const [item] = reordered.splice(index, 1);
  if (item === undefined) return layers;

  switch (direction) {
    case 'up': {
      const target = Math.max(0, index - 1);
      reordered.splice(target, 0, item);
      break;
    }
    case 'down': {
      const target = Math.min(reordered.length, index + 1);
      reordered.splice(target, 0, item);
      break;
    }
    case 'front':
      reordered.push(item);
      break;
    case 'back':
      reordered.unshift(item);
      break;
  }
  // Re-derive order from the array position just constructed above -
  // never from the (now stale) .order field normalizeLayerOrder would
  // otherwise re-sort by, which would silently undo this reorder.
  return reordered.map((layer, index) => ({ ...layer, order: index }));
}

// --- bounded undo/redo history (Stage 13A task Part 35) ------------------

export type History<T> = { past: T[]; present: T; future: T[] };

export function createHistory<T>(initial: T): History<T> {
  return { past: [], present: initial, future: [] };
}

/** Pushes a new present state, discarding future (redo) branches and
 * bounding past to MAX_UNDO_HISTORY entries. */
export function pushHistory<T>(history: History<T>, next: T): History<T> {
  const past = [...history.past, history.present].slice(-MAX_UNDO_HISTORY);
  return { past, present: next, future: [] };
}

export function undoHistory<T>(history: History<T>): History<T> {
  if (history.past.length === 0) return history;
  const previous = history.past[history.past.length - 1];
  if (previous === undefined) return history;
  return {
    past: history.past.slice(0, -1),
    present: previous,
    future: [history.present, ...history.future],
  };
}

export function redoHistory<T>(history: History<T>): History<T> {
  if (history.future.length === 0) return history;
  const next = history.future[0];
  if (next === undefined) return history;
  return {
    past: [...history.past, history.present],
    future: history.future.slice(1),
    present: next,
  };
}
