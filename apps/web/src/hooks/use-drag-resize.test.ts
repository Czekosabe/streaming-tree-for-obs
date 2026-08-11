import { renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { VisualDesignFrame } from '@/api/visualdesign-schemas';
import { CANVAS_LANDSCAPE } from '@/models/visualdesign';

import { useDragResize } from './use-drag-resize';

function fakePointerEvent(pointerId: number, clientX: number, clientY: number): React.PointerEvent {
  return {
    pointerId, clientX, clientY,
    stopPropagation: () => {},
    currentTarget: { setPointerCapture: () => {} },
  } as unknown as React.PointerEvent;
}

function setup(frame: VisualDesignFrame, overrides: Partial<Parameters<typeof useDragResize>[0]> = {}) {
  const onDraftChange = vi.fn();
  const onCommit = vi.fn();
  const { result } = renderHook(() =>
    useDragResize({
      frame, canvas: CANVAS_LANDSCAPE, scale: 1, snapping: false, snapThreshold: 12, locked: false,
      onDraftChange, onCommit,
      ...overrides,
    }),
  );
  return { result, onDraftChange, onCommit };
}

describe('useDragResize', () => {
  it('move updates the frame position by the pointer delta', () => {
    const frame = { x: 100, y: 100, width: 200, height: 100 };
    const { result, onDraftChange } = setup(frame);

    result.current.beginMove(fakePointerEvent(1, 0, 0));
    result.current.onPointerMove(fakePointerEvent(1, 50, 20));

    expect(onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ x: 150, y: 120 }));
  });

  it('commits exactly once on pointer up, not during the drag', () => {
    const frame = { x: 100, y: 100, width: 200, height: 100 };
    const { result, onDraftChange, onCommit } = setup(frame);

    result.current.beginMove(fakePointerEvent(1, 0, 0));
    result.current.onPointerMove(fakePointerEvent(1, 10, 0));
    result.current.onPointerMove(fakePointerEvent(1, 20, 0));
    result.current.onPointerMove(fakePointerEvent(1, 30, 0));
    expect(onCommit).not.toHaveBeenCalled();
    expect(onDraftChange).toHaveBeenCalledTimes(3);

    result.current.onPointerUp(fakePointerEvent(1, 30, 0));
    expect(onCommit).toHaveBeenCalledTimes(1);
  });

  it('clamps a move back onto the canvas', () => {
    const frame = { x: 1800, y: 100, width: 200, height: 100 };
    const { result, onDraftChange } = setup(frame);

    result.current.beginMove(fakePointerEvent(1, 0, 0));
    result.current.onPointerMove(fakePointerEvent(1, 500, 0));

    const [[applied]] = onDraftChange.mock.calls as [[VisualDesignFrame]];
    expect(applied.x + applied.width).toBeLessThanOrEqual(CANVAS_LANDSCAPE.width);
  });

  it('resize via the "se" handle grows width and height', () => {
    const frame = { x: 100, y: 100, width: 200, height: 100 };
    const { result, onDraftChange } = setup(frame);

    result.current.beginResize(fakePointerEvent(1, 0, 0), 'se');
    result.current.onPointerMove(fakePointerEvent(1, 50, 30));

    expect(onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ width: 250, height: 130, x: 100, y: 100 }));
  });

  it('resize via the "nw" handle moves the anchor and shrinks toward it', () => {
    const frame = { x: 100, y: 100, width: 200, height: 100 };
    const { result, onDraftChange } = setup(frame);

    result.current.beginResize(fakePointerEvent(1, 0, 0), 'nw');
    result.current.onPointerMove(fakePointerEvent(1, 20, 10));

    expect(onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ x: 120, y: 110, width: 180, height: 90 }));
  });

  it('never shrinks a resize below the minimum layer size', () => {
    const frame = { x: 100, y: 100, width: 20, height: 20 };
    const { result, onDraftChange } = setup(frame);

    result.current.beginResize(fakePointerEvent(1, 0, 0), 'se');
    result.current.onPointerMove(fakePointerEvent(1, -500, -500));

    const [[applied]] = onDraftChange.mock.calls as [[VisualDesignFrame]];
    expect(applied.width).toBeGreaterThanOrEqual(8);
    expect(applied.height).toBeGreaterThanOrEqual(8);
  });

  it('does nothing when the layer is locked', () => {
    const frame = { x: 100, y: 100, width: 200, height: 100 };
    const { result, onDraftChange } = setup(frame, { locked: true });

    result.current.beginMove(fakePointerEvent(1, 0, 0));
    result.current.onPointerMove(fakePointerEvent(1, 50, 50));

    expect(onDraftChange).not.toHaveBeenCalled();
  });

  it('snaps to the canvas center when snapping is enabled and within threshold', () => {
    const width = 200;
    const centeredX = (CANVAS_LANDSCAPE.width - width) / 2;
    const frame = { x: centeredX - 20, y: 100, width, height: 100 };
    const { result, onDraftChange } = setup(frame, { snapping: true, snapThreshold: 12 });

    result.current.beginMove(fakePointerEvent(1, 0, 0));
    // Move 15 design units right - lands within 12 units of dead-center (5 away).
    result.current.onPointerMove(fakePointerEvent(1, 15, 0));

    const [[applied]] = onDraftChange.mock.calls as [[VisualDesignFrame]];
    expect(applied.x).toBe(Math.round(centeredX));
  });
});
