import { useEffect } from 'react';

import type { VisualDesignCanvas, VisualDesignFrame, VisualDesignLayer } from '@/api/visualdesign-schemas';
import type { RenderableLayer, VisualAssetMap } from '@/components/visual-design/VisualLayer';
import { VisualDesignRenderer, type VisualDesignDataContext } from '@/components/visual-design/VisualDesignRenderer';
import { useDragResize } from '@/hooks/use-drag-resize';
import { clampFrameToCanvas } from '@/models/visualdesign';

const SNAP_THRESHOLD_DESIGN_UNITS = 12;
const KEYBOARD_NUDGE = 1;
const KEYBOARD_NUDGE_LARGE = 10;

const HANDLES: Array<{ handle: 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'; className: string; cursor: string }> = [
  { handle: 'nw', className: 'left-0 top-0 -translate-x-1/2 -translate-y-1/2', cursor: 'nwse-resize' },
  { handle: 'n', className: 'left-1/2 top-0 -translate-x-1/2 -translate-y-1/2', cursor: 'ns-resize' },
  { handle: 'ne', className: 'right-0 top-0 translate-x-1/2 -translate-y-1/2', cursor: 'nesw-resize' },
  { handle: 'e', className: 'right-0 top-1/2 translate-x-1/2 -translate-y-1/2', cursor: 'ew-resize' },
  { handle: 'se', className: 'right-0 bottom-0 translate-x-1/2 translate-y-1/2', cursor: 'nwse-resize' },
  { handle: 's', className: 'left-1/2 bottom-0 -translate-x-1/2 translate-y-1/2', cursor: 'ns-resize' },
  { handle: 'sw', className: 'left-0 bottom-0 -translate-x-1/2 translate-y-1/2', cursor: 'nesw-resize' },
  { handle: 'w', className: 'left-0 top-1/2 -translate-x-1/2 -translate-y-1/2', cursor: 'ew-resize' },
];

/**
 * The center canvas workspace (Stage 13A task Part 26/29/30/36):
 * renders the shared `VisualDesignRenderer` with a `chrome` render-prop
 * adding selection outline/resize handles/pointer-drag around each
 * layer, plus keyboard nudging for the selected layer. Every geometry
 * change here is available identically through the numeric X/Y/Width/
 * Height inputs in the properties panel - pointer interaction is never
 * the only way (Part 29/30).
 */
export function DesignerCanvas({
  canvas,
  layers,
  selectedLayerId,
  zoom,
  snapping,
  fixture,
  assetMap,
  onSelect,
  onLayerDraftChange,
  onLayerCommit,
}: {
  canvas: VisualDesignCanvas;
  layers: VisualDesignLayer[];
  selectedLayerId: string | null;
  zoom: number;
  snapping: boolean;
  fixture: VisualDesignDataContext;
  /** Resolves an image/video layer's or a custom-font reference's local
   * managed-asset id into a safe URL for the editor's own preview
   * (Stage 14B task Part 42). */
  assetMap?: VisualAssetMap | undefined;
  onSelect: (id: string | null) => void;
  onLayerDraftChange: (id: string, patch: Partial<VisualDesignLayer>) => void;
  onLayerCommit: () => void;
}) {
  const selectedLayer = layers.find((l) => l.id === selectedLayerId) ?? null;

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (selectedLayer === null || selectedLayer.locked) return;
      const target = e.target as HTMLElement | null;
      if (target !== null && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return;

      const step = e.shiftKey ? KEYBOARD_NUDGE_LARGE : KEYBOARD_NUDGE;
      let dx = 0;
      let dy = 0;
      if (e.key === 'ArrowLeft') dx = -step;
      else if (e.key === 'ArrowRight') dx = step;
      else if (e.key === 'ArrowUp') dy = -step;
      else if (e.key === 'ArrowDown') dy = step;
      else return;

      e.preventDefault();
      const next = clampFrameToCanvas(
        { ...selectedLayer.frame, x: selectedLayer.frame.x + dx, y: selectedLayer.frame.y + dy },
        canvas,
      );
      onLayerDraftChange(selectedLayer.id, { frame: next });
      onLayerCommit();
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [selectedLayer, canvas, onLayerDraftChange, onLayerCommit]);

  return (
    <div
      className="relative min-w-0 flex-1 overflow-auto bg-[repeating-conic-gradient(#00000010_0%_25%,transparent_0%_50%)] bg-[length:20px_20px] p-6"
      data-testid="designer-canvas-workspace"
      onClick={() => onSelect(null)}
    >
      <div
        style={{ width: canvas.width * zoom, height: canvas.height * zoom, margin: '0 auto' }}
        className="relative bg-black/5"
        data-testid="designer-canvas-frame"
      >
        <VisualDesignRenderer
          canvas={canvas}
          layers={layers as RenderableLayer[]}
          dataContext={fixture}
          mode="preview"
          prefersReducedMotion={false}
          assetMap={assetMap}
          chrome={(layer, scale, children) => (
            <SelectableLayer
              key={layer.id}
              layer={layer}
              frame={layer.frame}
              canvas={canvas}
              scale={scale}
              selected={layer.id === selectedLayerId}
              locked={findLocked(layers, layer.id)}
              onSelect={() => onSelect(layer.id)}
              onDraftChange={(frame) => onLayerDraftChange(layer.id, { frame })}
              onCommit={onLayerCommit}
              snapping={snapping}
            >
              {children}
            </SelectableLayer>
          )}
        />
      </div>
    </div>
  );
}

function findLocked(layers: VisualDesignLayer[], id: string): boolean {
  return layers.find((l) => l.id === id)?.locked ?? false;
}

function SelectableLayer({
  layer,
  frame,
  canvas,
  scale,
  selected,
  locked,
  onSelect,
  onDraftChange,
  onCommit,
  snapping,
  children,
}: {
  layer: RenderableLayer;
  frame: VisualDesignFrame;
  canvas: VisualDesignCanvas;
  scale: number;
  selected: boolean;
  locked: boolean;
  onSelect: () => void;
  onDraftChange: (frame: VisualDesignFrame) => void;
  onCommit: () => void;
  snapping: boolean;
  children: React.ReactNode;
}) {
  const drag = useDragResize({
    frame,
    canvas,
    scale,
    snapping,
    snapThreshold: SNAP_THRESHOLD_DESIGN_UNITS,
    locked,
    onDraftChange,
    onCommit,
  });

  return (
    <div
      className="absolute"
      style={{ left: frame.x * scale, top: frame.y * scale, width: frame.width * scale, height: frame.height * scale }}
      onClick={(e) => {
        e.stopPropagation();
        onSelect();
      }}
      onPointerDown={drag.beginMove}
      onPointerMove={drag.onPointerMove}
      onPointerUp={drag.onPointerUp}
      onPointerCancel={drag.onPointerCancel}
      data-testid="designer-selectable-layer"
      data-layer-id={layer.id}
      data-selected={selected}
    >
      <div className="pointer-events-none absolute inset-0">{children}</div>
      {selected ? (
        <>
          <div className="pointer-events-none absolute inset-0 border-2 border-accent" />
          {!locked
            ? HANDLES.map(({ handle, className, cursor }) => (
                <div
                  key={handle}
                  className={`absolute size-3 rounded-full border border-accent bg-white ${className}`}
                  style={{ cursor }}
                  onPointerDown={(e) => drag.beginResize(e, handle)}
                  onPointerMove={drag.onPointerMove}
                  onPointerUp={drag.onPointerUp}
                  onPointerCancel={drag.onPointerCancel}
                  data-testid="designer-resize-handle"
                  data-handle={handle}
                />
              ))
            : null}
        </>
      ) : null}
    </div>
  );
}
