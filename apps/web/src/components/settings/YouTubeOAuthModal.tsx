import { ExternalLink, Loader2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import {
  useCancelYouTubeAttemptMutation,
  useReconnectYouTubeAccountMutation,
  useSelectYouTubeChannelMutation,
  useStartYouTubeAttemptMutation,
  useYouTubeAttemptQuery,
} from '@/hooks/use-accounts';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { oauthAttemptIsTerminal, oauthAttemptStateKey } from '@/models/account-presentation';

type YouTubeOAuthModalProps = {
  open: boolean;
  onClose: () => void;
  /**
   * When set, this modal starts a reconnect attempt for an existing account
   * instead of a fresh connection - Google must authorize the same channel,
   * or the backend rejects the attempt with an identity-mismatch error.
   */
  reconnectAccountId?: string;
  /** Called once the attempt reaches `authorized`. */
  onAuthorized?: () => void;
};

/**
 * YouTube Authorization Code + PKCE modal.
 *
 * Shows an "Open Google authorization" action instead of a raw URL, and
 * never has access to the authorization code, PKCE verifier or state value -
 * OAuthAttemptSnapshot (api/account-schemas.ts) has no field for any of
 * them at all, not filtered out here, structurally absent from what the
 * backend ever sends. Opening Google always requires an explicit click - no
 * popup is opened automatically.
 */
export function YouTubeOAuthModal({
  open,
  onClose,
  reconnectAccountId,
  onAuthorized,
}: YouTubeOAuthModalProps) {
  const { t } = useTranslation(['accounts', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;

  const startMutation = useStartYouTubeAttemptMutation();
  const reconnectMutation = useReconnectYouTubeAccountMutation();
  const cancelMutation = useCancelYouTubeAttemptMutation();
  const selectChannelMutation = useSelectYouTubeChannelMutation();

  const [attemptId, setAttemptId] = useState<string | null>(null);
  const startedForOpen = useRef(false);

  const attemptQuery = useYouTubeAttemptQuery(attemptId);
  const snapshot = attemptQuery.data ?? null;
  const terminal = snapshot !== null && oauthAttemptIsTerminal(snapshot.state);

  useEffect(() => {
    if (!open) {
      startedForOpen.current = false;
      setAttemptId(null);
      return;
    }
    if (startedForOpen.current) return;
    startedForOpen.current = true;

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
    return t('accounts:oauthAttempt.expiresIn', { count: minutes });
  })();

  const handleSelectChannel = (channelId: string) => {
    if (attemptId === null) return;
    selectChannelMutation.mutate({ attemptId, channelId });
  };

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={t('accounts:oauthAttempt.title')}
      description={t('accounts:oauthAttempt.description')}
      dismissible={!cancelMutation.isPending}
      footer={
        terminal ? (
          <Button type="button" variant="primary" onClick={handleClose}>
            {t('accounts:oauthAttempt.close')}
          </Button>
        ) : (
          <Button
            type="button"
            onClick={handleClose}
            disabled={cancelMutation.isPending}
            icon={cancelMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : undefined}
          >
            {t('accounts:oauthAttempt.cancel')}
          </Button>
        )
      }
    >
      <div className="space-y-4">
        {busy && attemptId === null && (
          <div className="flex items-center justify-center gap-2 py-6 text-sm text-ink-muted">
            <Loader2 aria-hidden="true" className="size-4 animate-spin" />
            {t('accounts:oauthAttempt.state.creating')}
          </div>
        )}

        {startError !== null && attemptId === null && (
          <p role="alert" className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error">
            {resolveApiErrorMessage(tErrors, startError)}
          </p>
        )}

        {snapshot !== null && (
          <>
            {snapshot.authorizationUrl !== undefined && snapshot.state === 'waiting_for_browser' && (
              <div className="space-y-2">
                <a
                  href={snapshot.authorizationUrl}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface-raised px-3 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-surface-hover"
                >
                  {t('accounts:oauthAttempt.openGoogle')}
                  <ExternalLink aria-hidden="true" className="size-3.5" />
                </a>
                {expiresLabel !== null && <p className="text-[11px] text-ink-faint">{expiresLabel}</p>}
              </div>
            )}

            {snapshot.state === 'awaiting_channel_selection' && (
              <div className="space-y-2">
                <p className="text-sm text-ink">{t('accounts:oauthAttempt.chooseChannel')}</p>
                <ul className="space-y-1.5">
                  {(snapshot.channels ?? []).map((channel) => (
                    <li key={channel.channelId}>
                      <button
                        type="button"
                        disabled={selectChannelMutation.isPending}
                        onClick={() => handleSelectChannel(channel.channelId)}
                        className="flex w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 py-2 text-left text-xs text-ink transition-colors hover:bg-surface-hover disabled:opacity-60"
                      >
                        {channel.thumbnailUrl !== undefined && channel.thumbnailUrl !== '' && (
                          <img
                            src={channel.thumbnailUrl}
                            alt=""
                            aria-hidden="true"
                            className="size-6 shrink-0 rounded-full object-cover"
                          />
                        )}
                        <span className="truncate">{channel.title}</span>
                      </button>
                    </li>
                  ))}
                </ul>
                {selectChannelMutation.isPending && (
                  <p className="flex items-center gap-1.5 text-xs text-ink-muted">
                    <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
                    {t('accounts:oauthAttempt.channelSelecting')}
                  </p>
                )}
                {selectChannelMutation.error !== null && (
                  <p role="alert" className="text-xs text-status-error">
                    {resolveApiErrorMessage(tErrors, selectChannelMutation.error)}
                  </p>
                )}
              </div>
            )}

            <p aria-live="polite" className="flex items-center gap-2 text-sm text-ink-muted">
              {(snapshot.state === 'waiting_for_browser' || snapshot.state === 'processing_callback') && (
                <Loader2 aria-hidden="true" className="size-4 animate-spin" />
              )}
              {t(oauthAttemptStateKey(snapshot.state))}
            </p>

            {snapshot.state === 'authorized' && (
              <p className="text-sm text-status-live">{t('accounts:oauthAttempt.successNote')}</p>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}
