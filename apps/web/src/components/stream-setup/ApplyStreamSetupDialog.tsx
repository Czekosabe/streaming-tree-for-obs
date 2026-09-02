import { AlertTriangle, Check, Loader2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { FieldStatus } from '@/api/metadata-preset-apply-schemas';
import type { StreamSetupProfile } from '@/api/stream-setup-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Modal } from '@/components/ui/Modal';
import { useApplyStreamSetupMutation, useStreamSetupPreviewQuery } from '@/hooks/use-stream-setups';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

import { ProviderBrand } from '../providers/ProviderBrand';

type ApplyStreamSetupDialogProps = {
  open: boolean;
  onClose: () => void;
  profile: StreamSetupProfile;
  /** The destination tab currently open in Stream details, if any. */
  activeMetadataId: string | null;
  /** Whether that open tab has unsaved edits right now. */
  activeMetadataDirty: boolean;
};

const FIELD_ORDER = [
  'title', 'description', 'category', 'tags',
  'language', 'visibility', 'matureContent', 'dvr', 'latencyMode',
] as const;

const STATUS_CLASSES: Record<FieldStatus, string> = {
  will_change: 'bg-status-warning/15 text-status-warning border-status-warning/30',
  unchanged: 'bg-surface-sunken text-ink-faint border-line',
  not_supported: 'bg-surface-sunken text-ink-faint/60 border-line/60',
};

const CHANGE_CLASSES: Record<string, string> = {
  will_enable: 'bg-status-live/15 text-status-live border-status-live/30',
  will_disable: 'bg-status-error/15 text-status-error border-status-error/30',
  unchanged: 'bg-surface-sunken text-ink-faint border-line',
  missing: 'bg-status-warning/15 text-status-warning border-status-warning/30',
};

/**
 * "Apply setup" - compatibility/change preview, then an explicit,
 * local-only apply (docs/stream-setup-profiles.md §3/§6/§17): shows
 * exactly which destinations will be enabled/disabled and what the
 * referenced metadata preset (if any) would change, blocks entirely
 * if an affected destination is currently streaming, and never starts
 * a stream, publishes metadata, or touches a credential itself.
 */
export function ApplyStreamSetupDialog({
  open,
  onClose,
  profile,
  activeMetadataId,
  activeMetadataDirty,
}: ApplyStreamSetupDialogProps) {
  const { t } = useTranslation(['streamSetups', 'metadataPresets', 'metadata', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;

  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  const [applied, setApplied] = useState(false);

  const preview = useStreamSetupPreviewQuery(profile.id);
  const applySetup = useApplyStreamSetupMutation();
  const busy = applySetup.isPending;

  const handleClose = () => {
    if (busy) return;
    setConfirmingDiscard(false);
    setApplied(false);
    applySetup.reset();
    onClose();
  };

  const metadataPreviewById = new Map(
    (preview.data?.metadataDestinationPreviews ?? []).map((p) => [p.platformId, p]),
  );
  const touchesActiveMetadata =
    activeMetadataId !== null && metadataPreviewById.has(activeMetadataId);

  const canApply = preview.isSuccess && !preview.data.blocked && !busy;

  const runApply = () => {
    applySetup.mutate(profile.id, {
      onSuccess: () => {
        setApplied(true);
        window.setTimeout(() => handleClose(), 900);
      },
    });
  };

  const handleApplyClick = () => {
    if (!canApply) return;
    if (activeMetadataDirty && touchesActiveMetadata) {
      setConfirmingDiscard(true);
      return;
    }
    runApply();
  };

  const generalError = applySetup.error !== null ? resolveApiErrorMessage(tErrors, applySetup.error) : null;

  return (
    <>
      <Modal
        open={open}
        onClose={handleClose}
        title={t('streamSetups:apply.title', { setup: profile.name })}
        description={t('streamSetups:apply.description')}
        dismissible={!busy}
        footer={
          <>
            <Button type="button" onClick={handleClose} disabled={busy}>
              {t('common:actions.cancel')}
            </Button>
            <Button
              type="button"
              variant="primary"
              onClick={handleApplyClick}
              disabled={!canApply}
              icon={
                busy ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : applied ? (
                  <Check className="size-3.5" />
                ) : undefined
              }
            >
              {applied
                ? t('streamSetups:apply.applied')
                : busy
                  ? t('streamSetups:apply.applying')
                  : t('streamSetups:apply.submit')}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          {generalError !== null && (
            <p role="alert" className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error">
              {generalError}
            </p>
          )}

          {preview.isLoading && <p className="text-sm text-ink-muted">{t('streamSetups:apply.checking')}</p>}

          {preview.isError && (
            <p role="alert" className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error">
              {resolveApiErrorMessage(tErrors, preview.error)}
            </p>
          )}

          {preview.isSuccess && preview.data.blocked && (
            <p
              role="alert"
              className="flex items-start gap-2 rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error"
            >
              <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
              {t('streamSetups:apply.blocked')}
            </p>
          )}

          {preview.isSuccess && preview.data.metadataPresetMissing && (
            <p className="rounded-lg border border-status-warning/30 bg-status-warning/10 px-3 py-2 text-xs text-status-warning">
              {t('streamSetups:apply.presetMissing')}
            </p>
          )}

          {preview.isSuccess && (
            <ul className="space-y-3">
              {preview.data.destinations.map((destination) => {
                const metadataPreview = metadataPreviewById.get(destination.platformId);
                return (
                  <li key={`${destination.platformId}-${destination.displayName}`} className="rounded-lg border border-line p-3">
                    <div className="flex items-center gap-2 text-sm text-ink">
                      <ProviderBrand
                        providerId={destination.providerId}
                        fallbackLabel={destination.providerId.slice(0, 2).toUpperCase()}
                        size="sm"
                      />
                      <span className="min-w-0 flex-1 truncate">{destination.displayName}</span>
                      <span
                        className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${CHANGE_CLASSES[destination.change] ?? ''}`}
                      >
                        {t(`streamSetups:apply.change.${destination.change}` as const)}
                      </span>
                      {destination.active && (
                        <span className="rounded-full border border-status-error/30 bg-status-error/10 px-2 py-0.5 text-[10px] font-medium text-status-error">
                          {t('streamSetups:apply.active')}
                        </span>
                      )}
                    </div>

                    {metadataPreview !== undefined && (
                      <div className="mt-2 pl-6">
                        {metadataPreview.valid ? (
                          <div className="flex flex-wrap gap-1.5">
                            {FIELD_ORDER.map((field) => {
                              const entry = metadataPreview.fields.find((f) => f.field === field);
                              if (entry === undefined) return null;
                              return (
                                <span
                                  key={field}
                                  className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${STATUS_CLASSES[entry.status]}`}
                                >
                                  {t(`metadata:fields.${field}` as const)}
                                  {': '}
                                  {t(`metadataPresets:apply.status.${entry.status}` as const)}
                                </span>
                              );
                            })}
                          </div>
                        ) : (
                          <p className="text-[11px] text-status-error">
                            {t('metadataPresets:apply.incompatible')}
                          </p>
                        )}
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </Modal>

      <ConfirmDialog
        open={confirmingDiscard}
        title={t('metadataPresets:apply.discardDialog.title')}
        message={t('metadataPresets:apply.discardDialog.message')}
        confirmLabel={t('metadataPresets:apply.discardDialog.confirm')}
        destructive
        onConfirm={() => {
          setConfirmingDiscard(false);
          runApply();
        }}
        onCancel={() => setConfirmingDiscard(false)}
      />
    </>
  );
}
