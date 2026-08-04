import { Download, Loader2, ShieldCheck } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { useInstallMediaMtxMutation } from '@/hooks/use-runtime';

type InstallMediaMtxDialogProps = {
  open: boolean;
  onClose: () => void;
  supportedVersion: string;
};

/**
 * Explicit installation flow for the managed MediaMTX dependency.
 *
 * Nothing is downloaded without this confirmation. The dialog states the exact
 * version, that it comes from the official release, that the checksum is
 * verified, and that MediaMTX is third-party software with its own licence -
 * so the user can make an informed decision rather than being surprised by a
 * background download.
 */
export function InstallMediaMtxDialog({
  open,
  onClose,
  supportedVersion,
}: InstallMediaMtxDialogProps) {
  const { t } = useTranslation(['runtime', 'common']);
  const install = useInstallMediaMtxMutation();

  const busy = install.isPending;

  const handleClose = () => {
    if (busy) return;
    install.reset();
    onClose();
  };

  const handleConfirm = () => {
    // Guards a second click while the request is in flight.
    if (busy) return;

    install.mutate(undefined, {
      onSuccess: () => {
        // The backend accepted the job; progress is shown by the runtime
        // panels, so the dialog closes rather than blocking on a long download.
        install.reset();
        onClose();
      },
    });
  };

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={t('runtime:install.title')}
      description={t('runtime:install.intro')}
      dismissible={!busy}
      footer={
        <>
          <Button type="button" onClick={handleClose} disabled={busy}>
            {t('runtime:install.cancel')}
          </Button>
          <Button
            type="button"
            variant="primary"
            onClick={handleConfirm}
            disabled={busy}
            icon={
              busy ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Download className="size-3.5" />
              )
            }
          >
            {busy ? t('runtime:controls.installing') : t('runtime:install.confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-3 text-xs leading-relaxed text-ink-muted">
        <p className="flex gap-2">
          <Download aria-hidden="true" className="mt-0.5 size-3.5 shrink-0 text-accent-soft" />
          <span>{t('runtime:install.versionLine', { version: supportedVersion })}</span>
        </p>
        <p className="flex gap-2">
          <ShieldCheck aria-hidden="true" className="mt-0.5 size-3.5 shrink-0 text-status-live" />
          <span>{t('runtime:install.checksumLine')}</span>
        </p>
        <p className="rounded-lg border border-line bg-surface-sunken px-3 py-2 text-[11px]">
          {t('runtime:install.thirdPartyLine')}
        </p>
        <p className="text-[11px] text-ink-faint">{t('runtime:install.sizeLine')}</p>
      </div>
    </Modal>
  );
}
