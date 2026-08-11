import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { VisualDesignDocument } from '@/api/visualdesign-schemas';
import {
  VISUAL_TEMPLATE_IMPORT_MAX_BYTES,
  visualTemplateFileSchema,
  type VisualTemplate,
  type VisualTemplateTarget,
} from '@/api/visualtemplate-schemas';
import type { VisualTemplatePackagePreview } from '@/api/visualpackage-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Modal } from '@/components/ui/Modal';
import { TextArea, TextInput } from '@/components/ui/TextInput';
import type { RenderableLayer, VisualAssetMap } from '@/components/visual-design/VisualLayer';
import { VisualDesignRenderer } from '@/components/visual-design/VisualDesignRenderer';
import {
  useCancelVisualTemplatePackagePreviewMutation,
  useCreateVisualTemplateMutation,
  useDeleteVisualTemplateMutation,
  useExportVisualTemplateMutation,
  useExportVisualTemplatePackageMutation,
  useImportVisualTemplateMutation,
  useImportVisualTemplatePackageMutation,
  useImportVisualTemplatePackagePreviewMutation,
  useImportVisualTemplatePreviewMutation,
  useVisualTemplatesQuery,
} from '@/hooks/use-visual-templates';
import { downloadBlob, downloadVisualTemplateFile, templateHasAssets } from '@/models/visualtemplate';

import { templatePreviewDataContext } from './template-preview-context';

type GalleryMode =
  | { kind: 'list' }
  | { kind: 'preview'; template: VisualTemplate }
  | { kind: 'import' }
  | { kind: 'importPackage' }
  | { kind: 'saveAsTemplate' };

/** `Blob.prototype.text()` is not implemented in every test/older
 * browser environment this project's own jsdom test setup runs under -
 * `FileReader` is the more broadly compatible way to read a selected
 * file's own text content. */
function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
    reader.onerror = () => reject(reader.error ?? new Error('failed to read file'));
    reader.readAsText(file);
  });
}

/**
 * The one shared template gallery both the Alert Overlay Designer and
 * the Chat Overlay Designer use (Stage 14A task Part 32) - never a
 * separate AlertTemplateGallery/ChatTemplateGallery. Opening it, or
 * previewing/importing a template inside it, never saves the owner's
 * design (Part 14): only `onUseAsDraft` (routed through the caller's
 * own existing `commitDraft`, one undo step) or "Save as template"
 * (an independent, owner-untouching write) ever change anything.
 */
export function TemplateGallery({
  open,
  onClose,
  target,
  ownerId,
  draftIsDirty,
  currentDraftDocument,
  onUseAsDraft,
}: {
  open: boolean;
  onClose: () => void;
  target: VisualTemplateTarget;
  ownerId: string;
  draftIsDirty: boolean;
  currentDraftDocument: VisualDesignDocument;
  onUseAsDraft: (document: VisualDesignDocument) => void;
}) {
  const { t } = useTranslation('visualTemplates');
  const { t: tOverlays } = useTranslation('overlays');

  const [mode, setMode] = useState<GalleryMode>({ kind: 'list' });
  const [pendingDraft, setPendingDraft] = useState<VisualDesignDocument | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VisualTemplate | null>(null);
  const [importFile, setImportFile] = useState<{ raw: unknown; preview: VisualTemplate } | null>(null);
  const [importError, setImportError] = useState<string | null>(null);
  const [saveMeta, setSaveMeta] = useState({ name: '', description: '', author: '', license: '' });
  const [packageFile, setPackageFile] = useState<{ file: File; preview: VisualTemplatePackagePreview } | null>(null);
  const [packageError, setPackageError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const packageFileInputRef = useRef<HTMLInputElement>(null);

  const listQuery = useVisualTemplatesQuery({ target, ownerId }, { enabled: open });
  const createMutation = useCreateVisualTemplateMutation();
  const deleteMutation = useDeleteVisualTemplateMutation();
  const importPreviewMutation = useImportVisualTemplatePreviewMutation();
  const importMutation = useImportVisualTemplateMutation();
  const exportMutation = useExportVisualTemplateMutation();
  const importPackagePreviewMutation = useImportVisualTemplatePackagePreviewMutation();
  const cancelPackagePreviewMutation = useCancelVisualTemplatePackagePreviewMutation();
  const importPackageMutation = useImportVisualTemplatePackageMutation();
  const exportPackageMutation = useExportVisualTemplatePackageMutation();

  const builtins = (listQuery.data ?? []).filter((tpl) => tpl.source === 'builtin');
  const userTemplates = (listQuery.data ?? []).filter((tpl) => tpl.source === 'user');

  function requestUseAsDraft(document: VisualDesignDocument) {
    if (draftIsDirty) {
      setPendingDraft(document);
      return;
    }
    onUseAsDraft(document);
    onClose();
  }

  function confirmReplaceDraft() {
    if (pendingDraft !== null) {
      onUseAsDraft(pendingDraft);
      setPendingDraft(null);
      onClose();
    }
  }

  function handleExport(tpl: VisualTemplate) {
    exportMutation.mutate(tpl.id, {
      onSuccess: (file) => downloadVisualTemplateFile(file, tpl.name),
    });
  }

  function handleExportPackage(tpl: VisualTemplate) {
    exportPackageMutation.mutate(tpl.id, {
      onSuccess: ({ blob, filename }) => downloadBlob(blob, filename),
    });
  }

  function handlePackageFileSelected(fileList: FileList | null) {
    const file = fileList?.[0];
    if (file === undefined) return;
    setPackageError(null);
    importPackagePreviewMutation.mutate(file, {
      onSuccess: (preview) => setPackageFile({ file, preview }),
      onError: (error) => setPackageError(error.message),
    });
  }

  function closePackageImport() {
    if (packageFile !== null) {
      cancelPackagePreviewMutation.mutate(packageFile.preview.token);
    }
    setPackageFile(null);
    setPackageError(null);
    setMode({ kind: 'list' });
    if (packageFileInputRef.current) packageFileInputRef.current.value = '';
  }

  function confirmImportPackage() {
    if (packageFile === null) return;
    // Re-uploads and fully re-validates the original file's own bytes -
    // never trusts the preview token as proof (docs/visual-template-
    // packages.md §19 step 6).
    importPackageMutation.mutate(packageFile.file, {
      onSuccess: () => {
        setPackageFile(null);
        setMode({ kind: 'list' });
        if (packageFileInputRef.current) packageFileInputRef.current.value = '';
      },
      onError: (error) => setPackageError(error.message),
    });
  }

  function packagePreviewAssetMap(preview: VisualTemplatePackagePreview): VisualAssetMap {
    const map: VisualAssetMap = {};
    for (const asset of preview.assets) {
      map[asset.packageAssetId] = { url: asset.url, mediaType: asset.mediaType };
    }
    return map;
  }

  function handleFileSelected(fileList: FileList | null) {
    const file = fileList?.[0];
    if (file === undefined) return;
    setImportError(null);
    if (file.size > VISUAL_TEMPLATE_IMPORT_MAX_BYTES) {
      setImportError(t('import.tooLarge'));
      return;
    }
    void readFileAsText(file)
      .then((text) => JSON.parse(text) as unknown)
      .then((raw) => {
        const parsed = visualTemplateFileSchema.safeParse(raw);
        if (!parsed.success) {
          setImportError(t('import.malformed'));
          return;
        }
        importPreviewMutation.mutate(
          { file: parsed.data, ownerContext: { target, ownerId } },
          {
            onSuccess: (preview) => setImportFile({ raw: parsed.data, preview }),
            onError: () => setImportError(t('import.invalid')),
          },
        );
      })
      .catch(() => setImportError(t('import.malformed')));
  }

  function confirmImport() {
    if (importFile === null) return;
    const parsed = visualTemplateFileSchema.safeParse(importFile.raw);
    if (!parsed.success) {
      setImportError(t('import.malformed'));
      return;
    }
    importMutation.mutate(parsed.data, {
      onSuccess: () => {
        setImportFile(null);
        setMode({ kind: 'list' });
        if (fileInputRef.current) fileInputRef.current.value = '';
      },
    });
  }

  function submitSaveAsTemplate() {
    createMutation.mutate(
      { target, ...saveMeta, document: currentDraftDocument },
      {
        onSuccess: () => {
          setSaveMeta({ name: '', description: '', author: '', license: '' });
          setMode({ kind: 'list' });
        },
      },
    );
  }

  function renderCard(tpl: VisualTemplate) {
    const compatible = tpl.compatibility?.compatible ?? true;
    return (
      <div key={tpl.id} className="flex flex-col gap-2 rounded border border-line p-3" data-testid="template-card" data-template-id={tpl.id}>
        <div className="h-24 w-full overflow-hidden rounded bg-canvas-muted" data-testid="template-card-preview">
          <VisualDesignRenderer
            canvas={tpl.document.canvas}
            layers={tpl.document.layers as RenderableLayer[]}
            dataContext={templatePreviewDataContext(tpl.target, tOverlays)}
            mode="preview"
            prefersReducedMotion
          />
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm font-medium text-ink">{tpl.name}</span>
          <span
            className="rounded bg-canvas-muted px-1.5 py-0.5 text-[10px] uppercase text-ink-muted"
            data-testid="template-source"
          >
            {tpl.source === 'builtin' ? t('source.builtin') : t('source.user')}
          </span>
        </div>
        {tpl.description !== '' ? <p className="text-xs text-ink-muted">{tpl.description}</p> : null}
        <div
          className="text-xs"
          data-testid="template-compatibility"
          data-compatible={compatible}
        >
          {compatible ? (
            <span className="text-success">{t('compatibility.compatible')}</span>
          ) : (
            <span className="text-danger">
              {t('compatibility.incompatible')}
              {tpl.compatibility?.blockers !== undefined && tpl.compatibility.blockers.length > 0
                ? ` (${tpl.compatibility.blockers.map((b) => t(`compatibility.blocker.${b}`)).join(', ')})`
                : ''}
            </span>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={() => setMode({ kind: 'preview', template: tpl })} data-testid="template-preview-button">
            {t('actions.preview')}
          </Button>
          <Button
            type="button"
            variant="primary"
            disabled={!compatible}
            onClick={() => requestUseAsDraft(tpl.document)}
            data-testid="template-use-as-draft"
            title={compatible ? undefined : t('compatibility.disabledReason')}
          >
            {t('actions.useAsDraft')}
          </Button>
          <Button
            type="button"
            onClick={() => handleExport(tpl)}
            disabled={templateHasAssets(tpl.document)}
            title={templateHasAssets(tpl.document) ? t('actions.exportRequiresPackage') : undefined}
            data-testid="template-export"
          >
            {t('actions.export')}
          </Button>
          <Button type="button" onClick={() => handleExportPackage(tpl)} data-testid="template-export-package">
            {t('actions.exportPackage')}
          </Button>
          {tpl.source === 'user' ? (
            <Button type="button" variant="danger" onClick={() => setDeleteTarget(tpl)} data-testid="template-delete">
              {t('actions.delete')}
            </Button>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <>
      <Modal open={open} onClose={onClose} title={t('title')} size="md">
        {mode.kind === 'list' ? (
          <div className="flex flex-col gap-4" data-testid="template-gallery-list">
            <div className="flex flex-wrap gap-2">
              <Button type="button" onClick={() => fileInputRef.current?.click()} data-testid="template-import-button">
                {t('actions.import')}
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept=".json,.streaming-tree-template.json"
                className="hidden"
                aria-label={t('actions.import')}
                data-testid="template-import-file-input"
                onChange={(e) => {
                  handleFileSelected(e.target.files);
                  setMode({ kind: 'import' });
                }}
              />
              <Button
                type="button"
                onClick={() => {
                  setSaveMeta({ name: '', description: '', author: '', license: '' });
                  setMode({ kind: 'saveAsTemplate' });
                }}
                data-testid="template-save-as-template-button"
              >
                {t('actions.saveAsTemplate')}
              </Button>
              <Button type="button" onClick={() => packageFileInputRef.current?.click()} data-testid="template-import-package-button">
                {t('actions.importPackage')}
              </Button>
              <input
                ref={packageFileInputRef}
                type="file"
                accept=".streaming-tree-template"
                className="hidden"
                aria-label={t('actions.importPackage')}
                data-testid="template-import-package-file-input"
                onChange={(e) => {
                  handlePackageFileSelected(e.target.files);
                  setMode({ kind: 'importPackage' });
                }}
              />
            </div>

            <section>
              <h3 className="mb-2 text-sm font-semibold text-ink">{t('section.builtin')}</h3>
              {listQuery.isLoading ? (
                <p className="text-sm text-ink-muted">{t('loading')}</p>
              ) : builtins.length === 0 ? (
                <p className="text-sm text-ink-muted">{t('empty.builtin')}</p>
              ) : (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">{builtins.map(renderCard)}</div>
              )}
            </section>

            <section>
              <h3 className="mb-2 text-sm font-semibold text-ink">{t('section.user')}</h3>
              {userTemplates.length === 0 ? (
                <p className="text-sm text-ink-muted">{t('empty.user')}</p>
              ) : (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">{userTemplates.map(renderCard)}</div>
              )}
            </section>
          </div>
        ) : null}

        {mode.kind === 'preview' ? (
          <div className="flex flex-col gap-3" data-testid="template-preview-detail">
            <div className="h-64 w-full overflow-hidden rounded bg-canvas-muted">
              <VisualDesignRenderer
                canvas={mode.template.document.canvas}
                layers={mode.template.document.layers as RenderableLayer[]}
                dataContext={templatePreviewDataContext(mode.template.target, tOverlays)}
                mode="preview"
                prefersReducedMotion
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="button" onClick={() => setMode({ kind: 'list' })}>
                {t('actions.back')}
              </Button>
              <Button
                type="button"
                variant="primary"
                disabled={mode.template.compatibility?.compatible === false}
                onClick={() => requestUseAsDraft(mode.template.document)}
                data-testid="template-preview-use-as-draft"
              >
                {t('actions.useAsDraft')}
              </Button>
            </div>
          </div>
        ) : null}

        {mode.kind === 'import' ? (
          <div className="flex flex-col gap-3" data-testid="template-import-preview">
            {importError !== null ? <p className="text-sm text-danger">{importError}</p> : null}
            {importFile !== null ? (
              <>
                <dl className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-1 text-sm">
                  <dt className="text-ink-muted">{t('import.field.name')}</dt>
                  <dd>{importFile.preview.name}</dd>
                  <dt className="text-ink-muted">{t('import.field.target')}</dt>
                  <dd>{importFile.preview.target}</dd>
                  <dt className="text-ink-muted">{t('import.field.description')}</dt>
                  <dd>{importFile.preview.description}</dd>
                  <dt className="text-ink-muted">{t('import.field.author')}</dt>
                  <dd>{importFile.preview.author}</dd>
                  <dt className="text-ink-muted">{t('import.field.license')}</dt>
                  <dd>{importFile.preview.license}</dd>
                </dl>
                <div className="h-40 w-full overflow-hidden rounded bg-canvas-muted">
                  <VisualDesignRenderer
                    canvas={importFile.preview.document.canvas}
                    layers={importFile.preview.document.layers as RenderableLayer[]}
                    dataContext={templatePreviewDataContext(importFile.preview.target, tOverlays)}
                    mode="preview"
                    prefersReducedMotion
                  />
                </div>
              </>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                onClick={() => {
                  setMode({ kind: 'list' });
                  setImportFile(null);
                  setImportError(null);
                  if (fileInputRef.current) fileInputRef.current.value = '';
                }}
              >
                {t('actions.cancel')}
              </Button>
              <Button
                type="button"
                variant="primary"
                disabled={importFile === null || importMutation.isPending}
                onClick={confirmImport}
                data-testid="template-import-confirm"
              >
                {t('actions.import')}
              </Button>
            </div>
          </div>
        ) : null}

        {mode.kind === 'importPackage' ? (
          <div className="flex flex-col gap-3" data-testid="template-import-package-preview">
            {packageError !== null ? <p className="text-sm text-danger">{packageError}</p> : null}
            {packageFile !== null ? (
              <>
                <dl className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-1 text-sm">
                  <dt className="text-ink-muted">{t('import.field.name')}</dt>
                  <dd>{packageFile.preview.name}</dd>
                  <dt className="text-ink-muted">{t('import.field.target')}</dt>
                  <dd>{packageFile.preview.target}</dd>
                  <dt className="text-ink-muted">{t('import.field.description')}</dt>
                  <dd>{packageFile.preview.description}</dd>
                  <dt className="text-ink-muted">{t('import.field.author')}</dt>
                  <dd>{packageFile.preview.author}</dd>
                  <dt className="text-ink-muted">{t('import.field.license')}</dt>
                  <dd>{packageFile.preview.license}</dd>
                </dl>
                <div className="h-40 w-full overflow-hidden rounded bg-canvas-muted">
                  <VisualDesignRenderer
                    canvas={packageFile.preview.document.canvas}
                    layers={packageFile.preview.document.layers as RenderableLayer[]}
                    dataContext={templatePreviewDataContext(packageFile.preview.target, tOverlays)}
                    mode="preview"
                    prefersReducedMotion
                    assetMap={packagePreviewAssetMap(packageFile.preview)}
                  />
                </div>
                {packageFile.preview.assets.length > 0 ? (
                  <div data-testid="template-package-assets">
                    <p className="mb-1 text-xs font-semibold uppercase text-ink-muted">{t('packageImport.assetsTitle')}</p>
                    <ul className="space-y-1 text-xs text-ink-muted">
                      {packageFile.preview.assets.map((asset) => (
                        <li key={asset.packageAssetId} data-testid="template-package-asset">
                          {asset.displayName || asset.packageAssetId} · {asset.kind}
                          {asset.author !== '' ? ` · ${asset.author}` : ''}
                          {asset.license !== '' ? ` · ${asset.license}` : ''}
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : null}
              </>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button type="button" onClick={closePackageImport}>
                {t('actions.cancel')}
              </Button>
              <Button
                type="button"
                variant="primary"
                disabled={packageFile === null || importPackageMutation.isPending}
                onClick={confirmImportPackage}
                data-testid="template-import-package-confirm"
              >
                {t('actions.import')}
              </Button>
            </div>
          </div>
        ) : null}

        {mode.kind === 'saveAsTemplate' ? (
          <form
            className="flex flex-col gap-3"
            data-testid="template-save-as-template-form"
            onSubmit={(e) => {
              e.preventDefault();
              submitSaveAsTemplate();
            }}
          >
            <label className="flex flex-col gap-1 text-sm">
              {t('metadata.name')}
              <TextInput
                required
                value={saveMeta.name}
                onChange={(e) => setSaveMeta((m) => ({ ...m, name: e.target.value }))}
                data-testid="template-save-name"
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {t('metadata.description')}
              <TextArea
                value={saveMeta.description}
                onChange={(e) => setSaveMeta((m) => ({ ...m, description: e.target.value }))}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {t('metadata.author')}
              <TextInput value={saveMeta.author} onChange={(e) => setSaveMeta((m) => ({ ...m, author: e.target.value }))} />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {t('metadata.license')}
              <TextInput value={saveMeta.license} onChange={(e) => setSaveMeta((m) => ({ ...m, license: e.target.value }))} />
            </label>
            <div className="flex flex-wrap gap-2">
              <Button type="button" onClick={() => setMode({ kind: 'list' })}>
                {t('actions.cancel')}
              </Button>
              <Button type="submit" variant="primary" disabled={createMutation.isPending} data-testid="template-save-confirm">
                {t('actions.save')}
              </Button>
            </div>
          </form>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={pendingDraft !== null}
        title={t('replaceDraft.title')}
        message={t('replaceDraft.message')}
        confirmLabel={t('replaceDraft.confirm')}
        onConfirm={confirmReplaceDraft}
        onCancel={() => setPendingDraft(null)}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('deleteConfirm.title')}
        message={t('deleteConfirm.message', { name: deleteTarget?.name ?? '' })}
        confirmLabel={t('deleteConfirm.confirm')}
        destructive
        busy={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteTarget === null) return;
          deleteMutation.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) });
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </>
  );
}
