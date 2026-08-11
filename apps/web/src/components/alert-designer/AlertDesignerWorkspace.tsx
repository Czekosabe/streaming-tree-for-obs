import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import type { AlertEventTypeCapability, AlertProfile, AlertRule } from '@/api/alerts-schemas';
import type { VisualAsset } from '@/api/visualasset-schemas';
import type { VisualDesignDocument, VisualDesignLayer, VisualDesignLayerKind, VisualDesignResponse } from '@/api/visualdesign-schemas';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { platformDisplayName } from '@/components/visual-design/text-binding';
import { VisualAssetPicker } from '@/components/visual-design/VisualAssetPicker';
import { TemplateGallery } from '@/components/visual-templates/TemplateGallery';
import { useAlertPreviewMutation, useTestAlertRuleMutation } from '@/hooks/use-alerts';
import { useVisualAssetMap } from '@/hooks/use-visual-assets';
import { useDeleteVisualDesignMutation, useSaveVisualDesignMutation } from '@/hooks/use-visual-design';
import { ApiError } from '@/lib/api-client';
import {
  availableTextBindings,
  createHistory,
  createAvatarLayer,
  createImageLayer,
  createPlatformIconLayer,
  createShapeLayer,
  createTextLayer,
  createVideoLayer,
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
import { baseEventTypeForScenario, PREVIEW_SCENARIOS, previewScenarioFixture, type PreviewScenario } from './preview-scenarios';

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
  const { t: tAlerts } = useTranslation('alerts');
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
  const [templatesOpen, setTemplatesOpen] = useState(false);
  const [pendingAssetLayerKind, setPendingAssetLayerKind] = useState<'image' | 'video' | null>(null);

  const document_ = history.present;
  const dirty = !documentsEqual(document_, savedDocument);
  const selectedLayer = document_.layers.find((l) => l.id === selectedLayerId) ?? null;
  const assetMap = useVisualAssetMap();

  const saveMutation = useSaveVisualDesignMutation('alert-rules', rule.id);
  const deleteMutation = useDeleteVisualDesignMutation('alert-rules', rule.id);
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

  function handleAddLayer(kind: VisualDesignLayerKind) {
    // The Alert Designer's own DesignerLayersPanel only ever offers the
    // six shared kinds (VISUAL_DESIGN_LAYER_KINDS, its default
    // layerKinds) - message_fragments/badge_list are structurally
    // unreachable from this UI, kept here only so this handler's own
    // parameter type matches the shared, owner-agnostic
    // DesignerLayersPanel prop exactly (Stage 13B task Part 25).
    if (kind === 'message_fragments' || kind === 'badge_list') return;
    // image/video need a real managed asset chosen first (Stage 14B) -
    // the layer itself is only created once the picker reports a
    // selection, see handleAssetChosenForNewLayer.
    if (kind === 'image' || kind === 'video') {
      setPendingAssetLayerKind(kind);
      return;
    }
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

  function handleAssetChosenForNewLayer(asset: VisualAsset) {
    if (pendingAssetLayerKind === null) return;
    const order = document_.layers.length;
    const frame = { x: 100, y: 100, width: 400, height: 200 };
    const layer =
      pendingAssetLayerKind === 'image' ? createImageLayer(frame, order, asset.id) : createVideoLayer(frame, order, asset.id);
    withLayers((layers) => [...layers, layer]);
    setSelectedLayerId(layer.id);
    setPendingAssetLayerKind(null);
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
    const baseEventType = baseEventTypeForScenario(scenario);
    return {
      ...base,
      bindings: {
        ...base.bindings,
        renderedText: previewMutation.data?.renderedText ?? rule.textTemplate,
        platform: platformDisplayName(base.providerId),
        eventType: tAlerts(`rules.eventType.${baseEventType}`, { defaultValue: baseEventType }),
      },
    };
  }, [scenario, previewMutation.data, rule.textTemplate, tAlerts]);

  return (
    <div className="flex h-dvh flex-col bg-canvas" data-testid="alert-designer-workspace">
      <DesignerTopBar
        itemName={rule.name}
        backLabel={t('page.backToRules')}
        dirty={dirty}
        saving={saveMutation.isPending}
        canUndo={history.past.length > 0}
        canRedo={history.future.length > 0}
        zoom={zoom}
        onZoomChange={setZoom}
        scenario={scenario}
        onScenarioChange={setScenario}
        scenarios={PREVIEW_SCENARIOS}
        scenarioLabel={(s) => t(`preview.scenario.${s}`)}
        onBack={handleBack}
        onUndo={() => setHistory(undoHistory)}
        onRedo={() => setHistory(redoHistory)}
        onSave={handleSave}
        onResetToLegacy={() => setResetConfirmOpen(true)}
        onOpenTemplates={() => setTemplatesOpen(true)}
        testAction={{
          label: 'Test Rule',
          onClick: () => testRuleMutation.mutate({ id: rule.id }),
          pending: testRuleMutation.isPending,
          succeeded: testRuleMutation.isSuccess,
        }}
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
          assetMap={assetMap}
          onSelect={setSelectedLayerId}
          onLayerDraftChange={handleUpdateLayerDraft}
          onLayerCommit={() => commitDraft(document_)}
        />
        <DesignerPropertiesPanel
          canvas={document_.canvas}
          onCanvasChange={(canvas) => commitDraft({ ...document_, canvas })}
          layer={selectedLayer}
          availableBindings={availableTextBindings(eventTypeCapability)}
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

      <TemplateGallery
        open={templatesOpen}
        onClose={() => setTemplatesOpen(false)}
        target="alert"
        ownerId={rule.id}
        draftIsDirty={dirty}
        currentDraftDocument={document_}
        onUseAsDraft={(doc) => commitDraft(doc)}
      />

      {pendingAssetLayerKind !== null && (
        <VisualAssetPicker
          open
          onClose={() => setPendingAssetLayerKind(null)}
          kind={pendingAssetLayerKind}
          onSelect={handleAssetChosenForNewLayer}
        />
      )}
    </div>
  );
}
