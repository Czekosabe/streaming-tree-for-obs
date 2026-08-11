import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import type { AlertEventTypeCapability, AlertProfile, AlertRule } from '@/api/alerts-schemas';
import type { VisualDesignDocument, VisualDesignLayer, VisualDesignResponse } from '@/api/visualdesign-schemas';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { useAlertPreviewMutation, useTestAlertRuleMutation } from '@/hooks/use-alerts';
import { useDeleteVisualDesignMutation, useSaveVisualDesignMutation } from '@/hooks/use-visual-design';
import { ApiError } from '@/lib/api-client';
import {
  createHistory,
  createAvatarLayer,
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

import { DesignerCanvas } from './DesignerCanvas';
import { DesignerLayersPanel } from './DesignerLayersPanel';
import { DesignerPropertiesPanel } from './DesignerPropertiesPanel';
import { DesignerTopBar } from './DesignerTopBar';
import { baseEventTypeForScenario, previewScenarioFixture, type PreviewScenario } from './preview-scenarios';

function documentsEqual(a: VisualDesignDocument, b: VisualDesignDocument): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/**
 * The Alert Overlay Designer's own stateful editor (Stage 13A task
 * Part 26 onward). Owns the bounded undo/redo history, selection,
 * zoom/snapping, the deterministic local preview scenario, and the
 * save/delete/Test-Rule mutations. Every layer mutation goes through
 * `commitDraft` (one undo step) except live pointer-drag/numeric-typing
 * updates, which go through `updateDraft` (no history entry) until the
 * gesture completes (Stage 13A task Part 35).
 */
export function AlertDesignerWorkspace({
  rule,
  profile,
  eventTypeCapability,
  initialResponse,
}: {
  rule: AlertRule;
  profile: AlertProfile;
  eventTypeCapability: AlertEventTypeCapability | undefined;
  initialResponse: VisualDesignResponse;
}) {
  const { t } = useTranslation('alertDesigner');
  const navigate = useNavigate();

  const [history, setHistory] = useState<History<VisualDesignDocument>>(() => createHistory(initialResponse.document));
  const [savedDocument, setSavedDocument] = useState(initialResponse.document);
  const [savedRevision, setSavedRevision] = useState(initialResponse.revision);
  const [persisted, setPersisted] = useState(initialResponse.persisted);
  const [selectedLayerId, setSelectedLayerId] = useState<string | null>(null);
  const [zoom, setZoom] = useState(1);
  const [snapping, setSnapping] = useState(true);
  const [scenario, setScenario] = useState<PreviewScenario>('follow');
  const [conflict, setConflict] = useState(false);
  const [resetConfirmOpen, setResetConfirmOpen] = useState(false);
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false);

  const document_ = history.present;
  const dirty = !documentsEqual(document_, savedDocument);
  const selectedLayer = document_.layers.find((l) => l.id === selectedLayerId) ?? null;

  const saveMutation = useSaveVisualDesignMutation(rule.id);
  const deleteMutation = useDeleteVisualDesignMutation(rule.id);
  const testRuleMutation = useTestAlertRuleMutation();
  const previewMutation = useAlertPreviewMutation();

  const baseEventType = baseEventTypeForScenario(scenario);
  useEffect(() => {
    previewMutation.mutate({ template: rule.textTemplate, eventType: baseEventType, language: profile.language });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- re-render the fixture only when the scenario's own base event type or the rule's saved template changes
  }, [baseEventType, rule.textTemplate]);

  // A real browser tab close/refresh with unsaved changes gets the
  // native confirmation prompt; the in-app "Back" button below shows
  // its own ConfirmDialog for the normal in-app navigation case (Stage
  // 13A task Part 35: "navigating away with unsaved changes requires
  // confirmation").
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

  function handleAddLayer(kind: 'shape' | 'text' | 'platform_icon' | 'avatar') {
    const order = document_.layers.length;
    const frame = { x: 100, y: 100, width: 400, height: 200 };
    const textFrame = { x: 100, y: 100, width: 400, height: 100 };
    const smallFrame = { x: 100, y: 100, width: 96, height: 96 };
    const layer: VisualDesignLayer =
      kind === 'shape'
        ? createShapeLayer(frame, order)
        : kind === 'text'
          ? createTextLayer(textFrame, order)
          : kind === 'platform_icon'
            ? createPlatformIconLayer(smallFrame, order)
            : createAvatarLayer(smallFrame, order);
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
    // A fresh reload is the simplest correct way to reload the server's
    // own current version - never a silent client-side merge (Part 41).
    navigate(0);
  }

  function handleResetToLegacy() {
    deleteMutation.mutate(undefined, {
      onSuccess: () => {
        setResetConfirmOpen(false);
        navigate('/alerts');
      },
    });
  }

  function handleBack() {
    if (dirty) {
      setDiscardConfirmOpen(true);
      return;
    }
    navigate('/alerts');
  }

  const fixture = useMemo(() => {
    const base = previewScenarioFixture(scenario);
    return { ...base, renderedText: previewMutation.data?.renderedText ?? rule.textTemplate };
  }, [scenario, previewMutation.data, rule.textTemplate]);

  return (
    <div className="flex h-dvh flex-col bg-canvas" data-testid="alert-designer-workspace">
      <DesignerTopBar
        ruleName={rule.name}
        dirty={dirty}
        saving={saveMutation.isPending}
        canUndo={history.past.length > 0}
        canRedo={history.future.length > 0}
        zoom={zoom}
        onZoomChange={setZoom}
        scenario={scenario}
        onScenarioChange={setScenario}
        onBack={handleBack}
        onUndo={() => setHistory(undoHistory)}
        onRedo={() => setHistory(redoHistory)}
        onSave={handleSave}
        onResetToLegacy={() => setResetConfirmOpen(true)}
        onTestRule={() => testRuleMutation.mutate({ id: rule.id })}
        testRulePending={testRuleMutation.isPending}
        testRuleSucceeded={testRuleMutation.isSuccess}
      />
      <div className="flex min-h-0 flex-1">
        <DesignerLayersPanel
          layers={document_.layers}
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
          eventTypeCapability={eventTypeCapability}
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
        onConfirm={() => navigate('/alerts')}
        onCancel={() => setDiscardConfirmOpen(false)}
        destructive
      />
    </div>
  );
}
