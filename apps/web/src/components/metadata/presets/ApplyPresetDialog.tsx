import { Check, Loader2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import type { FieldStatus } from '@/api/metadata-preset-apply-schemas';
import type { MetadataPreset } from '@/api/metadata-preset-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Modal } from '@/components/ui/Modal';
import { useApplyMetadataPresetMutation, useApplyPreviewQuery } from '@/hooks/use-metadata-presets';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

import { ProviderBrand } from '../../providers/ProviderBrand';

type ApplyPresetDialogProps = {
  open: boolean;
  onClose: () => void;
  preset: MetadataPreset;
  platforms: readonly ConfiguredPlatform[];
  /** The destination tab currently open in Stream details, if any. */
  activeId: string | null;
  /** Whether that open tab has unsaved edits right now. */
  activeDirty: boolean;
  /** Called after a successful apply, with the destination ids that were
   * actually written - lets the caller force the active form to reload
   * its draft when the applied set includes the open tab. */
  onApplied: (appliedIds: string[]) => void;
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

/**
 * "Apply preset" - compatibility preview then an explicit, atomic,
 * local-only apply across one or more destinations
 * (docs/metadata-presets.md §6). Never publishes anything: applying
 * only ever changes what MetadataForm itself would save.
 */
export function ApplyPresetDialog({
  open,
  onClose,
  preset,
  platforms,
  activeId,
  activeDirty,
  onApplied,
}: ApplyPresetDialogProps) {
  const { t } = useTranslation(['metadataPresets', 'metadata', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;

  const [selected, setSelected] = useState<string[]>([]);
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  const [applied, setApplied] = useState(false);

  const applyPreset = useApplyMetadataPresetMutation();
  const preview = useApplyPreviewQuery(preset.id, selected);
  const previewById = new Map((preview.data ?? []).map((p) => [p.platformId, p]));

  const busy = applyPreset.isPending;

  const toggle = (id: string, checked: boolean) => {
    setSelected((current) => (checked ? [...current, id] : current.filter((x) => x !== id)));
    setApplied(false);
  };

  const handleClose = () => {
    if (busy) return;
    setSelected([]);
    setConfirmingDiscard(false);
    setApplied(false);
    applyPreset.reset();
    onClose();
  };

  const invalidSelected = selected.some((id) => previewById.get(id)?.valid === false);
  const canApply = selected.length > 0 && !invalidSelected && !preview.isFetching;

  const runApply = () => {
    applyPreset.mutate(
      { presetId: preset.id, platformIds: selected },
      {
        onSuccess: () => {
          setApplied(true);
          onApplied(selected);
          window.setTimeout(() => {
            handleClose();
          }, 900);
        },
      },
    );
  };

  const handleApplyClick = () => {
    if (!canApply || busy) return;
    if (activeDirty && activeId !== null && selected.includes(activeId)) {
      setConfirmingDiscard(true);
      return;
    }
    runApply();
  };

  const generalError = applyPreset.error !== null ? resolveApiErrorMessage(tErrors, applyPreset.error) : null;

  return (
    <>
      <Modal
        open={open}
        onClose={handleClose}
        title={t('metadataPresets:apply.title', { preset: preset.name })}
        description={t('metadataPresets:apply.description')}
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
              disabled={!canApply || busy}
              icon={
                busy ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : applied ? (
                  <Check className="size-3.5" />
                ) : undefined
              }
            >
              {applied
                ? t('metadataPresets:apply.applied')
                : busy
                  ? t('metadataPresets:apply.applying')
                  : t('metadataPresets:apply.submit')}
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

          {platforms.length === 0 ? (
            <p className="text-sm text-ink-muted">{t('metadataPresets:apply.noDestinations')}</p>
          ) : (
            <ul className="space-y-3">
              {platforms.map((platform) => {
                const platformPreview = previewById.get(platform.id);
                return (
                  <li key={platform.id} className="rounded-lg border border-line p-3">
                    <label className="flex cursor-pointer items-center gap-2 text-sm text-ink">
                      <input
                        type="checkbox"
                        checked={selected.includes(platform.id)}
                        disabled={busy}
                        onChange={(event) => toggle(platform.id, event.target.checked)}
                        className="size-4 rounded border-line accent-accent"
                      />
                      <ProviderBrand
                        providerId={platform.providerId}
                        fallbackLabel={platform.providerId.slice(0, 2).toUpperCase()}
                        size="sm"
                      />
                      <span className="min-w-0 flex-1 truncate">{platform.displayName}</span>
                    </label>

                    {selected.includes(platform.id) && (
                      <div className="mt-2 pl-6" aria-live="polite">
                        {platformPreview === undefined ? (
                          <p className="text-[11px] text-ink-faint">{t('metadataPresets:apply.checking')}</p>
                        ) : platformPreview.valid ? (
                          <div className="flex flex-wrap gap-1.5">
                            {FIELD_ORDER.map((field) => {
                              const entry = platformPreview.fields.find((f) => f.field === field);
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
                            {platformPreview.errors !== undefined && (
                              <span className="block text-ink-faint">
                                {Object.values(platformPreview.errors).join(' ')}
                              </span>
                            )}
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
