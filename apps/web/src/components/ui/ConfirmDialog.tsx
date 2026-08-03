import { useTranslation } from 'react-i18next';

import { Button } from './Button';
import { Modal } from './Modal';

type ConfirmDialogProps = {
  open: boolean;
  title: string;
  message: string;
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  /** Uses the destructive button styling for irreversible actions. */
  destructive?: boolean;
  busy?: boolean;
};

/**
 * Application-styled confirmation.
 *
 * Replaces `window.confirm`, which cannot be translated, cannot be styled and
 * blocks the whole browser tab.
 */
export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel,
  onConfirm,
  onCancel,
  destructive = false,
  busy = false,
}: ConfirmDialogProps) {
  const { t } = useTranslation('common');

  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={title}
      size="sm"
      dismissible={!busy}
      footer={
        <>
          <Button type="button" onClick={onCancel} disabled={busy}>
            {t('actions.cancel')}
          </Button>
          <Button
            type="button"
            variant={destructive ? 'danger' : 'primary'}
            onClick={onConfirm}
            disabled={busy}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      <p className="text-sm leading-relaxed text-ink-muted">{message}</p>
    </Modal>
  );
}
