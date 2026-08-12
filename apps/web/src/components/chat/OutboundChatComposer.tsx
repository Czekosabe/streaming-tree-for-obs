import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConnectedAccount } from '@/api/account-schemas';
import { ApiError } from '@/lib/api-client';
import { Button } from '@/components/ui/Button';
import { Panel, PanelBody } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextArea } from '@/components/ui/TextInput';
import { useAccountEngagementQuery } from '@/hooks/use-engagement';
import {
  useAuthorizeOutboundChatMutation,
  useOutboundChatStatusQuery,
  useSendOutboundChatMessageMutation,
} from '@/hooks/use-outbound-chat';
import { codePointLength, isMessageSendable, MAX_MESSAGE_CODE_POINTS, type ReplyTarget } from '@/models/outbound-chat';

type OutboundChatComposerProps = {
  /** Every connected account whose provider supports outbound chat
   * (currently Twitch and YouTube) - never filtered to one provider,
   * see ChatPage.tsx's own account query. */
  accounts: ConnectedAccount[];
  replyTarget: ReplyTarget | null;
  onCancelReply: () => void;
  onReplySent: () => void;
};

/**
 * Stage 11A's manual chat composer: sends as the connected account through
 * the backend's own bounded dispatcher - never a separate bot identity, no
 * IRC, no optimistic local echo (the real EventSub echo produces the
 * timeline item once inbound engagement is enabled - see the stage's own
 * "self-message" design note). Stage 15A: the same composer, unchanged in
 * structure, now also sends through the YouTube outbound adapter for a
 * YouTube account - one shared dispatcher, two providers, never a second
 * send queue.
 */
export function OutboundChatComposer({
  accounts,
  replyTarget,
  onCancelReply,
  onReplySent,
}: OutboundChatComposerProps) {
  const { t } = useTranslation('chat');

  const [selectedAccountId, setSelectedAccountId] = useState<string | undefined>(accounts[0]?.id);
  const [message, setMessage] = useState('');

  // A reply always selects and locks the message's own source account.
  useEffect(() => {
    if (replyTarget !== null) {
      setSelectedAccountId(replyTarget.accountId);
    }
  }, [replyTarget]);

  useEffect(() => {
    if (selectedAccountId === undefined && accounts.length > 0) {
      setSelectedAccountId(accounts[0]?.id);
    }
  }, [selectedAccountId, accounts]);

  const status = useOutboundChatStatusQuery(selectedAccountId);
  const engagement = useAccountEngagementQuery(selectedAccountId ?? '');
  const authorize = useAuthorizeOutboundChatMutation();
  const send = useSendOutboundChatMessageMutation();

  const selectedAccount = accounts.find((a) => a.id === selectedAccountId);

  if (accounts.length === 0) {
    return null;
  }

  const codePoints = codePointLength(message);
  const overLimit = codePoints > MAX_MESSAGE_CODE_POINTS;
  const canSend = status.data?.canSendNow === true && isMessageSendable(message) && !send.isPending;

  const sendErrorCode = send.error instanceof ApiError ? send.error.code : null;
  const inboundConnected = engagement.data?.enabled === true && engagement.data.state === 'connected';

  function handleSend() {
    if (selectedAccountId === undefined || !canSend) return;
    send.mutate(
      {
        accountId: selectedAccountId,
        input: {
          message,
          ...(replyTarget !== null ? { replyParentMessageId: replyTarget.providerMessageId } : {}),
        },
      },
      {
        onSuccess: () => {
          setMessage('');
          onReplySent();
        },
        // Validation/drop/rate-limit errors intentionally preserve both the
        // typed message and any active reply target, so the operator can
        // edit and retry - see the stage's own reply-preservation
        // requirement.
      },
    );
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      if (!send.isPending) handleSend();
    }
  }

  return (
    <Panel data-testid="outbound-chat-composer">
      <PanelBody className="space-y-2">
        {accounts.length > 1 && (
          <SelectInput
            aria-label={t('compose.accountSelectorLabel')}
            value={selectedAccountId}
            disabled={replyTarget !== null}
            onChange={(e) => setSelectedAccountId(e.target.value)}
            options={accounts.map((a) => ({
              value: a.id,
              label: a.displayName !== '' ? a.displayName : a.login,
            }))}
          />
        )}

        {selectedAccount !== undefined && (
          <p className="text-[11px] text-ink-faint">
            {t('compose.sendAsLabel', {
              name: selectedAccount.displayName !== '' ? selectedAccount.displayName : selectedAccount.login,
            })}
          </p>
        )}

        {replyTarget !== null && (
          <div className="flex items-start justify-between gap-2 rounded-md border border-line/60 bg-surface-raised px-2 py-1.5 text-[11px]">
            <p className="text-ink-muted">
              {t('compose.replyingTo', { name: replyTarget.authorDisplayName || t('anonymous') })}
              <span className="ml-1 italic text-ink-faint">{replyTarget.preview}</span>
            </p>
            <button
              type="button"
              className="shrink-0 text-ink-faint hover:text-ink"
              onClick={onCancelReply}
              aria-label={t('compose.cancelReply')}
            >
              ×
            </button>
          </div>
        )}

        {selectedAccount?.providerId === 'twitch' && (
          <p className="text-[11px] text-status-starting" role="note">
            {t('compose.sharedChatWarning')}
          </p>
        )}

        {status.data?.capability === 'permission_required' && (
          <div className="space-y-1.5 rounded-md border border-status-starting/40 bg-status-starting/10 p-2">
            <p className="text-[11px] font-medium text-status-starting">{t('compose.permissionRequired')}</p>
            <Button
              size="sm"
              variant="primary"
              disabled={authorize.isPending || selectedAccountId === undefined}
              onClick={() => selectedAccountId !== undefined && authorize.mutate(selectedAccountId)}
            >
              {t('compose.authorizeAction')}
            </Button>
            {authorize.isSuccess && (
              <p className="text-[11px] text-ink-muted">
                {t('compose.authorizeStarted', { userCode: authorize.data.userCode ?? '' })}
              </p>
            )}
          </div>
        )}

        {status.data?.capability === 'unsupported' && (
          <p className="text-[11px] text-ink-faint">{t('compose.unsupported')}</p>
        )}

        {status.isError && (
          <p className="text-[11px] text-status-error">{t('compose.backendUnavailable')}</p>
        )}

        {status.data?.dispatcherState === 'rate_limited' && (
          <p className="text-[11px] text-status-starting">
            {status.data.retryAt !== undefined
              ? t('compose.rateLimitedUntil', { time: new Date(status.data.retryAt).toLocaleTimeString() })
              : t('compose.rateLimited')}
          </p>
        )}

        {!inboundConnected && status.data?.capability === 'ready' && (
          <p className="text-[11px] text-ink-faint">{t('compose.noLocalEchoExplanation')}</p>
        )}

        <TextArea
          rows={2}
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={t('compose.placeholder')}
          disabled={status.data?.capability !== 'ready'}
          aria-label={t('compose.messageLabel')}
          autoComplete="off"
        />

        <div className="flex items-center justify-between">
          <span className={overLimit ? 'text-[11px] text-status-error' : 'text-[11px] text-ink-faint'}>
            {t('compose.characterCount', { count: codePoints, max: MAX_MESSAGE_CODE_POINTS })}
          </span>
          <Button size="sm" variant="primary" disabled={!canSend} onClick={handleSend}>
            {send.isPending ? t('compose.sending') : t('compose.sendAction')}
          </Button>
        </div>

        {sendErrorCode === 'outbound_chat_message_dropped' && (
          <p className="text-[11px] text-status-error">{t('compose.dropped')}</p>
        )}
        {sendErrorCode === 'outbound_chat_delivery_unknown' && (
          <p className="text-[11px] text-status-starting">{t('compose.deliveryUnknown')}</p>
        )}
        {sendErrorCode === 'outbound_chat_rate_limited' && (
          <p className="text-[11px] text-status-starting">{t('compose.rateLimited')}</p>
        )}
        {sendErrorCode === 'outbound_chat_forbidden' && (
          <p className="text-[11px] text-status-error">{t('compose.forbidden')}</p>
        )}
        {sendErrorCode === 'outbound_chat_provider_failure' && (
          <p className="text-[11px] text-status-error">{t('compose.providerFailure')}</p>
        )}
        {sendErrorCode === 'outbound_chat_queue_full' && (
          <p className="text-[11px] text-status-error">{t('compose.queueFull')}</p>
        )}
        {sendErrorCode === 'outbound_chat_unavailable' && (
          <p className="text-[11px] text-status-starting">{t('compose.chatUnavailable')}</p>
        )}
        {sendErrorCode === 'outbound_chat_reply_unsupported' && (
          <p className="text-[11px] text-status-error">{t('compose.replyUnsupported')}</p>
        )}
      </PanelBody>
    </Panel>
  );
}
