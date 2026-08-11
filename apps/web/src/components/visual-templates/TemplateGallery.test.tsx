import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import * as visualTemplateApi from '@/api/visualtemplate';
import type { VisualTemplate, VisualTemplateFile } from '@/api/visualtemplate-schemas';
import { renderWithProviders } from '@/test/render';

import { TemplateGallery } from './TemplateGallery';

vi.mock('@/api/visualtemplate');

function chatDocument() {
  return {
    version: 2,
    canvas: { width: 960, height: 280, transparent: true },
    layers: [
      {
        id: 'layer_1', name: 'Username', kind: 'text' as const, visible: true, locked: false, order: 0,
        frame: { x: 10, y: 10, width: 400, height: 60 }, opacity: 1,
        text: {
          binding: 'username' as const, missingValueBehavior: 'hide' as const,
          fontFamily: 'system-ui' as const, fontSize: 20, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
          textColor: '#FFFFFF', horizontalAlign: 'left' as const, verticalAlign: 'middle' as const,
          outlineWidth: 0, outlineColor: '#000000',
          shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
        },
        entryAnimation: 'none' as const, exitAnimation: 'none' as const, animationDurationMs: 0,
      },
    ],
  };
}

function builtinTemplate(overrides: Partial<VisualTemplate> = {}): VisualTemplate {
  return {
    id: 'builtin_chat_minimal_dark', target: 'chat', source: 'builtin',
    name: 'Minimal Dark', description: 'A dark chat item.', author: 'Streaming Tree', license: 'CC0-1.0',
    templateSchemaVersion: 1, document: chatDocument(),
    compatibility: { compatible: true },
    ...overrides,
  };
}

function userTemplate(overrides: Partial<VisualTemplate> = {}): VisualTemplate {
  return {
    id: 'tpl_1', target: 'chat', source: 'user',
    name: 'My Template', description: '', author: '', license: '',
    templateSchemaVersion: 1, document: chatDocument(),
    compatibility: { compatible: true },
    ...overrides,
  };
}

function renderGallery(props: Partial<Parameters<typeof TemplateGallery>[0]> = {}) {
  const onClose = vi.fn();
  const onUseAsDraft = vi.fn();
  const utils = renderWithProviders(
    <TemplateGallery
      open
      onClose={onClose}
      target="chat"
      ownerId="co_1"
      draftIsDirty={false}
      currentDraftDocument={chatDocument()}
      onUseAsDraft={onUseAsDraft}
      {...props}
    />,
  );
  return { ...utils, onClose, onUseAsDraft };
}

afterEach(() => {
  vi.clearAllMocks();
});

describe('TemplateGallery', () => {
  it('lists built-in and user templates in separate sections', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([builtinTemplate(), userTemplate()]);
    renderGallery();
    await screen.findByText('Minimal Dark');
    expect(screen.getByText('My Template')).toBeInTheDocument();
    const cards = screen.getAllByTestId('template-card');
    expect(cards).toHaveLength(2);
  });

  it('shows an incompatible template with a disabled Use as draft button', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([
      builtinTemplate({ compatibility: { compatible: false, blockers: ['template_target_mismatch'] } }),
    ]);
    renderGallery();
    await screen.findByText('Minimal Dark');
    const card = screen.getByTestId('template-card');
    expect(within(card).getByTestId('template-use-as-draft')).toBeDisabled();
    expect(within(card).getByTestId('template-compatibility')).toHaveAttribute('data-compatible', 'false');
  });

  it('Use as draft calls onUseAsDraft and closes when the draft is clean', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([userTemplate()]);
    const { onUseAsDraft, onClose } = renderGallery({ draftIsDirty: false });
    await screen.findByText('My Template');
    fireEvent.click(screen.getByTestId('template-use-as-draft'));
    expect(onUseAsDraft).toHaveBeenCalledWith(userTemplate().document);
    expect(onClose).toHaveBeenCalled();
  });

  it('Use as draft with a dirty current draft shows a confirmation instead of applying immediately', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([userTemplate()]);
    const { onUseAsDraft, onClose } = renderGallery({ draftIsDirty: true });
    await screen.findByText('My Template');
    fireEvent.click(screen.getByTestId('template-use-as-draft'));
    expect(onUseAsDraft).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog', { name: /replace current draft/i });
    fireEvent.click(within(dialog).getByRole('button', { name: /replace draft/i }));
    expect(onUseAsDraft).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it('Export downloads the file without saving anything', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([userTemplate()]);
    const file: VisualTemplateFile = {
      format: 'streaming-tree-visual-template', schemaVersion: 1, target: 'chat',
      name: 'My Template', description: '', author: '', license: '', visualDesign: chatDocument(),
    };
    vi.mocked(visualTemplateApi).exportVisualTemplate.mockResolvedValue(file);
    renderGallery();
    await screen.findByText('My Template');
    fireEvent.click(screen.getByTestId('template-export'));
    await waitFor(() => expect(vi.mocked(visualTemplateApi).exportVisualTemplate).toHaveBeenCalledWith('tpl_1'));
  });

  it('Delete requires confirmation and never appears for a built-in', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([builtinTemplate(), userTemplate()]);
    vi.mocked(visualTemplateApi).deleteVisualTemplate.mockResolvedValue(undefined);
    renderGallery();
    await screen.findByText('My Template');
    const cards = screen.getAllByTestId('template-card');
    const builtinCard = cards.find((c) => c.getAttribute('data-template-id') === 'builtin_chat_minimal_dark');
    expect(builtinCard === undefined ? undefined : within(builtinCard).queryByTestId('template-delete')).toBeNull();

    fireEvent.click(screen.getByTestId('template-delete'));
    expect(vi.mocked(visualTemplateApi).deleteVisualTemplate).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog', { name: /delete template/i });
    fireEvent.click(within(dialog).getByRole('button', { name: /^delete$/i }));
    await waitFor(() => expect(vi.mocked(visualTemplateApi).deleteVisualTemplate).toHaveBeenCalledWith('tpl_1'));
  });

  it('Save as template persists the current draft, not the owner design', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([]);
    vi.mocked(visualTemplateApi).createVisualTemplate.mockResolvedValue(userTemplate());
    const draft = chatDocument();
    renderGallery({ currentDraftDocument: draft });
    fireEvent.click(await screen.findByTestId('template-save-as-template-button'));
    fireEvent.change(screen.getByTestId('template-save-name'), { target: { value: 'New Template' } });
    fireEvent.click(screen.getByTestId('template-save-confirm'));
    await waitFor(() =>
      expect(vi.mocked(visualTemplateApi).createVisualTemplate).toHaveBeenCalledWith(
        expect.objectContaining({ target: 'chat', name: 'New Template', document: draft }),
      ),
    );
  });

  it('import preview never persists until the operator explicitly confirms', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([]);
    const preview = userTemplate({ name: 'Imported' });
    vi.mocked(visualTemplateApi).previewVisualTemplateImport.mockResolvedValue(preview);
    renderGallery();
    await screen.findByTestId('template-gallery-list');
    const file = new File(
      [
        JSON.stringify({
          format: 'streaming-tree-visual-template', schemaVersion: 1, target: 'chat',
          name: 'Imported', description: '', author: '', license: '', visualDesign: chatDocument(),
        }),
      ],
      'template.json',
      { type: 'application/json' },
    );
    fireEvent.change(screen.getByTestId('template-import-file-input'), { target: { files: [file] } });
    await screen.findByText('Imported');
    expect(vi.mocked(visualTemplateApi).importVisualTemplate).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId('template-import-confirm'));
    await waitFor(() => expect(vi.mocked(visualTemplateApi).importVisualTemplate).toHaveBeenCalled());
  });

  it('opening the gallery never calls any save/delete API on its own', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([builtinTemplate(), userTemplate()]);
    renderGallery();
    await screen.findByText('My Template');
    expect(vi.mocked(visualTemplateApi).createVisualTemplate).not.toHaveBeenCalled();
    expect(vi.mocked(visualTemplateApi).deleteVisualTemplate).not.toHaveBeenCalled();
    expect(vi.mocked(visualTemplateApi).importVisualTemplate).not.toHaveBeenCalled();
  });
});
