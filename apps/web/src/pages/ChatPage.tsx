import { Filter, Settings2 } from 'lucide-react';
import { useEffect, useReducer, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { DEFAULT_OPERATOR_CHAT_PREFERENCES, type OperatorChatPreferences } from '@/api/operator-chat-schemas';
import { ActivityRow } from '@/components/chat/ActivityRow';
import { ChatFilterBar } from '@/components/chat/ChatFilterBar';
import { ChatSettingsPanel } from '@/components/chat/ChatSettingsPanel';
import { MessageRow } from '@/components/chat/MessageRow';
import { ModerationRow } from '@/components/chat/ModerationRow';
import { OutboundChatComposer } from '@/components/chat/OutboundChatComposer';
import { AppShell } from '@/components/layout/AppShell';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { Panel, PanelBody } from '@/components/ui/Panel';
import { useAccountsQuery } from '@/hooks/use-accounts';
import {
  useAddOperatorChatBotUserMutation,
  useAddOperatorChatHiddenUserMutation,
  useOperatorChatAccountVisibilityQuery,
  useOperatorChatBotUsersQuery,
  useOperatorChatHiddenUsersQuery,
  useOperatorChatPreferencesQuery,
  useOperatorChatStatusQuery,
  useSetOperatorChatAccountVisibilityMutation,
  useSetOperatorChatPreferencesMutation,
} from '@/hooks/use-operator-chat';
import { useOperatorChatStream } from '@/hooks/use-operator-chat-stream';
import { autoscrollReducer, initialAutoscrollState, isNearBottom } from '@/models/autoscroll';
import { replyTargetFor, type ReplyTarget } from '@/models/outbound-chat';
import { isCommandMessage } from '@/models/operator-chat-presentation';

/**
 * Stage 9 unified operator chat: the real, merged, live working view of
 * Twitch chat across every connected account - distinct from the
 * Engagement page's connector diagnostics (see pages/EngagementPage.tsx's
 * own doc comment). Consumes the operator-chat projection API, never the
 * raw Engagement Event Bus stream.
 */
export function ChatPage() {
  const { t } = useTranslation(['pages', 'chat']);

  const accountsQuery = useAccountsQuery();
  const twitchAccounts = (accountsQuery.data ?? []).filter((account) => account.providerId === 'twitch');

  const status = useOperatorChatStatusQuery();
  const stream = useOperatorChatStream();

  const prefsQuery = useOperatorChatPreferencesQuery();
  const setPrefsMutation = useSetOperatorChatPreferencesMutation();
  const [previewPrefs, setPreviewPrefs] = useState<OperatorChatPreferences | null>(null);
  const preferences = previewPrefs ?? prefsQuery.data ?? DEFAULT_OPERATOR_CHAT_PREFERENCES;

  const visibilityQuery = useOperatorChatAccountVisibilityQuery();
  const setVisibilityMutation = useSetOperatorChatAccountVisibilityMutation();
  const visibilityOverrides = new Map(
    (visibilityQuery.data ?? []).map((entry) => [entry.accountId, entry.visible]),
  );
  const isAccountVisible = (accountId: string) => visibilityOverrides.get(accountId) ?? true;

  const [hideBotMessages, setHideBotMessages] = useState(false);
  const botUsersQuery = useOperatorChatBotUsersQuery();
  const botProviderUserIds = new Set((botUsersQuery.data ?? []).map((u) => u.providerUserId));
  const hiddenUsersQuery = useOperatorChatHiddenUsersQuery();
  const hiddenProviderUserIds = new Set((hiddenUsersQuery.data ?? []).map((u) => u.providerUserId));

  const hideUserMutation = useAddOperatorChatHiddenUserMutation();
  const markBotMutation = useAddOperatorChatBotUserMutation();

  const [settingsOpen, setSettingsOpen] = useState(false);
  const [filtersOpen, setFiltersOpen] = useState(false);

  // Stage 11A reply state - component state only, never persisted (no
  // browser storage - see the stage's own "do not persist reply state"
  // requirement). Cleared only after a confirmed successful send;
  // preserved across validation/drop/rate-limit failures so the operator
  // can edit and retry.
  const [replyTarget, setReplyTarget] = useState<ReplyTarget | null>(null);

  const visibleItems = stream.items.filter((item) => {
    if (!isAccountVisible(item.connectedAccountId)) return false;
    if (item.kind === 'activity' && !preferences.showActivityEvents) return false;
    if (item.kind === 'message') {
      if (item.lifecycle.deleted && !preferences.showDeletedMessages) return false;
      const providerUserId = item.user?.providerUserId;
      if (providerUserId !== undefined && hiddenProviderUserIds.has(providerUserId)) return false;
      if (hideBotMessages && providerUserId !== undefined && botProviderUserIds.has(providerUserId)) {
        return false;
      }
      if (preferences.hideCommandMessages && isCommandMessage(item.message?.plainText ?? '')) {
        return false;
      }
    }
    return true;
  });

  // Autoscroll - see models/autoscroll.ts for the pure state machine this
  // wires to the scroll container's real DOM measurements.
  const scrollRef = useRef<HTMLUListElement>(null);
  const [autoscroll, dispatchAutoscroll] = useReducer(autoscrollReducer, initialAutoscrollState);
  const previousCountRef = useRef(visibleItems.length);

  useEffect(() => {
    const delta = visibleItems.length - previousCountRef.current;
    previousCountRef.current = visibleItems.length;
    if (delta > 0) {
      dispatchAutoscroll({ type: 'items-appended', count: delta });
    }
    // Only the count drives this - re-running on every item mutation would
    // treat lifecycle updates as new arrivals.
  }, [visibleItems.length]);

  useEffect(() => {
    if (autoscroll.following && scrollRef.current !== null) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [visibleItems.length, autoscroll.following]);

  function handleScroll() {
    const el = scrollRef.current;
    if (el === null) return;
    dispatchAutoscroll({
      type: 'scrolled',
      nearBottom: isNearBottom(el.scrollTop, el.scrollHeight, el.clientHeight),
    });
  }

  function jumpToLatest() {
    dispatchAutoscroll({ type: 'jump-to-latest' });
    if (scrollRef.current !== null) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }

  function accountLabelFor(accountId: string): string | null {
    if (twitchAccounts.length <= 1) return null;
    const account = twitchAccounts.find((a) => a.id === accountId);
    if (account === undefined) return null;
    return account.displayName !== '' ? account.displayName : account.login;
  }

  return (
    <AppShell
      title={t('pages:chat.title')}
      description={t('pages:chat.description')}
      actions={
        <>
          <Button size="sm" icon={<Filter className="size-4" />} onClick={() => setFiltersOpen(true)}>
            {t('chat:filters.title')}
          </Button>
          <Button size="sm" icon={<Settings2 className="size-4" />} onClick={() => setSettingsOpen(true)}>
            {t('chat:settings.title')}
          </Button>
        </>
      }
    >
      <div className="mx-auto flex max-w-3xl flex-col gap-3">
        <Panel>
          <PanelBody className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
            <ChatStreamStatusChip status={stream.status} />
            {status.data !== undefined && (
              <span className="text-ink-faint">
                {t('chat:status.retainedCount', { count: status.data.retainedCount })}
              </span>
            )}
            {twitchAccounts.length > 0 && (
              <span className="text-ink-faint">
                {t('chat:status.accountsConnected', { count: twitchAccounts.length })}
              </span>
            )}
            {stream.gapDetected && (
              <span className="w-full text-status-starting">{t('chat:status.gapWarning')}</span>
            )}
            {status.data?.busGap === true && (
              <span className="w-full text-status-starting">{t('chat:status.busGapWarning')}</span>
            )}
          </PanelBody>
        </Panel>

        <Panel className="relative flex flex-col overflow-hidden">
          <ul
            ref={scrollRef}
            onScroll={handleScroll}
            role="log"
            aria-label={t('chat:a11y.timeline')}
            aria-live="off"
            className="max-h-[70vh] min-h-[50vh] space-y-1 overflow-y-auto p-3"
          >
            {visibleItems.length === 0 ? (
              <p className="text-xs text-ink-faint">{t('chat:empty')}</p>
            ) : (
              visibleItems.map((item) => {
                if (item.kind === 'message') {
                  const providerUserId = item.user?.providerUserId;
                  const target = replyTargetFor(item);
                  return (
                    <MessageRow
                      key={item.id}
                      item={item}
                      preferences={preferences}
                      accountLabel={accountLabelFor(item.connectedAccountId)}
                      onHideUser={
                        providerUserId === undefined
                          ? undefined
                          : () =>
                              hideUserMutation.mutate({
                                providerId: item.providerId,
                                connectedAccountId: item.connectedAccountId,
                                providerUserId,
                              })
                      }
                      onMarkBot={
                        providerUserId === undefined
                          ? undefined
                          : () =>
                              markBotMutation.mutate({
                                providerId: item.providerId,
                                connectedAccountId: item.connectedAccountId,
                                providerUserId,
                              })
                      }
                      onReply={target === null ? undefined : () => setReplyTarget(target)}
                    />
                  );
                }
                if (item.kind === 'activity') {
                  return (
                    <ActivityRow
                      key={item.id}
                      item={item}
                      preferences={preferences}
                      accountLabel={accountLabelFor(item.connectedAccountId)}
                    />
                  );
                }
                return <ModerationRow key={item.id} item={item} preferences={preferences} />;
              })
            )}
          </ul>

          {!autoscroll.following && (
            <div className="absolute bottom-3 left-1/2 -translate-x-1/2" aria-live="polite">
              <Button variant="primary" size="sm" onClick={jumpToLatest}>
                {autoscroll.unseenCount > 0
                  ? `${t('chat:autoscroll.jumpToLatest')} · ${t('chat:autoscroll.unread', { count: autoscroll.unseenCount })}`
                  : t('chat:autoscroll.jumpToLatest')}
              </Button>
            </div>
          )}
        </Panel>

        <OutboundChatComposer
          twitchAccounts={twitchAccounts}
          replyTarget={replyTarget}
          onCancelReply={() => setReplyTarget(null)}
          onReplySent={() => setReplyTarget(null)}
        />
      </div>

      <ChatSettingsPanel
        open={settingsOpen}
        onClose={() => {
          setSettingsOpen(false);
          setPreviewPrefs(null);
        }}
        preferences={preferences}
        onPreview={setPreviewPrefs}
        onSave={(next) => {
          setPrefsMutation.mutate(next);
          setSettingsOpen(false);
          setPreviewPrefs(null);
        }}
        saving={setPrefsMutation.isPending}
      />

      <Modal
        open={filtersOpen}
        onClose={() => setFiltersOpen(false)}
        title={t('chat:filters.title')}
        size="sm"
      >
        <ChatFilterBar
          accounts={twitchAccounts}
          isAccountVisible={isAccountVisible}
          onSetAccountVisible={(accountId, visible) =>
            setVisibilityMutation.mutate({ accountId, visible })
          }
          hideBotMessages={hideBotMessages}
          onToggleHideBotMessages={setHideBotMessages}
          onResetAll={() => {
            setHideBotMessages(false);
            for (const account of twitchAccounts) {
              if (!isAccountVisible(account.id)) {
                setVisibilityMutation.mutate({ accountId: account.id, visible: true });
              }
            }
          }}
        />
      </Modal>
    </AppShell>
  );
}

function ChatStreamStatusChip({ status }: { status: 'connecting' | 'open' | 'error' | 'closed' }) {
  const { t } = useTranslation('chat');
  const labelKey =
    status === 'open'
      ? 'status.open'
      : status === 'connecting'
        ? 'status.connecting'
        : status === 'error'
          ? 'status.error'
          : 'status.closed';
  return <span className="font-medium text-ink">{t(labelKey)}</span>;
}
