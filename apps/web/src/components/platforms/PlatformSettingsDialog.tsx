import { Loader2, Save, Trash2 } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { ProviderBrand } from '@/components/providers/ProviderBrand';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useApiFieldErrors } from '@/hooks/use-api-field-errors';
import { useDeletePlatformMutation, useUpdatePlatformMutation } from '@/hooks/use-platforms';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { DISPLAY_NAME_MAX_LENGTH, SORT_ORDER_MAX } from '@/models/platform-constraints';

import { AccountLinkSection } from './AccountLinkSection';
import { BroadcastSelectSection } from './BroadcastSelectSection';
import { OutputSettingsSection } from './OutputSettingsSection';
import { StreamKeySection } from './StreamKeySection';

type PlatformSettingsDialogProps = {
  platform: ConfiguredPlatform | null;
  onClose: () => void;
  /** Called after a successful delete so the caller can change selection. */
  onDeleted: (id: string) => void;
};

/**
 * Editor for one configured destination: display name, enabled state, sort
 * order, and deletion behind a confirmation step.
 *
 * The provider cannot be changed, because metadata validity depends on it;
 * switching provider means deleting and creating a destination.
 */
export function PlatformSettingsDialog({
  platform,
  onClose,
  onDeleted,
}: PlatformSettingsDialogProps) {
  const { t } = useTranslation(['platforms', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;
  const fieldErrorsOf = useApiFieldErrors();

  const updatePlatform = useUpdatePlatformMutation();
  const deletePlatform = useDeletePlatformMutation();

  const [displayName, setDisplayName] = useState(platform?.displayName ?? '');
  const [enabled, setEnabled] = useState(platform?.enabled ?? false);
  const [sortOrder, setSortOrder] = useState(String(platform?.sortOrder ?? 0));
  const [localError, setLocalError] = useState<string | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [saved, setSaved] = useState(false);

  if (platform === null) return null;

  const busy = updatePlatform.isPending || deletePlatform.isPending;
  const serverFieldErrors = fieldErrorsOf(updatePlatform.error);

  const handleClose = () => {
    if (busy) return;
    updatePlatform.reset();
    deletePlatform.reset();
    setLocalError(null);
    setSaved(false);
    onClose();
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) return;

    const trimmed = displayName.trim();
    if (trimmed === '') {
      setLocalError(t('platforms:validation.displayNameRequired'));
      return;
    }

    const parsedSortOrder = Number.parseInt(sortOrder, 10);
    if (Number.isNaN(parsedSortOrder) || parsedSortOrder < 0 || parsedSortOrder > SORT_ORDER_MAX) {
      setLocalError(t('platforms:validation.sortOrderInvalid'));
      return;
    }

    setLocalError(null);
    setSaved(false);
    updatePlatform.mutate(
      { id: platform.id, input: { displayName: trimmed, enabled, sortOrder: parsedSortOrder } },
      { onSuccess: () => setSaved(true) },
    );
  };

  const handleDelete = () => {
    deletePlatform.mutate(platform.id, {
      onSuccess: () => {
        setConfirmingDelete(false);
        onDeleted(platform.id);
        onClose();
      },
    });
  };

  const updateFailure =
    updatePlatform.error !== null && Object.keys(serverFieldErrors).length === 0
      ? resolveApiErrorMessage(tErrors, updatePlatform.error)
      : null;
  const deleteFailure =
    deletePlatform.error !== null ? resolveApiErrorMessage(tErrors, deletePlatform.error) : null;

  return (
    <>
      <Modal
        open
        onClose={handleClose}
        title={t('platforms:settings.title')}
        description={t('platforms:settings.description', { platform: platform.displayName })}
        dismissible={!busy}
        footer={
          <>
            <Button
              type="button"
              variant="danger"
              onClick={() => setConfirmingDelete(true)}
              disabled={busy}
              icon={<Trash2 className="size-3.5" />}
              className="mr-auto"
            >
              {t('platforms:settings.delete')}
            </Button>
            <Button type="button" onClick={handleClose} disabled={busy}>
              {t('common:actions.cancel')}
            </Button>
            <Button
              type="submit"
              form="platform-settings-form"
              variant="primary"
              disabled={busy}
              icon={
                updatePlatform.isPending ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Save className="size-3.5" />
                )
              }
            >
              {updatePlatform.isPending
                ? t('platforms:settings.saving')
                : t('platforms:settings.save')}
            </Button>
          </>
        }
      >
        <form id="platform-settings-form" onSubmit={handleSubmit} noValidate className="space-y-4">
          {(updateFailure ?? deleteFailure) !== null && (
            <p
              role="alert"
              className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error"
            >
              {updateFailure ?? deleteFailure}
            </p>
          )}

          <div className="flex items-center gap-3 rounded-lg border border-line bg-surface-sunken px-3 py-2">
            <ProviderBrand
              providerId={platform.providerId}
              fallbackLabel={platform.providerId.slice(0, 2).toUpperCase()}
              size="sm"
            />
            <div className="min-w-0">
              <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
                {t('platforms:settings.providerLabel')}
              </p>
              {/* Brand name, never translated, and not editable. */}
              <p className="mt-0.5 text-sm text-ink">
                {platform.provider?.brandName ?? platform.providerId}
              </p>
              <p className="mt-1 text-[11px] text-ink-faint">
                {t('platforms:settings.providerImmutable')}
              </p>
            </div>
          </div>

          <FormField
            label={t('platforms:settings.displayNameLabel')}
            error={localError ?? serverFieldErrors.displayName}
            counter={`${displayName.trim().length} / ${DISPLAY_NAME_MAX_LENGTH}`}
          >
            {({ inputId, describedBy }) => (
              <TextInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={localError !== null || serverFieldErrors.displayName !== undefined}
                value={displayName}
                maxLength={DISPLAY_NAME_MAX_LENGTH}
                disabled={busy}
                onChange={(event) => {
                  setDisplayName(event.target.value);
                  setLocalError(null);
                  setSaved(false);
                }}
              />
            )}
          </FormField>

          <FormField
            label={t('platforms:settings.sortOrderLabel')}
            hint={t('platforms:settings.sortOrderHint')}
            error={serverFieldErrors.sortOrder}
          >
            {({ inputId, describedBy }) => (
              <TextInput
                id={inputId}
                type="number"
                min={0}
                max={SORT_ORDER_MAX}
                aria-describedby={describedBy}
                value={sortOrder}
                disabled={busy}
                onChange={(event) => {
                  setSortOrder(event.target.value);
                  setSaved(false);
                }}
              />
            )}
          </FormField>

          <div className="rounded-lg border border-line bg-surface-sunken p-3">
            <ToggleSwitch
              label={t('platforms:settings.enabledLabel')}
              description={t('platforms:settings.enabledDescription')}
              checked={enabled}
              onCheckedChange={(next) => {
                setEnabled(next);
                setSaved(false);
              }}
            />
          </div>

          <p aria-live="polite" className="text-[11px] text-ink-faint">
            {saved ? (
              <span className="text-status-live">{t('platforms:settings.saved')}</span>
            ) : (
              t('platforms:settings.hint')
            )}
          </p>
        </form>

        {/* Keyed by platform id so its input, status and confirmation state
            never leak across platforms - closing the dialog (platform becomes
            null) unmounts it entirely, and opening a different platform
            remounts it fresh even if this dialog's own instance persists. */}
        <StreamKeySection key={platform.id} platform={platform} />
        <OutputSettingsSection key={`output-${platform.id}`} platform={platform} />
        <AccountLinkSection key={`account-link-${platform.id}`} platform={platform} />
        <BroadcastSelectSection key={`broadcast-${platform.id}`} platform={platform} />
      </Modal>

      <ConfirmDialog
        open={confirmingDelete}
        title={t('platforms:deleteDialog.title')}
        message={t('platforms:deleteDialog.message', { platform: platform.displayName })}
        confirmLabel={t('platforms:deleteDialog.confirm')}
        destructive
        busy={deletePlatform.isPending}
        onConfirm={handleDelete}
        onCancel={() => setConfirmingDelete(false)}
      />
    </>
  );
}
