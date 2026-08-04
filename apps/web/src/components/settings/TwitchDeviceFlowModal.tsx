import { ExternalLink, Loader2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { CopyableValue } from '@/components/runtime/CopyableValue';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import {
  useCancelDeviceFlowMutation,
  useDeviceFlowQuery,
  useReconnectAccountMutation,
  useStartDeviceFlowMutation,
} from '@/hooks/use-accounts';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { deviceFlowIsTerminal, deviceFlowStateKey } from '@/models/account-presentation';

type TwitchDeviceFlowModalProps = {
  open: boolean;
  onClose: () => void;
  /**
   * When set, this modal starts a reconnect attempt for an existing account
   * instead of a fresh connection - Twitch must authorize the same account,
   * or the backend rejects the attempt with an identity-mismatch error.
   */
  reconnectAccountId?: string;
  /** Called once the attempt reaches `authorized`. */
  onAuthorized?: () => void;
};

/**
 * Twitch device-authorization modal.
 *
 * Shows the user code (safe to display and copy) and a link to open Twitch;
 * it never has access to the device code, because
 * DeviceFlowSnapshot (api/account-schemas.ts) has no field for one at all -
 * not filtered out here, structurally absent from what the backend ever
 * sends. Opening Twitch always requires an explicit click - no popup is
 * opened automatically.
 */
export function TwitchDeviceFlowModal({
  open,
  onClose,
  reconnectAccountId,
  onAuthorized,
}: TwitchDeviceFlowModalProps) {
  const { t } = useTranslation(['accounts', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;

  const startMutation = useStartDeviceFlowMutation();
  const reconnectMutation = useReconnectAccountMutation();
  const cancelMutation = useCancelDeviceFlowMutation();

  const [attemptId, setAttemptId] = useState<string | null>(null);
  const startedForOpen = useRef(false);

  const deviceFlowQuery = useDeviceFlowQuery(attemptId);
  const snapshot = deviceFlowQuery.data ?? null;
  const terminal = snapshot !== null && deviceFlowIsTerminal(snapshot.state);

  useEffect(() => {
    if (!open) {
      startedForOpen.current = false;
      setAttemptId(null);
      return;
    }
    if (startedForOpen.current) return;
    startedForOpen.current = true;

    // Both mutations resolve to a DeviceFlowSnapshot; the reconnect one just
    // needs the account id as its argument.
    if (reconnectAccountId === undefined) {
      startMutation.mutate(undefined, { onSuccess: (data) => setAttemptId(data.attemptId) });
    } else {
      reconnectMutation.mutate(reconnectAccountId, {
        onSuccess: (data) => setAttemptId(data.attemptId),
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- start exactly once per open, not on every mutation identity change
  }, [open, reconnectAccountId]);

  useEffect(() => {
    if (snapshot?.state === 'authorized') {
      onAuthorized?.();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire once when this specific attempt authorizes
  }, [snapshot?.state, snapshot?.attemptId]);

  if (!open) return null;

  const handleClose = () => {
    if (attemptId !== null && snapshot !== null && !terminal) {
      cancelMutation.mutate(attemptId);
    }
    onClose();
  };

  const startError = startMutation.error ?? reconnectMutation.error;
  const busy = startMutation.isPending || reconnectMutation.isPending;

  const expiresLabel = (() => {
    if (snapshot?.expiresAt === undefined) return null;
    const remainingMs = new Date(snapshot.expiresAt).getTime() - Date.now();
    if (remainingMs <= 0) return null;
    const minutes = Math.ceil(remainingMs / 60_000);
    return t('accounts:deviceFlow.expiresIn', { count: minutes });
  })();

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={t('accounts:deviceFlow.title')}
      description={t('accounts:deviceFlow.description')}
      dismissible={!cancelMutation.isPending}
      footer={
        terminal ? (
          <Button type="button" variant="primary" onClick={handleClose}>
            {t('accounts:deviceFlow.close')}
          </Button>
        ) : (
          <Button
            type="button"
            onClick={handleClose}
            disabled={cancelMutation.isPending}
            icon={cancelMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : undefined}
          >
            {t('accounts:deviceFlow.cancel')}
          </Button>
        )
      }
    >
      <div className="space-y-4">
        {busy && attemptId === null && (
          <div className="flex items-center justify-center gap-2 py-6 text-sm text-ink-muted">
            <Loader2 aria-hidden="true" className="size-4 animate-spin" />
            {t('accounts:deviceFlow.state.requesting_code')}
          </div>
        )}

        {startError !== null && attemptId === null && (
          <p role="alert" className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error">
            {resolveApiErrorMessage(tErrors, startError)}
          </p>
        )}

        {snapshot !== null && (
          <>
            {snapshot.userCode !== undefined && !terminal && (
              <>
                <CopyableValue
                  label={t('accounts:deviceFlow.userCodeLabel')}
                  value={snapshot.userCode}
                  copyLabel={t('accounts:deviceFlow.copyCode')}
                />
                {snapshot.verificationUri !== undefined && (
                  <div className="space-y-2">
                    <p className="text-xs text-ink-muted">
                      {t('accounts:deviceFlow.verificationNote', { uri: snapshot.verificationUri })}
                    </p>
                    <a
                      href={snapshot.verificationUri}
                      target="_blank"
                      rel="noreferrer noopener"
                      className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface-raised px-3 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-surface-hover"
                    >
                      {t('accounts:deviceFlow.openTwitch')}
                      <ExternalLink aria-hidden="true" className="size-3.5" />
                    </a>
                  </div>
                )}
                {expiresLabel !== null && <p className="text-[11px] text-ink-faint">{expiresLabel}</p>}
              </>
            )}

            <p aria-live="polite" className="flex items-center gap-2 text-sm text-ink-muted">
              {(snapshot.state === 'waiting_for_user' || snapshot.state === 'polling') && (
                <Loader2 aria-hidden="true" className="size-4 animate-spin" />
              )}
              {t(deviceFlowStateKey(snapshot.state))}
            </p>

            {snapshot.state === 'authorized' && (
              <p className="text-sm text-status-live">{t('accounts:deviceFlow.successNote')}</p>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}
