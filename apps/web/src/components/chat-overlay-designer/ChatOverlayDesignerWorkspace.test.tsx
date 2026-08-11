import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { ChatOverlayProfile } from '@/api/chat-overlay-schemas';
import type { VisualDesignResponse } from '@/api/visualdesign-schemas';
import * as visualDesignApi from '@/api/visualdesign';
import * as visualTemplateApi from '@/api/visualtemplate';
import { renderWithProviders } from '@/test/render';

import { ChatOverlayDesignerWorkspace } from './ChatOverlayDesignerWorkspace';

vi.mock('@/api/visualdesign');
vi.mock('@/api/visualtemplate');

const overlay: ChatOverlayProfile = {
  id: 'co_1', publicSlug: 'slug', name: 'Main Overlay', enabled: true,
  layoutMode: 'horizontal', stackDirection: 'bottom_up', horizontalAlignment: 'left',
  showPlatformIcon: true, showPlatformName: false, showAccountLabel: false, showAvatar: false,
  showBadges: true, showTimestamp: false, showActivityEvents: true, showDeletedPlaceholder: false,
  hideCommands: true, hideBots: true, maxVisibleItems: 30, messageLifetimeSeconds: 0,
  fontFamily: 'sans_serif', fontSize: 16, fontWeight: 400, lineHeight: 1.4, textColor: '#FFFFFF',
  usernameColorMode: 'provider', bubbleColor: '#000000', bubbleOpacity: 0.45, borderRadius: 8,
  itemSpacing: 6, textOutline: true, textShadow: false, entryAnimation: 'fade', exitAnimation: 'fade',
  animationDurationMs: 250, highlightBroadcaster: true, highlightModerators: true,
  highlightSubscribers: false, highlightVips: false, language: 'en',
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
};

function draftResponse(): VisualDesignResponse {
  return {
    persisted: false,
    revision: 0,
    document: {
      version: 2,
      canvas: { width: 960, height: 280, transparent: true },
      layers: [
        {
          id: 'layer_1', name: 'Username', kind: 'text', visible: true, locked: false, order: 0,
          frame: { x: 10, y: 10, width: 400, height: 60 }, opacity: 1,
          text: {
            binding: 'username', missingValueBehavior: 'hide',
            fontFamily: 'system-ui', fontSize: 20, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
            textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'middle',
            outlineWidth: 0, outlineColor: '#000000',
            shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
          },
          entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
        },
      ],
    },
  };
}

function renderWorkspace(initialResponse: VisualDesignResponse = draftResponse()) {
  return renderWithProviders(
    <MemoryRouter>
      <ChatOverlayDesignerWorkspace overlay={overlay} initialResponse={initialResponse} />
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.clearAllMocks();
});

describe('ChatOverlayDesignerWorkspace', () => {
  it('loads with the generated draft and never auto-saves', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    expect(screen.getByTestId('designer-layers-list')).toBeInTheDocument();
    expect(vi.mocked(visualDesignApi).saveVisualDesign).not.toHaveBeenCalled();
  });

  it('Save is disabled until the draft actually changes', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    expect(screen.getByTestId('designer-save')).toBeDisabled();
  });

  it('adding a message_fragments layer marks the draft dirty', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-message_fragments'));
    expect(screen.getByTestId('designer-save')).not.toBeDisabled();
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);
  });

  it('adding a badge_list layer works too', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-badge_list'));
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);
  });

  it('no "Test Rule"-equivalent action is offered - chat has no real-queue test path', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    expect(screen.queryByTestId('designer-test-rule')).not.toBeInTheDocument();
  });

  it('the text binding selector marks alert_rendered_text unavailable and offers the new chat-only bindings', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-layer-row'));
    const select = await screen.findByTestId('designer-text-binding');
    const options = Array.from(select.querySelectorAll('option'));
    const alertRenderedTextOption = options.find((o) => o.getAttribute('value') === 'alert_rendered_text');
    // Never excluded from the dropdown entirely (the shared SelectInput has
    // no per-option disabled support - see DesignerPropertiesPanel.tsx's
    // own doc comment) but always marked unavailable with the same "⚠"
    // suffix an alert design uses for an event-type-unavailable binding.
    expect(alertRenderedTextOption?.textContent).toContain('⚠');
    const values = options.map((o) => o.getAttribute('value'));
    expect(values).toContain('timestamp');
    expect(values).toContain('account_label');
  });

  it('duplicating and deleting a layer works identically to the Alert Designer', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-layer-duplicate'));
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);
    fireEvent.click(screen.getAllByTestId('designer-layer-delete')[0]!);
    await waitFor(() => expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(1));
  });

  it('undo reverts the last committed change, redo reapplies it', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    await waitFor(() => expect(screen.getByTestId('designer-undo')).not.toBeDisabled());
    fireEvent.click(screen.getByTestId('designer-undo'));
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(1);
    fireEvent.click(screen.getByTestId('designer-redo'));
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);
  });

  it('Save calls saveVisualDesign against the chat-overlays owner path', async () => {
    vi.mocked(visualDesignApi).saveVisualDesign.mockResolvedValue({ persisted: true, revision: 1, document: draftResponse().document });
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    fireEvent.click(screen.getByTestId('designer-save'));
    await waitFor(() => expect(vi.mocked(visualDesignApi).saveVisualDesign).toHaveBeenCalledWith('chat-overlays', overlay.id, expect.anything(), 0));
  });

  it('a 409 revision conflict on Save shows the reload banner and preserves the local draft', async () => {
    const { ApiError } = await import('@/lib/api-client');
    vi.mocked(visualDesignApi).saveVisualDesign.mockRejectedValue(new ApiError('http', 'conflict', { status: 409, code: 'visual_design_revision_conflict' }));
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    fireEvent.click(screen.getByTestId('designer-save'));
    await screen.findByTestId('revision-conflict-banner');
    expect(screen.getAllByTestId('designer-layer-row')).toHaveLength(2);
  });

  it('Reset to legacy requires confirmation before calling deleteVisualDesign', async () => {
    vi.mocked(visualDesignApi).deleteVisualDesign.mockResolvedValue(undefined);
    renderWorkspace({ persisted: true, revision: 2, document: draftResponse().document });
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-reset-to-legacy'));
    expect(vi.mocked(visualDesignApi).deleteVisualDesign).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: /reset to legacy/i }));
    await waitFor(() => expect(vi.mocked(visualDesignApi).deleteVisualDesign).toHaveBeenCalledWith('chat-overlays', overlay.id));
  });

  it('Back with unsaved changes shows a discard confirmation', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-add-shape'));
    fireEvent.click(screen.getByTestId('designer-back'));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getAllByText(/discard/i).length).toBeGreaterThan(0);
  });

  it('changing the preview scenario never calls saveVisualDesign', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.change(screen.getByTestId('designer-scenario-select'), { target: { value: 'bits' } });
    expect(vi.mocked(visualDesignApi).saveVisualDesign).not.toHaveBeenCalled();
  });

  it('the preview canvas reflects the selected scenario data, never a real queue/network call', async () => {
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    expect(await screen.findByText('TestViewer')).toBeInTheDocument();
    fireEvent.change(screen.getByTestId('designer-scenario-select'), { target: { value: 'very_long_username' } });
    expect(screen.queryByText('TestViewer')).not.toBeInTheDocument();
  });

  it('the Templates button opens the shared gallery, scoped to this overlay as a chat owner', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([]);
    renderWorkspace();
    await screen.findByTestId('chat-overlay-designer-workspace');
    fireEvent.click(screen.getByTestId('designer-open-templates'));
    await screen.findByTestId('template-gallery-list');
    await waitFor(() =>
      expect(vi.mocked(visualTemplateApi).fetchVisualTemplates).toHaveBeenCalledWith(
        { target: 'chat', ownerId: overlay.id },
        expect.anything(),
      ),
    );
    expect(vi.mocked(visualDesignApi).saveVisualDesign).not.toHaveBeenCalled();
  });
});
