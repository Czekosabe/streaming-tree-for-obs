import { Loader2, UploadCloud } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { usePlatformAccountLinkQuery, usePublishMetadataMutation, usePublishPreviewQuery } from '@/hooks/use-accounts';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { publishBlockerKey } from '@/models/account-presentation';

type PublishPanelProps = {
  platform: ConfiguredPlatform;
  /** Publishing is disabled while the local form has unsaved edits - it
   * always sends what is currently saved, never an in-progress draft. */
  dirty: boolean;
};

/**
 * "Publish to Twitch" panel: a preview of what would change, and an
 * explicit publish action behind a confirmation.
 *
 * Deliberately separate from the local Save action above it in the form -
 * saving stores metadata in Streaming Tree; publishing sends it to Twitch.
 * The two are never combined behind one button.
 */
export function PublishPanel({ platform, dirty }: PublishPanelProps) {
  const { t } = useTranslation(['accounts', 'errors']);
  const tErrors = useTranslation('errors').t;

  const linkQuery = usePlatformAccountLinkQuery(platform.id);
  const linked = linkQuery.data !== null && linkQuery.data !== undefined;

  const previewQuery = usePublishPreviewQuery(platform.id, linked && !dirty);
  const publishMutation = usePublishMetadataMutation();

  const [confirming, setConfirming] = useState(false);
  const [justPublished, setJustPublished] = useState(false);

  if (platform.providerId !== 'twitch') return null;
  if (!linked) {
    return (
      <div className="rounded-lg border border-line bg-surface-sunken p-3">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t('accounts:publish.heading')}
        </p>
        <p className="mt-1 text-[11px] text-ink-faint">{t('accounts:link.notLinked')}</p>
      </div>
    );
  }

  if (dirty) {
    return (
      <div className="rounded-lg border border-line bg-surface-sunken p-3">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t('accounts:publish.heading')}
        </p>
        <p className="mt-1 text-[11px] text-status-warning">{t('accounts:publish.unsavedNote')}</p>
      </div>
    );
  }

  const preview = previewQuery.data;
  const changedFields = preview?.fields.filter((f) => f.changed) ?? [];

  const handlePublish = () => {
    publishMutation.mutate(platform.id, {
      onSuccess: () => {
        setConfirming(false);
        setJustPublished(true);
      },
    });
  };

  return (
    <div className="space-y-3 rounded-lg border border-line bg-surface-sunken p-3">
      <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {t('accounts:publish.heading')}
      </p>
      <p className="text-[11px] text-ink-faint">{t('accounts:publish.description')}</p>

      {previewQuery.isLoading && (
        <p className="flex items-center gap-1.5 text-xs text-ink-muted">
          <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
          {t('accounts:publish.loadingPreview')}
        </p>
      )}

      {previewQuery.isError && (
        <p role="alert" className="text-xs text-status-error">
          {resolveApiErrorMessage(tErrors, previewQuery.error)}
        </p>
      )}

      {preview !== undefined && (
        <>
          {preview.blockers.length > 0 && (
            <ul className="space-y-1">
              {preview.blockers.map((blocker) => {
                const key = publishBlockerKey(blocker);
                return (
                  <li key={blocker} className="text-[11px] text-status-warning">
                    {key === null ? blocker : t(key)}
                  </li>
                );
              })}
            </ul>
          )}

          {preview.allowed && (
            <>
              {changedFields.length === 0 ? (
                <p className="text-[11px] text-ink-faint">{t('accounts:publish.fieldUnchanged')}</p>
              ) : (
                <ul className="space-y-1.5 text-[11px]">
                  {changedFields.map((field) => (
                    <li key={field.field} className="rounded border border-line bg-surface px-2 py-1.5">
                      <p className="font-medium text-ink">{field.field}</p>
                      <p className="text-ink-faint">
                        {t('accounts:publish.remoteValue')}: {field.remote || '--'}
                      </p>
                      <p className="text-ink-faint">
                        {t('accounts:publish.localValue')}: {field.local || '--'}
                      </p>
                    </li>
                  ))}
                </ul>
              )}

              {preview.skipped.length > 0 && (
                <p className="text-[11px] text-ink-faint">
                  {t('accounts:publish.skippedFields', { fields: preview.skipped.join(', ') })}
                </p>
              )}

              <Button
                type="button"
                variant="primary"
                size="sm"
                disabled={publishMutation.isPending || changedFields.length === 0}
                icon={
                  publishMutation.isPending ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <UploadCloud className="size-3.5" />
                  )
                }
                onClick={() => setConfirming(true)}
              >
                {publishMutation.isPending
                  ? t('accounts:publish.publishing')
                  : t('accounts:publish.publishButton')}
              </Button>
            </>
          )}
        </>
      )}

      {publishMutation.error !== null && (
        <p role="alert" className="text-xs text-status-error">
          {resolveApiErrorMessage(tErrors, publishMutation.error)}
        </p>
      )}

      <p aria-live="polite" className="text-[11px] text-status-live">
        {justPublished ? t('accounts:publish.success') : ''}
      </p>

      <ConfirmDialog
        open={confirming}
        title={t('accounts:publish.confirmDialog.title')}
        message={t('accounts:publish.confirmDialog.message')}
        confirmLabel={t('accounts:publish.confirmDialog.confirm')}
        busy={publishMutation.isPending}
        onConfirm={handlePublish}
        onCancel={() => setConfirming(false)}
      />
    </div>
  );
}
