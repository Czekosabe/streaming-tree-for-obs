import { useTranslation } from 'react-i18next';

import { Button, IconButton } from '@/components/ui/Button';
import { SelectInput } from '@/components/ui/SelectInput';
import { ZOOM_LEVELS_ARRAY } from '@/models/visualdesign';

import { PREVIEW_SCENARIOS, type PreviewScenario } from './preview-scenarios';

/**
 * The Designer's own top bar (Stage 13A task Part 26; shared by both
 * designers as of Stage 13B task Part 25): Back, name, unsaved
 * indicator, Undo/Redo, Fit/zoom, preview scenario, Save, Reset to
 * legacy, and an owner-specific "test" action (Test Rule for alerts;
 * chat has no equivalent live-queue action, so the Chat Overlay
 * Designer omits it by passing no `onTestAction`).
 */
export function DesignerTopBar({
  itemName,
  backLabel,
  dirty,
  saving,
  canUndo,
  canRedo,
  zoom,
  onZoomChange,
  scenario,
  onScenarioChange,
  onBack,
  onUndo,
  onRedo,
  onSave,
  onResetToLegacy,
  testAction,
}: {
  itemName: string;
  backLabel: string;
  dirty: boolean;
  saving: boolean;
  canUndo: boolean;
  canRedo: boolean;
  zoom: number;
  onZoomChange: (zoom: number) => void;
  scenario: PreviewScenario;
  onScenarioChange: (scenario: PreviewScenario) => void;
  onBack: () => void;
  onUndo: () => void;
  onRedo: () => void;
  onSave: () => void;
  onResetToLegacy: () => void;
  /** Owner-specific action (e.g. alerts' own "Test Rule", which goes
   * through the real backend queue and always uses the last SAVED
   * design). Omitted entirely (no button rendered) when the owner has
   * no equivalent action. */
  testAction?: { label: string; onClick: () => void; pending: boolean; succeeded: boolean } | undefined;
}) {
  const { t } = useTranslation('alertDesigner');

  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface px-3 py-2" data-testid="designer-topbar">
      <Button variant="ghost" onClick={onBack} data-testid="designer-back">
        {backLabel}
      </Button>
      <span className="ml-1 text-sm font-medium text-ink" data-testid="designer-rule-name">{itemName}</span>
      <span
        className="text-xs text-ink-muted"
        data-testid="designer-dirty-indicator"
        data-dirty={dirty}
      >
        {dirty ? t('topbar.unsaved') : t('topbar.saved')}
      </span>

      <div className="mx-2 h-5 w-px bg-line" aria-hidden />

      <IconButton label={t('topbar.undo')} icon="↶" onClick={onUndo} disabled={!canUndo} data-testid="designer-undo" />
      <IconButton label={t('topbar.redo')} icon="↷" onClick={onRedo} disabled={!canRedo} data-testid="designer-redo" />

      <div className="mx-2 h-5 w-px bg-line" aria-hidden />

      <Button variant="ghost" onClick={() => onZoomChange(1)} data-testid="designer-fit">
        {t('topbar.fit')}
      </Button>
      <label className="flex items-center gap-1 text-xs text-ink-muted">
        {t('topbar.zoom')}
        <select
          className="rounded border border-line bg-surface px-1 py-0.5 text-xs"
          value={zoom}
          onChange={(e) => onZoomChange(Number(e.target.value))}
          data-testid="designer-zoom-select"
        >
          {ZOOM_LEVELS_ARRAY.map((level) => (
            <option key={level} value={level}>
              {Math.round(level * 100)}%
            </option>
          ))}
        </select>
      </label>

      <div className="mx-2 h-5 w-px bg-line" aria-hidden />

      <label className="flex items-center gap-1 text-xs text-ink-muted">
        {t('preview.title')}
        <SelectInput
          value={scenario}
          onChange={(e) => onScenarioChange(e.target.value as PreviewScenario)}
          options={PREVIEW_SCENARIOS.map((s) => ({ value: s, label: t(`preview.scenario.${s}`) }))}
          data-testid="designer-scenario-select"
        />
      </label>

      <div className="ml-auto flex items-center gap-2">
        {testAction !== undefined ? (
          <Button variant="ghost" onClick={testAction.onClick} disabled={testAction.pending} data-testid="designer-test-rule">
            {testAction.succeeded ? '✓' : null} {testAction.label}
          </Button>
        ) : null}
        <Button variant="danger" onClick={onResetToLegacy} data-testid="designer-reset-to-legacy">
          {t('topbar.resetToLegacy')}
        </Button>
        <Button variant="primary" onClick={onSave} disabled={saving || !dirty} data-testid="designer-save">
          {saving ? t('topbar.saving') : t('topbar.save')}
        </Button>
      </div>
    </div>
  );
}
