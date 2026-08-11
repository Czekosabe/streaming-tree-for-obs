import { useRef } from 'react';

import type { VisualDesignCanvas, VisualDesignFrame } from '@/api/visualdesign-schemas';
import { clampFrameToCanvas, MIN_LAYER_SIZE, snapFramePosition } from '@/models/visualdesign';

/**
 * Pointer-based move/resize for one selected layer (Stage 13A task
 * Part 29/30) - a brand-new primitive, no in-repo precedent to build
 * on (this codebase had no drag/pointer-capture code anywhere before
 * Stage 13A). Deliberately never the ONLY way to reposition a layer -
 * the property panel's own numeric X/Y/Width/Height inputs
 * (DesignerPropertiesPanel.tsx) always work too, satisfying Part 29's
 * "pointer dragging must never be the only way."
 *
 * `onDraftChange` fires continuously while dragging (local draft
 * update only, never a new undo step per pixel); `onCommit` fires
 * once on pointer up, creating exactly one undo step for the whole
 * gesture (Part 35: "pointer drag creates one undo step at
 * completion, not hundreds").
 */
export type ResizeHandle = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw';

type DragState = {
  pointerId: number;
  startClientX: number;
  startClientY: number;
  startFrame: VisualDesignFrame;
  mode: { type: 'move' } | { type: 'resize'; handle: ResizeHandle };
};

export function useDragResize({
  frame,
  canvas,
  scale,
  snapping,
  snapThreshold,
  locked,
  onDraftChange,
  onCommit,
}: {
  frame: VisualDesignFrame;
  canvas: VisualDesignCanvas;
  scale: number;
  snapping: boolean;
  snapThreshold: number;
  locked: boolean;
  onDraftChange: (frame: VisualDesignFrame) => void;
  onCommit: () => void;
}) {
  const stateRef = useRef<DragState | null>(null);

  function computeNextFrame(dxDesign: number, dyDesign: number, state: DragState): VisualDesignFrame {
    const { startFrame, mode } = state;
    let next: VisualDesignFrame = { ...startFrame };

    if (mode.type === 'move') {
      next = { ...startFrame, x: startFrame.x + dxDesign, y: startFrame.y + dyDesign };
    } else {
      const h = mode.handle;
      let { x, y, width, height } = startFrame;
      if (h.includes('e')) width = startFrame.width + dxDesign;
      if (h.includes('w')) {
        width = startFrame.width - dxDesign;
        x = startFrame.x + dxDesign;
      }
      if (h.includes('s')) height = startFrame.height + dyDesign;
      if (h.includes('n')) {
        height = startFrame.height - dyDesign;
        y = startFrame.y + dyDesign;
      }
      // Never let a resize handle collapse the frame past the minimum
      // size or drag the anchored edge past its own opposite edge.
      if (width < MIN_LAYER_SIZE) {
        if (h.includes('w')) x = startFrame.x + startFrame.width - MIN_LAYER_SIZE;
        width = MIN_LAYER_SIZE;
      }
      if (height < MIN_LAYER_SIZE) {
        if (h.includes('n')) y = startFrame.y + startFrame.height - MIN_LAYER_SIZE;
        height = MIN_LAYER_SIZE;
      }
      next = { x, y, width, height };
    }

    if (snapping && mode.type === 'move') {
      const snapped = snapFramePosition(next, canvas, snapThreshold);
      next = { ...next, x: snapped.x, y: snapped.y };
    }

    return clampFrameToCanvas(next, canvas);
  }

  function beginMove(e: React.PointerEvent) {
    if (locked) return;
    e.stopPropagation();
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
    stateRef.current = { pointerId: e.pointerId, startClientX: e.clientX, startClientY: e.clientY, startFrame: frame, mode: { type: 'move' } };
  }

  function beginResize(e: React.PointerEvent, handle: ResizeHandle) {
    if (locked) return;
    e.stopPropagation();
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
    stateRef.current = { pointerId: e.pointerId, startClientX: e.clientX, startClientY: e.clientY, startFrame: frame, mode: { type: 'resize', handle } };
  }

  function onPointerMove(e: React.PointerEvent) {
    const state = stateRef.current;
    if (state === null || e.pointerId !== state.pointerId) return;
    const dxDesign = (e.clientX - state.startClientX) / scale;
    const dyDesign = (e.clientY - state.startClientY) / scale;
    onDraftChange(computeNextFrame(dxDesign, dyDesign, state));
  }

  function endDrag(e: React.PointerEvent) {
    const state = stateRef.current;
    if (state === null || e.pointerId !== state.pointerId) return;
    stateRef.current = null;
    onCommit();
  }

  return { beginMove, beginResize, onPointerMove, onPointerUp: endDrag, onPointerCancel: endDrag };
}
