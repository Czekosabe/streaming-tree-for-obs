import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import * as alertsApi from '@/api/alerts';
import type { AlertProfile, AlertRule } from '@/api/alerts-schemas';
import type { VisualDesignResponse } from '@/api/visualdesign-schemas';
import * as visualDesignApi from '@/api/visualdesign';
import * as visualTemplateApi from '@/api/visualtemplate';
import { renderWithProviders } from '@/test/render';

import { AlertDesignerWorkspace } from './AlertDesignerWorkspace';

vi.mock('@/api/alerts');
vi.mock('@/api/visualdesign');
vi.mock('@/api/visualtemplate');

const rule: AlertRule = {
  id: 'alrule_1', profileId: 'alprof_1', name: 'Follow alert', enabled: true, eventType: 'follow',
  priority: 50, durationMs: 5000, requiredRole: 'everyone', showPlatform: true, showUsername: true,
  showMessage: false, showQuantity: false, textTemplate: '{username} followed!', entryAnimation: 'fade',
  exitAnimation: 'fade', animationDurationMs: 400, providers: [], accounts: [],
  allowGrouping: false, groupWindowMs: 5000, interruptMode: 'never', interruptible: true,
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
};

const profile: AlertProfile = {
  id: 'alprof_1', publicSlug: 'slug', name: 'Main', enabled: true, language: 'en', theme: 'minimal',
  position: 'bottom', textAlign: 'center', maxQueueItems: 100, maximumQueueAgeSeconds: 120,
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
};

function draftResponse(): VisualDesignResponse {
  return {
    persisted: false,
    revision: 0,
    document: {
      version: 1,
      canvas: { width: 1920, height: 1080, transparent: true },
      layers: [
        {
          id: 'layer_1', name: 'Alert text', kind: 'text', visible: true, locked: false, order: 0,
          frame: { x: 160, y: 940, width: 1600, height: 120 }, opacity: 1,
          text: {
            binding: 'alert_rendered_text', missingValueBehavior: 'hide',
            fontFamily: 'system-ui', fontSize: 44, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
            textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
            outlineWidth: 0, outlineColor: '#000000',
            shadowEnabled: true, shadowOffsetX: 0, shadowOffsetY: 2, shadowBlur: 8, shadowColor: '#000000CC',
          },
          entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
        },
      ],
    },
  };
}

function renderWorkspace(initialResponse: VisualDesignResponse = draftResponse()) {
  return renderWithProviders(
    <MemoryRouter>
      <AlertDesignerWorkspace rule={rule} profile={profile} eventTypeCapability={undefined} initialResponse={initialResponse} />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.mocked(alertsApi).previewAlertTemplate.mockResolvedValue({
    renderedText: 'Ann followed!', codePointCount: 12, resolvedPlaceholders: [], unresolvedPlaceholders: [], validForProvider: true,
  });
  vi.mocked(alertsApi).testAlertRule.mockResolvedValue({
    alertId: 'alinst_1', ruleId: rule.id, eventType: 'follow', queuedAt: '2026-01-01T00:00:00Z', priority: 50,
    renderedText: 'Ann followed!', synthetic: true, replayed: false, groupCount: 1, interruptible: true,
  });
  vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([]);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('AlertDesignerWorkspace', () => {
  it('loads with the generated draft and shows an unsaved-friendly state (never auto-saved)', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    expect(screen.getByTestId('designer-layers-list')).toBeInTheDocument();
    expect(vi.mocked(visualDesignApi).saveVisualDesign).not.toHaveBeenCalled();
  });

  it('Save is disabled until the draft actually changes', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    expect(screen.getByTestId('designer-save')).toBeDisabled();
  });

  it('adding a layer marks the draft dirty and enables Save', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    expect(screen.getByTestId('designer-save')).not.toBeDisabled();
    expect(screen.getByTestId('designer-dirty-indicator')).toHaveAttribute('data-dirty', 'true');
  });

  it('selecting a layer from the list shows its properties and allows numeric position editing', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-layer-row'));
    const xInput = await screen.findByTestId('designer-layer-x');
    expect(xInput).toHaveValue(160);

    fireEvent.change(xInput, { target: { value: '200' } });
    expect(xInput).toHaveValue(200);
  });

  it('deselecting by clicking empty canvas shows canvas properties instead', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-layer-row'));
    await screen.findByTestId('designer-layer-x');

    fireEvent.click(screen.getByTestId('designer-canvas-workspace'));
    expect(screen.queryByTestId('designer-layer-x')).not.toBeInTheDocument();
    expect(screen.getByTestId('designer-canvas-width')).toBeInTheDocument();
  });

  it('duplicating a layer creates a second layer with a different id', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-layer-duplicate'));
    const rows = screen.getAllByTestId('designer-layer-row');
    expect(rows).toHaveLength(2);
    expect(rows[0]?.dataset.layerId).not.toBe(rows[1]?.dataset.layerId);
  });

  it('deleting a layer removes it from the list', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-layer-delete'));
    await waitFor(() => expect(screen.queryAllByTestId('designer-layer-row')).toHaveLength(0));
  });

  it('hiding a layer removes it from the live canvas preview', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    expect(screen.getAllByTestId('visual-design-layer')).toHaveLength(1);
    fireEvent.click(screen.getByTestId('designer-layer-toggle-visible'));
    await waitFor(() => expect(screen.queryAllByTestId('visual-design-layer')).toHaveLength(0));
  });

  it('undo reverts the last committed change, redo reapplies it', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    expect(screen.getByTestId('designer-undo')).toBeDisabled();

    fireEvent.click(screen.getByTestId('designer-add-shape'));
    await waitFor(() => expect(screen.getByTestId('designer-undo')).not.toBeDisabled());
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);

    fireEvent.click(screen.getByTestId('designer-undo'));
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(1);
    expect(screen.getByTestId('designer-redo')).not.toBeDisabled();

    fireEvent.click(screen.getByTestId('designer-redo'));
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);
  });

  it('zoom selection never changes layer geometry', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-layer-row'));
    const xInput = await screen.findByTestId('designer-layer-x');
    const before = (xInput as HTMLInputElement).value;

    fireEvent.change(screen.getByTestId('designer-zoom-select'), { target: { value: '0.5' } });
    expect((screen.getByTestId('designer-layer-x') as HTMLInputElement).value).toBe(before);
  });

  it('reorder move-to-back changes z-order', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    const rowsBefore = screen.getAllByTestId('designer-layer-row');
    const frontId = rowsBefore[0]?.dataset.layerId;

    fireEvent.click(screen.getAllByTestId('designer-layer-move-back')[0]!);
    const rowsAfter = screen.getAllByTestId('designer-layer-row');
    expect(rowsAfter[rowsAfter.length - 1]?.dataset.layerId).toBe(frontId);
  });

  it('Save calls saveVisualDesign with expectedRevision 0 for a first save', async () => {
    vi.mocked(visualDesignApi).saveVisualDesign.mockResolvedValue({ persisted: true, revision: 1, document: draftResponse().document });
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    fireEvent.click(screen.getByTestId('designer-save'));

    await waitFor(() => expect(vi.mocked(visualDesignApi).saveVisualDesign).toHaveBeenCalledWith('alert-rules', rule.id, expect.anything(), 0));
    await waitFor(() => expect(screen.getByTestId('designer-dirty-indicator')).toHaveAttribute('data-dirty', 'false'));
  });

  it('a 409 revision conflict on Save shows the reload banner and preserves the local draft', async () => {
    const { ApiError } = await import('@/lib/api-client');
    vi.mocked(visualDesignApi).saveVisualDesign.mockRejectedValue(new ApiError('http', 'conflict', { status: 409, code: 'visual_design_revision_conflict' }));
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    fireEvent.click(screen.getByTestId('designer-save'));

    await screen.findByTestId('revision-conflict-banner');
    // The local draft (2 layers) is still shown, never silently discarded.
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);
  });

  it('Reset to legacy requires confirmation before calling deleteVisualDesign', async () => {
    vi.mocked(visualDesignApi).deleteVisualDesign.mockResolvedValue(undefined);
    renderWorkspace({ persisted: true, revision: 2, document: draftResponse().document });
    await screen.findByTestId('alert-designer-workspace');

    fireEvent.click(screen.getByTestId('designer-reset-to-legacy'));
    expect(vi.mocked(visualDesignApi).deleteVisualDesign).not.toHaveBeenCalled();

    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: /reset to legacy/i }));
    await waitFor(() => expect(vi.mocked(visualDesignApi).deleteVisualDesign).toHaveBeenCalledWith('alert-rules', rule.id));
  });

  it('Back with unsaved changes shows a discard confirmation instead of navigating immediately', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));

    fireEvent.click(screen.getByTestId('designer-back'));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getAllByText(/discard/i).length).toBeGreaterThan(0);
  });

  it('Back with no unsaved changes navigates immediately, no confirmation', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-back'));
    expect(screen.queryByText(/discard/i)).not.toBeInTheDocument();
  });

  it('Test Rule uses the real backend testAlertRule endpoint, never the unsaved draft directly', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-test-rule'));
    await waitFor(() => expect(vi.mocked(alertsApi).testAlertRule).toHaveBeenCalledWith(rule.id, undefined));
  });

  it('changing the preview scenario never calls saveVisualDesign or testAlertRule', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.change(screen.getByTestId('designer-scenario-select'), { target: { value: 'bits' } });
    await waitFor(() => expect(vi.mocked(alertsApi).previewAlertTemplate).toHaveBeenCalled());
    expect(vi.mocked(visualDesignApi).saveVisualDesign).not.toHaveBeenCalled();
    expect(vi.mocked(alertsApi).testAlertRule).not.toHaveBeenCalled();
  });

  it('the Templates button opens the shared gallery, scoped to this rule as an alert owner', async () => {
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-open-templates'));
    await screen.findByTestId('template-gallery-list');
    await waitFor(() =>
      expect(vi.mocked(visualTemplateApi).fetchVisualTemplates).toHaveBeenCalledWith(
        { target: 'alert', ownerId: rule.id },
        expect.anything(),
      ),
    );
    expect(vi.mocked(visualDesignApi).saveVisualDesign).not.toHaveBeenCalled();
  });

  it('using a template applies it as a dirty draft without saving the owner design', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([
      {
        id: 'tpl_1', target: 'alert', source: 'user', name: 'My Template', description: '', author: '', license: '',
        templateSchemaVersion: 1,
        document: {
          version: 2, canvas: { width: 1920, height: 1080, transparent: true },
          layers: [
            {
              id: 'layer_from_template', name: 'Text', kind: 'text', visible: true, locked: false, order: 0,
              frame: { x: 0, y: 0, width: 400, height: 100 }, opacity: 1,
              text: {
                binding: 'username', missingValueBehavior: 'hide', fontFamily: 'system-ui', fontSize: 32, fontWeight: 700,
                lineHeight: 1.2, letterSpacing: 0, textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
                outlineWidth: 0, outlineColor: '#000000',
                shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
              },
              entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
            },
          ],
        },
        compatibility: { compatible: true },
      },
    ]);
    renderWorkspace();
    await screen.findByTestId('alert-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-open-templates'));
    await screen.findByTestId('template-gallery-list');
    fireEvent.click(await screen.findByTestId('template-use-as-draft'));
    await waitFor(() => expect(screen.queryByTestId('template-gallery-list')).not.toBeInTheDocument());
    expect(screen.getByTestId('designer-dirty-indicator')).toHaveAttribute('data-dirty', 'true');
    expect(vi.mocked(visualDesignApi).saveVisualDesign).not.toHaveBeenCalled();
  });
});
