import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { RestoreBackupPreview, RestoreBackupResult } from '@/api/backup-schemas';
import { ApiError } from '@/lib/api-client';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import {
  useCancelRestoreBackupPreviewMutation,
  useCommitRestoreBackupMutation,
  useExportBackupMutation,
  usePreviewRestoreBackupMutation,
} from '@/hooks/use-backup';
import { useShutdownMutation } from '@/hooks/use-shutdown';
import { useLanguage } from '@/i18n/use-language';
import { formatBytes } from '@/lib/format';
import { downloadBlob } from '@/models/visualtemplate';

const INCLUDES_KEYS = [
  'destinations',
  'accounts',
  'chat',
  'alerts',
  'visual',
  'audio',
  'goals',
  'metadataPresets',
  'streamSetups',
  'donationSources',
] as const;

const EXCLUDES_KEYS = ['streamKeys', 'oauth', 'donationCredentials', 'remoteManagement', 'engagementContent'] as const;

type CountKey = keyof RestoreBackupPreview['counts'];

// Only the counts this panel actually shows have a matching
// `preview.counts.*` translation key - narrowed via `satisfies` rather
// than `: CountKey[]` so `t()`'s own key-literal typing stays precise
// to these eight, not the full `ObjectCounts` union.
const COUNT_ROWS = [
  'platforms',
  'connectedAccounts',
  'chatOverlays',
  'alertProfiles',
  'visualTemplates',
  'goals',
  'metadataPresets',
  'streamSetupProfiles',
  'donationSources',
] as const satisfies readonly CountKey[];

/**
 * Stage 23E: the Settings-area Backup & Restore panel (docs/backup-
 * restore.md). Mirrors the visual-template *package* import/export
 * flow's own preview-then-confirm shape (`TemplateGallery.tsx`) - a
 * hidden file input, a validated preview staged server-side under a
 * token, an explicit confirmation for the destructive commit step, and
 * re-uploading nothing but that same token to commit (never the file a
 * second time, unlike a template package - the backend already holds
 * the staged bytes under the preview's own token).
 */
export function BackupRestorePanel() {
  const { t } = useTranslation('backup');
  const { locale } = useLanguage();

  const exportMutation = useExportBackupMutation();
  const previewMutation = usePreviewRestoreBackupMutation();
  const cancelPreviewMutation = useCancelRestoreBackupPreviewMutation();
  const commitMutation = useCommitRestoreBackupMutation();
  const quitMutation = useShutdownMutation();

  const fileInputRef = useRef<HTMLInputElement>(null);
  const [preview, setPreview] = useState<RestoreBackupPreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [restoreResult, setRestoreResult] = useState<RestoreBackupResult | null>(null);
  const [quitConfirming, setQuitConfirming] = useState(false);

  function handleFileSelected(fileList: FileList | null) {
    const file = fileList?.[0];
    if (file === undefined) return;
    setPreviewError(null);
    previewMutation.mutate(file, {
      onSuccess: (result) => setPreview(result),
      onError: (error) => setPreviewError(error.message),
    });
  }

  function closePreview() {
    if (preview !== null) {
      cancelPreviewMutation.mutate(preview.token);
    }
    setPreview(null);
    setPreviewError(null);
    if (fileInputRef.current) fileInputRef.current.value = '';
  }

  function confirmRestore() {
    if (preview === null) return;
    commitMutation.mutate(preview.token, {
      onSuccess: (result) => {
        setPreview(null);
        setRestoreResult(result);
        if (fileInputRef.current) fileInputRef.current.value = '';
      },
    });
    setConfirming(false);
  }

  const commitErrorMessage =
    commitMutation.error instanceof ApiError && commitMutation.error.code === 'restore_blocked_streaming_active'
      ? t('streamingActive.message')
      : commitMutation.isError
        ? t('confirm.error')
        : null;

  // Once a restore has committed, RestoreResult.restartRequired is
  // always true (docs/backup-restore.md §7 step 8) - several runtime
  // managers only reload their working state at process start, so the
  // panel replaces itself with a blocking restart notice rather than
  // returning to normal use.
  if (restoreResult !== null) {
    if (quitMutation.isSuccess) {
      return (
        <Panel>
          <PanelHeader title={t('panel.heading')} />
          <PanelBody>
            <p className="text-sm text-ink">{t('restartNotice.stopped')}</p>
          </PanelBody>
        </Panel>
      );
    }

    return (
      <Panel>
        <PanelHeader title={t('panel.heading')} />
        <PanelBody className="space-y-2.5">
          <p className="text-sm font-medium text-ink">{t('restartNotice.title')}</p>
          <p className="text-sm text-ink-muted">{t('restartNotice.body')}</p>
          <Button type="button" variant="danger" onClick={() => setQuitConfirming(true)}>
            {t('restartNotice.quitButton')}
          </Button>
        </PanelBody>
        <ConfirmDialog
          open={quitConfirming}
          title={t('restartNotice.quitConfirmTitle')}
          message={t('restartNotice.quitConfirmMessage')}
          confirmLabel={t('restartNotice.quitButton')}
          destructive
          busy={quitMutation.isPending}
          onCancel={() => setQuitConfirming(false)}
          onConfirm={() => {
            quitMutation.mutate();
            setQuitConfirming(false);
          }}
        />
      </Panel>
    );
  }

  return (
    <Panel>
      <PanelHeader title={t('panel.heading')} description={t('panel.description')} />
      <PanelBody className="space-y-5">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <p className="mb-1 text-xs font-semibold uppercase text-ink-muted">{t('includes.title')}</p>
            <ul className="space-y-0.5 text-xs text-ink-muted">
              {INCLUDES_KEYS.map((key) => (
                <li key={key}>{t(`includes.${key}`)}</li>
              ))}
            </ul>
          </div>
          <div>
            <p className="mb-1 text-xs font-semibold uppercase text-ink-muted">{t('excludes.title')}</p>
            <ul className="space-y-0.5 text-xs text-ink-muted">
              {EXCLUDES_KEYS.map((key) => (
                <li key={key}>{t(`excludes.${key}`)}</li>
              ))}
            </ul>
          </div>
        </div>

        <div className="space-y-2 border-t border-line pt-4">
          <p className="text-sm font-medium text-ink">{t('export.heading')}</p>
          <p className="text-xs text-ink-muted">{t('export.body')}</p>
          <Button
            type="button"
            onClick={() =>
              exportMutation.mutate(undefined, {
                onSuccess: ({ blob, filename }) => downloadBlob(blob, filename),
              })
            }
            disabled={exportMutation.isPending}
            data-testid="backup-export-button"
          >
            {t('export.button')}
          </Button>
          {exportMutation.isError && <p className="text-xs text-status-error">{t('export.error')}</p>}
        </div>

        <div className="space-y-2 border-t border-line pt-4">
          <p className="text-sm font-medium text-ink">{t('restore.heading')}</p>
          <p className="text-xs text-ink-muted">{t('restore.body')}</p>

          {preview === null ? (
            <>
              <Button type="button" onClick={() => fileInputRef.current?.click()} data-testid="backup-restore-choose-button">
                {t('restore.button')}
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept=".streaming-tree-backup"
                className="hidden"
                aria-label={t('restore.chooseFileLabel')}
                data-testid="backup-restore-file-input"
                onChange={(e) => handleFileSelected(e.target.files)}
              />
              {previewError !== null && <p className="text-xs text-status-error">{previewError}</p>}
            </>
          ) : (
            <div className="flex flex-col gap-3" data-testid="backup-restore-preview">
              <p className="text-sm font-medium text-ink">{t('preview.title')}</p>
              <dl className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-1 text-sm">
                <dt className="text-ink-muted">{t('preview.sourceLabel')}</dt>
                <dd>{t('preview.sourceValue', { platform: preview.manifest.sourcePlatform, version: preview.manifest.sourceAppVersion })}</dd>
                <dt className="text-ink-muted">{t('preview.createdAtLabel')}</dt>
                <dd>{new Date(preview.manifest.createdAt).toLocaleString()}</dd>
              </dl>

              <div>
                <p className="mb-1 text-xs font-semibold uppercase text-ink-muted">{t('preview.countsTitle')}</p>
                <ul className="space-y-0.5 text-xs text-ink-muted">
                  {COUNT_ROWS.filter((key) => preview.counts[key] > 0).map((key) => (
                    <li key={key}>{t(`preview.counts.${key}`, { count: preview.counts[key] })}</li>
                  ))}
                  {preview.assetCount > 0 && (
                    <li>
                      {t('preview.assetsLabel')}:{' '}
                      {t('preview.assetsValue', { count: preview.assetCount, size: formatBytes(preview.assetTotalBytes, locale) })}
                    </li>
                  )}
                </ul>
              </div>

              {(preview.connectedAccountsRequireReconnect > 0 ||
                preview.destinationsNeedStreamKey > 0 ||
                preview.donationSourcesNeedCredential > 0) && (
                <div>
                  <p className="mb-1 text-xs font-semibold uppercase text-ink-muted">{t('preview.attentionTitle')}</p>
                  <ul className="space-y-0.5 text-xs text-ink-muted">
                    {preview.connectedAccountsRequireReconnect > 0 && (
                      <li>{t('preview.attentionAccounts', { count: preview.connectedAccountsRequireReconnect })}</li>
                    )}
                    {preview.destinationsNeedStreamKey > 0 && (
                      <li>{t('preview.attentionKeys', { count: preview.destinationsNeedStreamKey })}</li>
                    )}
                    {preview.donationSourcesNeedCredential > 0 && (
                      <li>{t('preview.attentionDonations', { count: preview.donationSourcesNeedCredential })}</li>
                    )}
                  </ul>
                </div>
              )}

              {commitErrorMessage !== null && <p className="text-xs text-status-error">{commitErrorMessage}</p>}

              <div className="flex flex-wrap gap-2">
                <Button type="button" onClick={closePreview}>
                  {t('preview.cancelButton')}
                </Button>
                <Button
                  type="button"
                  variant="danger"
                  onClick={() => setConfirming(true)}
                  disabled={commitMutation.isPending}
                  data-testid="backup-restore-confirm-button"
                >
                  {t('preview.confirmButton')}
                </Button>
              </div>
            </div>
          )}
        </div>
      </PanelBody>

      <ConfirmDialog
        open={confirming}
        title={t('confirm.title')}
        message={t('confirm.message')}
        confirmLabel={t('confirm.confirmButton')}
        destructive
        busy={commitMutation.isPending}
        onCancel={() => setConfirming(false)}
        onConfirm={confirmRestore}
      />
    </Panel>
  );
}
