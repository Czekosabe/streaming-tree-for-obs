import { describe, expect, it } from 'vitest';

import type { AlertEventTypeCapability } from '@/api/alerts-schemas';

import {
  availableTextBindings,
  CANVAS_LANDSCAPE,
  clampFrameToCanvas,
  containScale,
  createHistory,
  createShapeLayer,
  createTextLayer,
  duplicateLayer,
  isFrameWithinCanvas,
  isValidCanvasSize,
  isValidColor,
  isValidFontSize,
  isValidFontWeight,
  isValidOpacity,
  isValidStaticText,
  MAX_UNDO_HISTORY,
  moveLayerOrder,
  newLayerId,
  normalizeLayerOrder,
  pushHistory,
  redoHistory,
  snapFramePosition,
  undoHistory,
} from './visualdesign';

describe('isValidCanvasSize', () => {
  it('accepts the two presets', () => {
    expect(isValidCanvasSize(1920, 1080)).toBe(true);
    expect(isValidCanvasSize(1080, 1920)).toBe(true);
  });
  it('rejects out-of-bounds dimensions', () => {
    expect(isValidCanvasSize(100, 1080)).toBe(false);
    expect(isValidCanvasSize(1920, 100)).toBe(false);
    expect(isValidCanvasSize(9999, 1080)).toBe(false);
  });
  it('rejects NaN/Infinity', () => {
    expect(isValidCanvasSize(NaN, 1080)).toBe(false);
    expect(isValidCanvasSize(1920, Infinity)).toBe(false);
  });
});

describe('isValidOpacity / isValidFontSize / isValidFontWeight', () => {
  it('accepts boundary values', () => {
    expect(isValidOpacity(0)).toBe(true);
    expect(isValidOpacity(1)).toBe(true);
    expect(isValidFontSize(8)).toBe(true);
    expect(isValidFontSize(300)).toBe(true);
    expect(isValidFontWeight(100)).toBe(true);
    expect(isValidFontWeight(900)).toBe(true);
  });
  it('rejects out of range', () => {
    expect(isValidOpacity(-0.1)).toBe(false);
    expect(isValidOpacity(1.1)).toBe(false);
    expect(isValidFontSize(7)).toBe(false);
    expect(isValidFontSize(301)).toBe(false);
  });
  it('rejects a non-increment font weight', () => {
    expect(isValidFontWeight(450)).toBe(false);
  });
});

describe('isValidColor', () => {
  it('accepts #RRGGBB and #RRGGBBAA', () => {
    expect(isValidColor('#000000')).toBe(true);
    expect(isValidColor('#a1b2c3d4')).toBe(true);
  });
  it('rejects a CSS color name, rgb(), or short hex', () => {
    expect(isValidColor('red')).toBe(false);
    expect(isValidColor('rgb(0,0,0)')).toBe(false);
    expect(isValidColor('#fff')).toBe(false);
  });
});

describe('isValidStaticText', () => {
  it('rejects empty and overlong text', () => {
    expect(isValidStaticText('')).toBe(false);
    expect(isValidStaticText('a'.repeat(501))).toBe(false);
  });
  it('accepts normal text', () => {
    expect(isValidStaticText('Hello!')).toBe(true);
  });
});

describe('availableTextBindings', () => {
  const followCapability: AlertEventTypeCapability = {
    eventType: 'follow', hasUser: true, hasMessage: false, hasQuantity: false,
    hasAnonymity: false, hasRewardTitle: false, hasRoles: false,
    availablePlaceholders: [], groupable: false, groupingRequiresHiddenMessage: false,
  };
  const bitsCapability: AlertEventTypeCapability = {
    eventType: 'bits', hasUser: true, hasMessage: true, hasQuantity: true,
    hasAnonymity: true, hasRewardTitle: false, hasRoles: false,
    availablePlaceholders: [], groupable: true, groupingRequiresHiddenMessage: true,
  };

  it('always includes static/alert_rendered_text/platform/event_type', () => {
    const bindings = availableTextBindings(followCapability);
    expect(bindings).toEqual(expect.arrayContaining(['static', 'alert_rendered_text', 'platform', 'event_type']));
  });
  it('excludes quantity/message/group_count for follow', () => {
    const bindings = availableTextBindings(followCapability);
    expect(bindings).not.toContain('quantity');
    expect(bindings).not.toContain('message');
    expect(bindings).not.toContain('group_count');
  });
  it('includes quantity/message/group_count for bits', () => {
    const bindings = availableTextBindings(bitsCapability);
    expect(bindings).toEqual(expect.arrayContaining(['quantity', 'message', 'group_count']));
  });
  it('returns the base set when capability is not yet loaded', () => {
    expect(availableTextBindings(undefined)).toEqual(['static', 'alert_rendered_text', 'platform', 'event_type']);
  });
});

describe('layer factories', () => {
  it('createShapeLayer produces a rectangle shape payload', () => {
    const layer = createShapeLayer({ x: 0, y: 0, width: 100, height: 50 }, 0);
    expect(layer.kind).toBe('shape');
    expect(layer.shape?.kind).toBe('rectangle');
    expect(layer.text).toBeUndefined();
  });
  it('createTextLayer defaults to alert_rendered_text binding', () => {
    const layer = createTextLayer({ x: 0, y: 0, width: 100, height: 50 }, 0);
    expect(layer.kind).toBe('text');
    expect(layer.text?.binding).toBe('alert_rendered_text');
  });
  it('every new layer gets a unique id', () => {
    const a = newLayerId();
    const b = newLayerId();
    expect(a).not.toBe(b);
    expect(a.startsWith('layer_')).toBe(true);
  });
});

describe('duplicateLayer', () => {
  it('gives the copy a new id and a small offset, clamped to canvas', () => {
    const original = createShapeLayer({ x: 1900, y: 0, width: 100, height: 50 }, 0);
    const copy = duplicateLayer(original, CANVAS_LANDSCAPE, 1);
    expect(copy.id).not.toBe(original.id);
    expect(copy.frame.x + copy.frame.width).toBeLessThanOrEqual(CANVAS_LANDSCAPE.width);
  });
});

describe('clampFrameToCanvas / isFrameWithinCanvas', () => {
  it('clamps an off-canvas frame back on-canvas', () => {
    const clamped = clampFrameToCanvas({ x: 1900, y: 0, width: 100, height: 50 }, CANVAS_LANDSCAPE);
    expect(isFrameWithinCanvas(clamped, CANVAS_LANDSCAPE)).toBe(true);
  });
  it('enforces the minimum layer size', () => {
    const clamped = clampFrameToCanvas({ x: 0, y: 0, width: 1, height: 1 }, CANVAS_LANDSCAPE);
    expect(clamped.width).toBeGreaterThanOrEqual(8);
    expect(clamped.height).toBeGreaterThanOrEqual(8);
  });
});

describe('containScale', () => {
  it('scales down uniformly for a smaller viewport, never distorting aspect ratio', () => {
    const scale = containScale(CANVAS_LANDSCAPE, 1280, 720);
    expect(scale).toBeCloseTo(1280 / 1920, 5);
  });
  it('is limited by the more constraining axis for a mismatched aspect ratio', () => {
    // A landscape canvas in a taller-than-wide viewport is limited by width.
    const scale = containScale(CANVAS_LANDSCAPE, 800, 2000);
    expect(scale).toBeCloseTo(800 / 1920, 5);
  });
});

describe('snapFramePosition', () => {
  it('snaps to the canvas horizontal/vertical center within threshold', () => {
    const centeredX = (CANVAS_LANDSCAPE.width - 100) / 2;
    const centeredY = (CANVAS_LANDSCAPE.height - 50) / 2;
    const result = snapFramePosition({ x: centeredX + 3, y: centeredY - 2, width: 100, height: 50 }, CANVAS_LANDSCAPE, 10);
    expect(result.x).toBe(Math.round(centeredX));
    expect(result.y).toBe(Math.round(centeredY));
    expect(result.snappedX).toBe(true);
    expect(result.snappedY).toBe(true);
  });
  it('does not snap when outside the threshold', () => {
    const result = snapFramePosition({ x: 500, y: 500, width: 100, height: 50 }, CANVAS_LANDSCAPE, 10);
    expect(result.snappedX).toBe(false);
    expect(result.snappedY).toBe(false);
    expect(result.x).toBe(500);
  });
});

describe('normalizeLayerOrder / moveLayerOrder', () => {
  it('normalizes to a dense 0..N-1 sequence', () => {
    const layers = [
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 50), id: 'a' },
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 5), id: 'b' },
    ];
    const normalized = normalizeLayerOrder(layers);
    expect(normalized.find((l) => l.id === 'b')?.order).toBe(0);
    expect(normalized.find((l) => l.id === 'a')?.order).toBe(1);
  });

  it('moveLayerOrder("front") moves a layer to the highest order', () => {
    const layers = [
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 0), id: 'a' },
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 1), id: 'b' },
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 2), id: 'c' },
    ];
    const reordered = moveLayerOrder(layers, 'a', 'front');
    const sorted = [...reordered].sort((x, y) => x.order - y.order);
    expect(sorted[sorted.length - 1]?.id).toBe('a');
  });

  it('moveLayerOrder("back") moves a layer to the lowest order', () => {
    const layers = [
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 0), id: 'a' },
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 1), id: 'b' },
      { ...createShapeLayer({ x: 0, y: 0, width: 10, height: 10 }, 2), id: 'c' },
    ];
    const reordered = moveLayerOrder(layers, 'c', 'back');
    const sorted = [...reordered].sort((x, y) => x.order - y.order);
    expect(sorted[0]?.id).toBe('c');
  });
});

describe('bounded undo/redo history', () => {
  it('undo/redo round-trips a single push', () => {
    let history = createHistory(0);
    history = pushHistory(history, 1);
    expect(history.present).toBe(1);
    history = undoHistory(history);
    expect(history.present).toBe(0);
    history = redoHistory(history);
    expect(history.present).toBe(1);
  });

  it('a new push after undo discards the redo (future) branch', () => {
    let history = createHistory(0);
    history = pushHistory(history, 1);
    history = undoHistory(history);
    history = pushHistory(history, 2);
    expect(history.future).toHaveLength(0);
    expect(history.present).toBe(2);
  });

  it('bounds past history to MAX_UNDO_HISTORY entries', () => {
    let history = createHistory(0);
    for (let i = 1; i <= MAX_UNDO_HISTORY + 10; i++) {
      history = pushHistory(history, i);
    }
    expect(history.past.length).toBe(MAX_UNDO_HISTORY);
  });

  it('undo/redo on an empty history is a no-op', () => {
    const history = createHistory(0);
    expect(undoHistory(history)).toEqual(history);
    expect(redoHistory(history)).toEqual(history);
  });
});
