import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import * as visualPackageApi from '@/api/visualpackage';
import type { VisualTemplatePackagePreview } from '@/api/visualpackage-schemas';
import * as visualTemplateApi from '@/api/visualtemplate';
import type { VisualTemplate, VisualTemplateFile } from '@/api/visualtemplate-schemas';
import { renderWithProviders } from '@/test/render';

import { TemplateGallery } from './TemplateGallery';

vi.mock('@/api/visualtemplate');
vi.mock('@/api/visualpackage');

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
    expect(onUseAsDraft).toHaveBeenCalledWith(userTemplate().document, undefined);
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

  // --- Stage 14B: package import/export -----------------------------------

  it('Export package downloads a blob without saving anything', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([userTemplate()]);
    vi.mocked(visualPackageApi).exportVisualTemplatePackage.mockResolvedValue({
      blob: new Blob(['fake zip bytes']),
      filename: 'my-template.streaming-tree-template',
    });
    renderGallery();
    await screen.findByText('My Template');
    fireEvent.click(screen.getByTestId('template-export-package'));
    await waitFor(() => expect(vi.mocked(visualPackageApi).exportVisualTemplatePackage).toHaveBeenCalledWith('tpl_1'));
  });

  it('JSON Export is disabled with an explanation for an asset-backed template', async () => {
    const assetBackedDoc = {
      ...chatDocument(),
      layers: [
        {
          id: 'layer_img', name: 'Badge', kind: 'image' as const, visible: true, locked: false, order: 0,
          frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
          image: { assetId: 'asset_1', fit: 'contain' as const, alt: '' },
          entryAnimation: 'none' as const, exitAnimation: 'none' as const, animationDurationMs: 0,
        },
      ],
    };
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([userTemplate({ document: assetBackedDoc })]);
    renderGallery();
    await screen.findByText('My Template');
    const exportButton = screen.getByTestId('template-export');
    expect(exportButton).toBeDisabled();
    fireEvent.click(exportButton);
    expect(vi.mocked(visualTemplateApi).exportVisualTemplate).not.toHaveBeenCalled();
    // Export package remains available for the same template.
    expect(screen.getByTestId('template-export-package')).not.toBeDisabled();
  });

  it('shows an audio badge and disables JSON Export for a template with an alert-audio preset (Stage 17B)', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([
      userTemplate({
        target: 'alert',
        alertAudio: { soundEnabled: true, soundAssetId: 'audioasset_1', soundVolume: 1, ttsEnabled: false, ttsVolume: 1 },
      }),
    ]);
    renderGallery({ target: 'alert' });
    await screen.findByText('My Template');
    expect(screen.getByTestId('template-has-audio')).toBeInTheDocument();
    const exportButton = screen.getByTestId('template-export');
    expect(exportButton).toBeDisabled();
    fireEvent.click(exportButton);
    expect(vi.mocked(visualTemplateApi).exportVisualTemplate).not.toHaveBeenCalled();
    expect(screen.getByTestId('template-export-package')).not.toBeDisabled();
  });

  it('never shows the audio badge for a template with no alert-audio preset', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([userTemplate()]);
    renderGallery();
    await screen.findByText('My Template');
    expect(screen.queryByTestId('template-has-audio')).not.toBeInTheDocument();
  });

  it('Use as draft passes the template own alert-audio preset through to onUseAsDraft (Stage 17B)', async () => {
    const audio = { soundEnabled: true, soundAssetId: 'audioasset_1', soundVolume: 0.8, ttsEnabled: true, ttsTemplate: '{username}', ttsVolume: 0.5 };
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([userTemplate({ target: 'alert', alertAudio: audio })]);
    const { onUseAsDraft } = renderGallery({ target: 'alert' });
    await screen.findByText('My Template');
    fireEvent.click(screen.getByTestId('template-use-as-draft'));
    expect(onUseAsDraft).toHaveBeenCalledWith(userTemplate().document, audio);
  });

  it('package import preview never persists until the operator explicitly confirms, and re-uploads the original bytes', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([]);
    const preview: VisualTemplatePackagePreview = {
      token: 'preview_tok_1',
      target: 'chat',
      name: 'Imported Package',
      description: '',
      author: '',
      license: '',
      document: chatDocument(),
      assets: [],
      expiresAt: '2026-08-12T00:10:00.000Z',
    };
    vi.mocked(visualPackageApi).previewVisualTemplatePackageImport.mockResolvedValue(preview);
    vi.mocked(visualPackageApi).importVisualTemplatePackage.mockResolvedValue(userTemplate({ name: 'Imported Package' }));
    renderGallery();
    await screen.findByTestId('template-gallery-list');

    const file = new File(['fake zip bytes'], 'template.streaming-tree-template');
    fireEvent.change(screen.getByTestId('template-import-package-file-input'), { target: { files: [file] } });
    await screen.findByText('Imported Package');
    expect(vi.mocked(visualPackageApi).importVisualTemplatePackage).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('template-import-package-confirm'));
    await waitFor(() => expect(vi.mocked(visualPackageApi).importVisualTemplatePackage).toHaveBeenCalledWith(file));
  });

  it('package import preview shows the audio in the package before it is imported (Stage 17B)', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([]);
    const preview: VisualTemplatePackagePreview = {
      token: 'preview_tok_audio',
      target: 'alert',
      name: 'Coin Alert Package',
      description: '',
      author: '',
      license: '',
      document: chatDocument(),
      assets: [],
      alertAudio: { soundEnabled: true, soundDisplayName: 'Coin chime', soundDurationMs: 2500, ttsEnabled: true, ttsTemplate: '{username} triggered a coin' },
      expiresAt: '2026-08-12T00:10:00.000Z',
    };
    vi.mocked(visualPackageApi).previewVisualTemplatePackageImport.mockResolvedValue(preview);
    renderGallery({ target: 'alert' });
    await screen.findByTestId('template-gallery-list');

    const file = new File(['fake zip bytes'], 'template.streaming-tree-template');
    fireEvent.change(screen.getByTestId('template-import-package-file-input'), { target: { files: [file] } });
    const audioSection = await screen.findByTestId('template-package-audio');
    expect(within(audioSection).getByText(/coin chime/i)).toBeInTheDocument();
  });

  it('canceling a package import preview releases the staged preview session', async () => {
    vi.mocked(visualTemplateApi).fetchVisualTemplates.mockResolvedValue([]);
    const preview: VisualTemplatePackagePreview = {
      token: 'preview_tok_2',
      target: 'chat',
      name: 'Imported Package',
      description: '',
      author: '',
      license: '',
      document: chatDocument(),
      assets: [],
      expiresAt: '2026-08-12T00:10:00.000Z',
    };
    vi.mocked(visualPackageApi).previewVisualTemplatePackageImport.mockResolvedValue(preview);
    vi.mocked(visualPackageApi).cancelVisualTemplatePackagePreview.mockResolvedValue(undefined);
    renderGallery();
    await screen.findByTestId('template-gallery-list');

    const file = new File(['fake zip bytes'], 'template.streaming-tree-template');
    fireEvent.change(screen.getByTestId('template-import-package-file-input'), { target: { files: [file] } });
    await screen.findByText('Imported Package');
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    await waitFor(() => expect(vi.mocked(visualPackageApi).cancelVisualTemplatePackagePreview).toHaveBeenCalledWith('preview_tok_2'));
    expect(vi.mocked(visualPackageApi).importVisualTemplatePackage).not.toHaveBeenCalled();
  });
});
