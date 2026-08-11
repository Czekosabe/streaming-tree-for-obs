import { useTranslation } from 'react-i18next';

import type { VisualDesignLayer, VisualDesignLayerKind } from '@/api/visualdesign-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { cn } from '@/lib/cn';
import { MAX_LAYERS } from '@/models/visualdesign';

const LAYER_KINDS: VisualDesignLayerKind[] = ['shape', 'text', 'platform_icon', 'avatar'];

/** Left panel: Add Layer and the ordered layers list (Stage 13A task
 * Part 27/32/33/34) - front-to-back visually, so the topmost list
 * entry is the frontmost layer. Every action (reorder/show-hide/lock/
 * duplicate/delete) is a labeled button, never drag-only, satisfying
 * Part 32's "keyboard/button reordering must exist even if drag
 * reorder exists" (this implementation offers only button reordering -
 * drag-to-reorder is explicitly optional per the same part). */
export function DesignerLayersPanel({
  layers,
  selectedLayerId,
  onSelect,
  onAddLayer,
  onToggleVisible,
  onToggleLocked,
  onDuplicate,
  onDelete,
  onMove,
}: {
  layers: VisualDesignLayer[];
  selectedLayerId: string | null;
  onSelect: (id: string) => void;
  onAddLayer: (kind: VisualDesignLayerKind) => void;
  onToggleVisible: (id: string, visible: boolean) => void;
  onToggleLocked: (id: string, locked: boolean) => void;
  onDuplicate: (id: string) => void;
  onDelete: (id: string) => void;
  onMove: (id: string, direction: 'up' | 'down' | 'front' | 'back') => void;
}) {
  const { t } = useTranslation('alertDesigner');
  const frontToBack = [...layers].sort((a, b) => b.order - a.order);

  return (
    <div className="flex w-64 shrink-0 flex-col border-r border-line bg-surface" data-testid="designer-layers-panel">
      <div className="border-b border-line p-2">
        <p className="mb-2 text-xs font-semibold uppercase text-ink-muted">{t('layers.addLayer')}</p>
        <div className="grid grid-cols-2 gap-1">
          {LAYER_KINDS.map((kind) => (
            <Button
              key={kind}
              variant="secondary"
              onClick={() => onAddLayer(kind)}
              disabled={layers.length >= MAX_LAYERS}
              data-testid={`designer-add-${kind}`}
            >
              {t(`layers.kind.${kind}`)}
            </Button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-2">
        <p className="mb-1 text-xs font-semibold uppercase text-ink-muted">{t('layers.title')}</p>
        {frontToBack.length === 0 ? (
          <p className="text-xs text-ink-muted">{t('layers.empty')}</p>
        ) : (
          <ul className="space-y-1" data-testid="designer-layers-list">
            {frontToBack.map((layer) => (
              <li key={layer.id}>
                <div
                  role="button"
                  tabIndex={0}
                  onClick={() => onSelect(layer.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') onSelect(layer.id);
                  }}
                  className={cn(
                    'flex items-center gap-1 rounded-md border px-2 py-1.5 text-xs',
                    layer.id === selectedLayerId ? 'border-accent bg-accent/10' : 'border-transparent hover:bg-surface-sunken',
                  )}
                  data-testid="designer-layer-row"
                  data-layer-id={layer.id}
                  data-selected={layer.id === selectedLayerId}
                >
                  <span className="flex-1 truncate">{layer.name}</span>
                  <IconButton
                    label={layer.visible ? t('layers.hide') : t('layers.show')}
                    icon={layer.visible ? '👁' : '🚫'}
                    onClick={(e) => {
                      e.stopPropagation();
                      onToggleVisible(layer.id, !layer.visible);
                    }}
                    data-testid="designer-layer-toggle-visible"
                  />
                  <IconButton
                    label={layer.locked ? t('layers.unlock') : t('layers.lock')}
                    icon={layer.locked ? '🔒' : '🔓'}
                    onClick={(e) => {
                      e.stopPropagation();
                      onToggleLocked(layer.id, !layer.locked);
                    }}
                    data-testid="designer-layer-toggle-locked"
                  />
                  <IconButton
                    label={t('layers.moveUp')}
                    icon="↑"
                    onClick={(e) => {
                      e.stopPropagation();
                      onMove(layer.id, 'up');
                    }}
                    data-testid="designer-layer-move-up"
                  />
                  <IconButton
                    label={t('layers.moveDown')}
                    icon="↓"
                    onClick={(e) => {
                      e.stopPropagation();
                      onMove(layer.id, 'down');
                    }}
                    data-testid="designer-layer-move-down"
                  />
                  <IconButton
                    label={t('layers.moveToFront')}
                    icon="⤒"
                    onClick={(e) => {
                      e.stopPropagation();
                      onMove(layer.id, 'front');
                    }}
                    data-testid="designer-layer-move-front"
                  />
                  <IconButton
                    label={t('layers.moveToBack')}
                    icon="⤓"
                    onClick={(e) => {
                      e.stopPropagation();
                      onMove(layer.id, 'back');
                    }}
                    data-testid="designer-layer-move-back"
                  />
                  <IconButton
                    label={t('layers.duplicate')}
                    icon="⧉"
                    onClick={(e) => {
                      e.stopPropagation();
                      onDuplicate(layer.id);
                    }}
                    data-testid="designer-layer-duplicate"
                  />
                  <IconButton
                    label={t('layers.delete')}
                    icon="🗑"
                    onClick={(e) => {
                      e.stopPropagation();
                      onDelete(layer.id);
                    }}
                    data-testid="designer-layer-delete"
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
