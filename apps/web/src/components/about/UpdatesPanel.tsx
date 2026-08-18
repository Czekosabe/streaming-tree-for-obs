import { Download } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  useCheckForUpdateMutation,
  useDownloadUpdateMutation,
  useInstallUpdateMutation,
  useSetAutoCheckMutation,
  useUpdateStatusQuery,
} from '@/hooks/use-updates';
import { updateBlockerKey, updateErrorKey } from '@/models/updates-presentation';

/**
 * The Stage 20B "Updates" panel (docs/updater.md §31) - lives on the About
 * & Legal page, next to the Quit action, since both are process-lifecycle
 * concerns. A development build shows an honest notice instead of any of
 * the controls below (docs/updater.md §35) - the backend itself already
 * refuses every action in that case, this only avoids offering buttons
 * that would just come back with an error.
 */
export function UpdatesPanel() {
  const { t } = useTranslation('updates');
  const { data: status } = useUpdateStatusQuery();
  const setAutoCheck = useSetAutoCheckMutation();
  const checkNow = useCheckForUpdateMutation();
  const download = useDownloadUpdateMutation();
  const install = useInstallUpdateMutation();
  const [confirming, setConfirming] = useState(false);

  if (status === undefined) {
    return null;
  }

  if (!status.releaseBuild) {
    return (
      <Panel>
        <PanelHeader title={t('panel.heading')} icon={<Download className="size-4" />} />
        <PanelBody>
          <p className="text-sm text-ink-muted">{t('panel.developmentBuildNotice')}</p>
        </PanelBody>
      </Panel>
    );
  }

  const isChecking = status.state === 'checking';
  const showAvailable =
    status.state === 'available' || status.state === 'downloading' || status.state === 'ready_to_install';
  const downloadPercent =
    status.totalBytes !== undefined && status.totalBytes > 0
      ? Math.min(100, Math.round(((status.downloadedBytes ?? 0) / status.totalBytes) * 100))
      : 0;

  return (
    <Panel>
      <PanelHeader title={t('panel.heading')} icon={<Download className="size-4" />} />
      <PanelBody className="space-y-3">
        {status.postUpdateOutcome !== undefined && (
          <p
            className={
              status.postUpdateOutcome === 'ok'
                ? 'rounded-lg bg-status-success/10 px-3 py-2 text-xs text-status-success'
                : 'rounded-lg bg-status-error/10 px-3 py-2 text-xs text-status-error'
            }
          >
            {status.postUpdateOutcome === 'ok'
              ? t('postUpdate.success', { version: status.postUpdateToVersion })
              : t('postUpdate.failure')}
          </p>
        )}

        <p className="text-sm text-ink">
          <span className="text-ink-muted">{t('panel.currentVersionLabel')}: </span>
          {status.currentVersion}
        </p>
        <p className="text-sm text-ink">
          <span className="text-ink-muted">{t('panel.channelLabel')}: </span>
          {t('panel.channelStable')}
        </p>

        <ToggleSwitch
          label={t('panel.autoCheckLabel')}
          description={t('panel.autoCheckDescription')}
          checked={status.autoCheck}
          onCheckedChange={(checked) => setAutoCheck.mutate(checked)}
        />

        <p className="text-[11px] text-ink-faint">
          {t('panel.lastCheckedLabel')}:{' '}
          {status.lastSuccessfulCheckAt !== undefined
            ? new Date(status.lastSuccessfulCheckAt).toLocaleString()
            : t('panel.neverChecked')}
        </p>

        <Button type="button" onClick={() => checkNow.mutate()} disabled={isChecking}>
          {isChecking ? t('panel.checking') : t('panel.checkButton')}
        </Button>

        {status.state === 'up_to_date' && (
          <p className="text-xs text-ink-muted">{t('state.upToDate')}</p>
        )}
        {status.state === 'error' && (
          <p className="text-xs text-status-error">{t(updateErrorKey(status.lastErrorCode))}</p>
        )}

        {showAvailable && status.latestVersion !== undefined && (
          <div className="space-y-2 border-t border-line pt-3">
            <p className="text-sm font-medium text-ink">
              {t('available.versionLabel', { version: status.latestVersion })}
            </p>

            {status.releaseNotes !== undefined && status.releaseNotes !== '' && (
              <div>
                <p className="text-xs font-medium text-ink-muted">{t('available.releaseNotesLabel')}</p>
                <p className="mt-1 max-h-40 overflow-y-auto whitespace-pre-wrap rounded-lg bg-surface-sunken p-2 text-xs text-ink-faint">
                  {status.releaseNotes}
                </p>
                {status.releaseNotesTruncated === true && (
                  <p className="mt-1 text-[11px] text-ink-faint">{t('available.releaseNotesTruncated')}</p>
                )}
              </div>
            )}

            {status.state === 'available' && (
              <Button type="button" variant="primary" onClick={() => download.mutate()}>
                {t('available.updateNow')}
              </Button>
            )}

            {status.state === 'downloading' && (
              <p className="text-xs text-ink-muted">
                {t('downloading.label')} {downloadPercent}%
              </p>
            )}

            {status.state === 'ready_to_install' && (
              <>
                <p className="text-xs text-ink-muted">{t('readyToInstall.label')}</p>
                <Button
                  type="button"
                  variant="primary"
                  onClick={() => setConfirming(true)}
                  disabled={status.installBlocked}
                >
                  {t('readyToInstall.installButton')}
                </Button>
                {status.installBlocked && (
                  <p className="text-xs text-status-error">{t(updateBlockerKey(status.blockerCode))}</p>
                )}
              </>
            )}
          </div>
        )}
      </PanelBody>

      <ConfirmDialog
        open={confirming}
        title={t('install.confirmTitle')}
        message={t('install.confirmMessage')}
        confirmLabel={t('install.confirmButton')}
        busy={install.isPending}
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          install.mutate();
          setConfirming(false);
        }}
      />
    </Panel>
  );
}
