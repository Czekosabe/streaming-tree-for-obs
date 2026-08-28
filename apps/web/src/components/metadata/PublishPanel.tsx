import { Loader2, UploadCloud } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { usePlatformAccountLinkQuery, usePublishMetadataMutation, usePublishPreviewQuery } from '@/hooks/use-accounts';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { publishBlockerKey, publishWarningKey } from '@/models/account-presentation';

type PublishPanelProps = {
  platform: ConfiguredPlatform;
  /** Publishing is disabled while the local form has unsaved edits - it
   * always sends what is currently saved, never an in-progress draft. */
  dirty: boolean;
};

const PUBLISHABLE_PROVIDERS = new Set(['twitch', 'youtube']);

/**
 * "Publish to <provider>" panel: a preview of what would change, and an
 * explicit publish action behind a confirmation.
 *
 * Deliberately separate from the local Save action above it in the form -
 * saving stores metadata in Streaming Tree; publishing sends it to the
 * provider. The two are never combined behind one button. Shared between
 * Twitch and YouTube, since both publish through the same non-secret
 * preview/result shape (internal/httpapi/accounts.go dispatches by the
 * destination's own provider server-side); every other provider is local-
 * only and this panel renders nothing for it.
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

  if (!PUBLISHABLE_PROVIDERS.has(platform.providerId)) return null;

  const providerName = platform.provider?.brandName ?? platform.providerId;
  const linkPrefix = platform.providerId === 'youtube' ? 'accounts:youtube.link' : 'accounts:link';

  if (!linked) {
    return (
      <div className="rounded-lg bg-surface-sunken/70 p-3">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t('accounts:publish.heading', { provider: providerName })}
        </p>
        <p className="mt-1 text-[11px] text-ink-faint">{t(`${linkPrefix}.notLinked`)}</p>
      </div>
    );
  }

  if (dirty) {
    return (
      <div className="rounded-lg bg-surface-sunken/70 p-3">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t('accounts:publish.heading', { provider: providerName })}
        </p>
        <p className="mt-1 text-[11px] text-status-warning">
          {t('accounts:publish.unsavedNote', { provider: providerName })}
        </p>
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

  const result = publishMutation.data;

  return (
    <div className="space-y-3 rounded-lg bg-surface-sunken/70 p-3">
      <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {t('accounts:publish.heading', { provider: providerName })}
      </p>
      <p className="text-[11px] text-ink-faint">
        {t('accounts:publish.description', { provider: providerName })}
      </p>

      {preview?.broadcastTitle !== undefined && preview.broadcastTitle !== '' && (
        <p className="text-[11px] text-ink-faint">
          {t('accounts:publish.broadcastLabel', { title: preview.broadcastTitle })}
        </p>
      )}

      {previewQuery.isLoading && (
        <p className="flex items-center gap-1.5 text-xs text-ink-muted">
          <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
          {t('accounts:publish.loadingPreview', { provider: providerName })}
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

          {(preview.warnings ?? []).length > 0 && (
            <ul className="space-y-1">
              {(preview.warnings ?? []).map((warning) => {
                const key = publishWarningKey(warning);
                return (
                  <li key={warning} className="text-[11px] text-ink-faint">
                    {key === null ? warning : t(key)}
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
                        {t('accounts:publish.remoteValue', { provider: providerName })}: {field.remote || '--'}
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
                  {t('accounts:publish.skippedFields', {
                    provider: providerName,
                    fields: preview.skipped.join(', '),
                  })}
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
                  : t('accounts:publish.publishButton', { provider: providerName })}
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

      {result !== undefined && (result.fieldsFailed ?? []).length > 0 && (
        <p role="alert" className="text-xs text-status-error">
          {t('accounts:publish.skippedFields', {
            provider: providerName,
            fields: (result.fieldsFailed ?? []).join(', '),
          })}
        </p>
      )}

      <p aria-live="polite" className="text-[11px] text-status-live">
        {justPublished ? t('accounts:publish.success', { provider: providerName }) : ''}
      </p>

      <ConfirmDialog
        open={confirming}
        title={t('accounts:publish.confirmDialog.title', { provider: providerName })}
        message={t('accounts:publish.confirmDialog.message', { provider: providerName })}
        confirmLabel={t('accounts:publish.confirmDialog.confirm')}
        busy={publishMutation.isPending}
        onConfirm={handlePublish}
        onCancel={() => setConfirming(false)}
      />
    </div>
  );
}
