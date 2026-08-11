import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import type { ChatOverlayProfile } from '@/api/chat-overlay-schemas';
import type { VisualDesignDocument, VisualDesignLayer, VisualDesignLayerKind, VisualDesignResponse } from '@/api/visualdesign-schemas';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
// Reused unchanged from the Alert Designer (Stage 13B task Part 25:
// "avoid parallel AlertFoo / ChatFoo implementations for generic editor
// mechanics") - every one of these is already owner-agnostic.
import { DesignerCanvas } from '@/components/alert-designer/DesignerCanvas';
import { DesignerLayersPanel } from '@/components/alert-designer/DesignerLayersPanel';
import { DesignerPropertiesPanel } from '@/components/alert-designer/DesignerPropertiesPanel';
import { DesignerTopBar } from '@/components/alert-designer/DesignerTopBar';
import { chatItemDataContext } from '@/components/chat-overlay/chat-item-data-context';
import { useDeleteVisualDesignMutation, useSaveVisualDesignMutation } from '@/hooks/use-visual-design';
import { ApiError } from '@/lib/api-client';
import {
  CHAT_VISUAL_DESIGN_LAYER_KINDS,
  CHAT_VISUAL_DESIGN_TEXT_BINDINGS,
  VISUAL_DESIGN_LAYER_KINDS,
  createAvatarLayer,
  createBadgeListLayer,
  createHistory,
  createMessageFragmentsLayer,
  createPlatformIconLayer,
  createShapeLayer,
  createTextLayer,
  duplicateLayer,
  moveLayerOrder,
  normalizeLayerOrder,
  pushHistory,
  redoHistory,
  undoHistory,
  type History,
} from '@/models/visualdesign';

import { CHAT_PREVIEW_SCENARIOS, chatPreviewScenarioItem, type ChatPreviewScenario } from './preview-scenarios';

const ALL_CHAT_LAYER_KINDS: readonly VisualDesignLayerKind[] = [...VISUAL_DESIGN_LAYER_KINDS, ...CHAT_VISUAL_DESIGN_LAYER_KINDS];

function documentsEqual(a: VisualDesignDocument, b: VisualDesignDocument): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/**
 * The Chat Overlay Designer's own stateful editor (Stage 13B), a
 * structural mirror of AlertDesignerWorkspace.tsx reusing every generic
 * editor mechanic unchanged - selection, bounded undo/redo, zoom/
 * snapping, save/delete-with-revision-conflict, discard confirmation.
 * The only genuinely chat-specific pieces are: the fixed (not event-
 * type-driven) `availableBindings`, the two extra layer kinds, no
 * "Test Rule"-equivalent action (chat has no real-queue synthetic test
 * path the way alerts do), and preview scenarios built from ordinary
 * `PublicChatOverlayItem` fixtures reusing the exact same
 * `chatItemDataContext` mapping the real public overlay route uses.
 */
export function ChatOverlayDesignerWorkspace({
  overlay,
  initialResponse,
}: {
  overlay: ChatOverlayProfile;
  initialResponse: VisualDesignResponse;
}) {
  const { t } = useTranslation('alertDesigner');
  const { t: tChat } = useTranslation('chatOverlayDesigner');
  const { t: tOverlays } = useTranslation('overlays');
  const navigate = useNavigate();

  const [history, setHistory] = useState<History<VisualDesignDocument>>(() => createHistory(initialResponse.document));
  const [savedDocument, setSavedDocument] = useState(initialResponse.document);
  const [savedRevision, setSavedRevision] = useState(initialResponse.revision);
  const [persisted, setPersisted] = useState(initialResponse.persisted);
  const [selectedLayerId, setSelectedLayerId] = useState<string | null>(null);
  const [zoom, setZoom] = useState(1);
  const [snapping, setSnapping] = useState(true);
  const [scenario, setScenario] = useState<ChatPreviewScenario>('message');
  const [conflict, setConflict] = useState(false);
  const [resetConfirmOpen, setResetConfirmOpen] = useState(false);
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false);

  const document_ = history.present;
  const dirty = !documentsEqual(document_, savedDocument);
  const selectedLayer = document_.layers.find((l) => l.id === selectedLayerId) ?? null;

  const saveMutation = useSaveVisualDesignMutation('chat-overlays', overlay.id);
  const deleteMutation = useDeleteVisualDesignMutation('chat-overlays', overlay.id);

  useEffect(() => {
    if (!dirty) return;
    const handler = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [dirty]);

  function updateDraft(next: VisualDesignDocument) {
    setHistory((h) => ({ ...h, present: next }));
  }

  function commitDraft(next: VisualDesignDocument) {
    setHistory((h) => pushHistory(h, next));
  }

  function withLayers(mutator: (layers: VisualDesignLayer[]) => VisualDesignLayer[]) {
    commitDraft({ ...document_, layers: normalizeLayerOrder(mutator(document_.layers)) });
  }

  function handleAddLayer(kind: VisualDesignLayerKind) {
    const order = document_.layers.length;
    const frame = { x: 40, y: 40, width: document_.canvas.width - 80, height: 120 };
    const textFrame = { x: 40, y: 40, width: document_.canvas.width - 80, height: 60 };
    const smallFrame = { x: 40, y: 40, width: 64, height: 64 };
    const badgeFrame = { x: 40, y: 40, width: 160, height: 24 };
    let layer: VisualDesignLayer;
    switch (kind) {
      case 'shape':
        layer = createShapeLayer(frame, order);
        break;
      case 'text':
        layer = createTextLayer(textFrame, order, 'username');
        break;
      case 'platform_icon':
        layer = createPlatformIconLayer(smallFrame, order);
        break;
      case 'avatar':
        layer = createAvatarLayer(smallFrame, order);
        break;
      case 'message_fragments':
        layer = createMessageFragmentsLayer(textFrame, order);
        break;
      case 'badge_list':
        layer = createBadgeListLayer(badgeFrame, order);
        break;
    }
    withLayers((layers) => [...layers, layer]);
    setSelectedLayerId(layer.id);
  }

  function handleUpdateLayer(id: string, patch: Partial<VisualDesignLayer>) {
    withLayers((layers) => layers.map((l) => (l.id === id ? { ...l, ...patch } : l)));
  }

  function handleUpdateLayerDraft(id: string, patch: Partial<VisualDesignLayer>) {
    updateDraft({ ...document_, layers: document_.layers.map((l) => (l.id === id ? { ...l, ...patch } : l)) });
  }

  function handleDuplicateLayer(id: string) {
    const layer = document_.layers.find((l) => l.id === id);
    if (layer === undefined) return;
    const copy = duplicateLayer(layer, document_.canvas, document_.layers.length);
    withLayers((layers) => [...layers, copy]);
    setSelectedLayerId(copy.id);
  }

  function handleDeleteLayer(id: string) {
    withLayers((layers) => layers.filter((l) => l.id !== id));
    setSelectedLayerId((current) => (current === id ? null : current));
  }

  function handleMoveLayer(id: string, direction: 'up' | 'down' | 'front' | 'back') {
    commitDraft({ ...document_, layers: moveLayerOrder(document_.layers, id, direction) });
  }

  function handleSave() {
    setConflict(false);
    saveMutation.mutate(
      { document: document_, expectedRevision: persisted ? savedRevision : 0 },
      {
        onSuccess: (response) => {
          setSavedDocument(response.document);
          setSavedRevision(response.revision);
          setPersisted(true);
          setHistory(createHistory(response.document));
        },
        onError: (error) => {
          if (error instanceof ApiError && error.status === 409) setConflict(true);
        },
      },
    );
  }

  function handleReloadServerVersion() {
    navigate(0);
  }

  function handleResetToLegacy() {
    deleteMutation.mutate(undefined, {
      onSuccess: () => {
        setResetConfirmOpen(false);
        navigate('/overlays');
      },
    });
  }

  function handleBack() {
    if (dirty) {
      setDiscardConfirmOpen(true);
      return;
    }
    navigate('/overlays');
  }

  const fixture = useMemo(
    () => chatItemDataContext(chatPreviewScenarioItem(scenario), tOverlays),
    [scenario, tOverlays],
  );

  return (
    <div className="flex h-dvh flex-col bg-canvas" data-testid="chat-overlay-designer-workspace">
      <DesignerTopBar
        itemName={overlay.name}
        backLabel={tChat('page.backToOverlays')}
        dirty={dirty}
        saving={saveMutation.isPending}
        canUndo={history.past.length > 0}
        canRedo={history.future.length > 0}
        zoom={zoom}
        onZoomChange={setZoom}
        scenario={scenario}
        onScenarioChange={setScenario}
        scenarios={CHAT_PREVIEW_SCENARIOS}
        scenarioLabel={(s) => tChat(`preview.scenario.${s}`)}
        onBack={handleBack}
        onUndo={() => setHistory(undoHistory)}
        onRedo={() => setHistory(redoHistory)}
        onSave={handleSave}
        onResetToLegacy={() => setResetConfirmOpen(true)}
      />
      <div className="flex min-h-0 flex-1">
        <DesignerLayersPanel
          layers={document_.layers}
          layerKinds={ALL_CHAT_LAYER_KINDS}
          selectedLayerId={selectedLayerId}
          onSelect={setSelectedLayerId}
          onAddLayer={handleAddLayer}
          onToggleVisible={(id, visible) => handleUpdateLayer(id, { visible })}
          onToggleLocked={(id, locked) => handleUpdateLayer(id, { locked })}
          onDuplicate={handleDuplicateLayer}
          onDelete={handleDeleteLayer}
          onMove={handleMoveLayer}
        />
        <DesignerCanvas
          canvas={document_.canvas}
          layers={document_.layers}
          selectedLayerId={selectedLayerId}
          zoom={zoom}
          snapping={snapping}
          fixture={fixture}
          onSelect={setSelectedLayerId}
          onLayerDraftChange={handleUpdateLayerDraft}
          onLayerCommit={() => commitDraft(document_)}
        />
        <DesignerPropertiesPanel
          canvas={document_.canvas}
          onCanvasChange={(canvas) => commitDraft({ ...document_, canvas })}
          layer={selectedLayer}
          availableBindings={CHAT_VISUAL_DESIGN_TEXT_BINDINGS}
          onLayerChange={(patch) => selectedLayerId !== null && handleUpdateLayer(selectedLayerId, patch)}
          snapping={snapping}
          onSnappingChange={setSnapping}
        />
      </div>

      {conflict ? (
        <div
          className="fixed inset-x-0 bottom-0 z-50 flex items-center justify-between gap-3 border-t border-line bg-surface p-4 shadow-lg"
          role="alert"
          data-testid="revision-conflict-banner"
        >
          <div>
            <p className="text-sm font-medium text-ink">{t('revisionConflict.title')}</p>
            <p className="text-xs text-ink-muted">{t('revisionConflict.message')}</p>
          </div>
          <button
            type="button"
            className="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-white"
            onClick={handleReloadServerVersion}
          >
            {t('revisionConflict.reload')}
          </button>
        </div>
      ) : null}

      <ConfirmDialog
        open={resetConfirmOpen}
        title={t('resetToLegacy.confirmTitle')}
        message={t('resetToLegacy.confirmMessage')}
        confirmLabel={t('resetToLegacy.confirmAction')}
        onConfirm={handleResetToLegacy}
        onCancel={() => setResetConfirmOpen(false)}
        destructive
        busy={deleteMutation.isPending}
      />

      <ConfirmDialog
        open={discardConfirmOpen}
        title={t('discard.confirmTitle')}
        message={t('discard.confirmMessage')}
        confirmLabel={t('discard.confirmAction')}
        onConfirm={() => navigate('/overlays')}
        onCancel={() => setDiscardConfirmOpen(false)}
        destructive
      />
    </div>
  );
}
