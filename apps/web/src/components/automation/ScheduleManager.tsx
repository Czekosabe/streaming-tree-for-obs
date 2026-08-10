import type { TFunction } from 'i18next';
import { Plus, Trash2, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChatAutomationTarget, Schedule, ScheduleInput, SendNowResult } from '@/api/chat-automation-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { Panel } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextArea, TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useAccountsQuery } from '@/hooks/use-accounts';
import {
  useCreateScheduleMutation,
  useDeleteScheduleMutation,
  usePreviewMutation,
  useSchedulesQuery,
  useSendScheduleNowMutation,
  useUpdateScheduleMutation,
} from '@/hooks/use-chat-automation';
import { usePlatformsQuery } from '@/hooks/use-platforms';
import { ApiError } from '@/lib/api-client';
import {
  KNOWN_PLACEHOLDERS,
  MAX_MESSAGES_PER_SCHEDULE,
  codePointLength,
  isValidFirstDelaySeconds,
  isValidIntervalSeconds,
  isValidJitterSeconds,
  isValidMaximumSendsPerHour,
  isValidMinimumChatMessages,
  isValidScheduleName,
  isValidTemplate,
  unknownPlaceholderNames,
} from '@/models/chat-automation';

function emptyDraft(): ScheduleInput {
  return {
    name: '', enabled: true, intervalSeconds: 3600, firstDelaySeconds: 0, jitterSeconds: 0,
    onlyWhileIngestReceiving: false, minimumChatMessages: 0, maximumSendsPerHour: 10,
    targets: [], messages: [''],
  };
}

function draftFromSchedule(schedule: Schedule): ScheduleInput {
  return {
    name: schedule.name, enabled: schedule.enabled, intervalSeconds: schedule.intervalSeconds,
    firstDelaySeconds: schedule.firstDelaySeconds, jitterSeconds: schedule.jitterSeconds,
    onlyWhileIngestReceiving: schedule.onlyWhileIngestReceiving, minimumChatMessages: schedule.minimumChatMessages,
    maximumSendsPerHour: schedule.maximumSendsPerHour,
    targets: schedule.targets, messages: schedule.messages.map((m) => m.template),
  };
}

function errorMessage(t: TFunction<'automation'>, error: unknown): string {
  if (error instanceof ApiError && error.code !== null) {
    const key = `errors.${error.code}` as never;
    const translated = t(key, { defaultValue: '' });
    return translated === '' ? t('errors.generic') : translated;
  }
  return t('errors.generic');
}

export function ScheduleManager() {
  const { t } = useTranslation('automation');
  const schedulesQuery = useSchedulesQuery();
  const accountsQuery = useAccountsQuery();
  const platformsQuery = usePlatformsQuery();
  const createMutation = useCreateScheduleMutation();
  const updateMutation = useUpdateScheduleMutation();
  const deleteMutation = useDeleteScheduleMutation();
  const sendNowMutation = useSendScheduleNowMutation();

  const [editing, setEditing] = useState<{ id: string | null } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Schedule | null>(null);
  const [sendNowTarget, setSendNowTarget] = useState<Schedule | null>(null);

  const schedules = schedulesQuery.data ?? [];
  const twitchAccounts = (accountsQuery.data ?? []).filter((a) => a.providerId === 'twitch');
  const twitchPlatforms = (platformsQuery.data ?? []).filter((p) => p.providerId === 'twitch');

  const editingSchedule = editing?.id !== null && editing?.id !== undefined
    ? (schedules.find((s) => s.id === editing.id) ?? null)
    : null;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="max-w-2xl text-xs text-ink-faint">{t('common.restartNotice')}</p>
        <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setEditing({ id: null })}>
          {t('common.create')}
        </Button>
      </div>

      {schedules.length === 0 ? (
        <Panel><p className="text-sm text-ink-muted">{t('schedules.empty')}</p></Panel>
      ) : (
        <ul className="space-y-2">
          {schedules.map((schedule) => (
            <li key={schedule.id}>
              <Panel className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-ink">{schedule.name}</span>
                    <span className="rounded-full border border-line bg-surface-sunken px-2 py-0.5 text-[11px] text-ink-muted">
                      {t(`schedules.state.${schedule.state}`)}
                    </span>
                  </div>
                  <p className="mt-1 text-[11px] text-ink-faint">
                    {schedule.nextRunAt !== undefined
                      ? t('schedules.nextRun', { time: new Date(schedule.nextRunAt).toLocaleString() })
                      : t('schedules.nextRun', { time: t('schedules.never') })}
                    {' · '}
                    {schedule.lastSuccessAt !== undefined
                      ? t('schedules.lastSuccess', { time: new Date(schedule.lastSuccessAt).toLocaleString() })
                      : t('schedules.lastSuccess', { time: t('schedules.never') })}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <Button size="sm" onClick={() => setSendNowTarget(schedule)}>
                    {t('schedules.sendNowAction')}
                  </Button>
                  <Button size="sm" onClick={() => setEditing({ id: schedule.id })}>
                    {t('common.edit')}
                  </Button>
                  <IconButton
                    label={t('common.delete')}
                    icon={<Trash2 className="size-4" />}
                    variant="danger"
                    onClick={() => setDeleteTarget(schedule)}
                  />
                </div>
              </Panel>
            </li>
          ))}
        </ul>
      )}

      {editing !== null && (
        <ScheduleFormModal
          initial={editingSchedule !== null ? draftFromSchedule(editingSchedule) : emptyDraft()}
          title={editingSchedule !== null ? t('schedules.editTitle') : t('schedules.createTitle')}
          accounts={twitchAccounts.map((a) => ({ id: a.id, label: a.displayName || a.login }))}
          platforms={twitchPlatforms.map((p) => ({ id: p.id, label: p.displayName }))}
          busy={createMutation.isPending || updateMutation.isPending}
          errorText={
            createMutation.isError
              ? errorMessage(t, createMutation.error)
              : updateMutation.isError
                ? errorMessage(t, updateMutation.error)
                : null
          }
          onCancel={() => {
            setEditing(null);
            createMutation.reset();
            updateMutation.reset();
          }}
          onSubmit={(input) => {
            const onDone = () => {
              setEditing(null);
              createMutation.reset();
              updateMutation.reset();
            };
            if (editingSchedule !== null) {
              updateMutation.mutate({ id: editingSchedule.id, input }, { onSuccess: onDone });
            } else {
              createMutation.mutate(input, { onSuccess: onDone });
            }
          }}
        />
      )}

      {deleteTarget !== null && (
        <ConfirmDialog
          open
          title={t('schedules.deleteConfirmTitle')}
          message={t('schedules.deleteConfirmMessage', { name: deleteTarget.name })}
          confirmLabel={t('common.delete')}
          destructive
          busy={deleteMutation.isPending}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => {
            deleteMutation.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) });
          }}
        />
      )}

      {sendNowTarget !== null && (
        <SendNowDialog
          schedule={sendNowTarget}
          accounts={twitchAccounts.map((a) => ({ id: a.id, label: a.displayName || a.login }))}
          busy={sendNowMutation.isPending}
          results={sendNowMutation.data?.results ?? null}
          onClose={() => {
            setSendNowTarget(null);
            sendNowMutation.reset();
          }}
          onConfirm={() => {
            sendNowMutation.mutate({ id: sendNowTarget.id, accountIds: [] });
          }}
        />
      )}
    </div>
  );
}

type AccountOption = { id: string; label: string };

function SendNowDialog({
  schedule,
  accounts,
  busy,
  results,
  onClose,
  onConfirm,
}: {
  schedule: Schedule;
  accounts: AccountOption[];
  busy: boolean;
  results: SendNowResult[] | null;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation('automation');
  const labelFor = (id: string) => accounts.find((a) => a.id === id)?.label ?? id;

  return (
    <Modal
      open
      onClose={onClose}
      title={t('schedules.sendNowTitle', { name: schedule.name })}
      dismissible={!busy}
      footer={
        results === null ? (
          <>
            <Button onClick={onClose} disabled={busy}>{t('actions.cancel', { ns: 'common' })}</Button>
            <Button variant="primary" onClick={onConfirm} disabled={busy}>
              {t('schedules.sendNowConfirm')}
            </Button>
          </>
        ) : (
          <Button variant="primary" onClick={onClose}>{t('actions.close', { ns: 'common' })}</Button>
        )
      }
    >
      {results === null ? (
        <>
          <p className="text-sm text-ink-muted">{t('schedules.sendNowMessage')}</p>
          <ul className="mt-3 space-y-1 text-xs text-ink-muted">
            {schedule.targets.map((target) => (
              <li key={target.accountId}>{labelFor(target.accountId)}</li>
            ))}
          </ul>
        </>
      ) : (
        <ul className="space-y-1 text-sm">
          {results.map((result) => (
            <li key={result.accountId} className={result.sent ? 'text-status-live' : 'text-status-error'}>
              {result.sent
                ? t('schedules.sendNowResultSent', { account: labelFor(result.accountId) })
                : t('schedules.sendNowResultSkipped', {
                    account: labelFor(result.accountId),
                    reason: result.skipReason ?? '',
                  })}
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}

function ScheduleFormModal({
  initial,
  title,
  accounts,
  platforms,
  busy,
  errorText,
  onCancel,
  onSubmit,
}: {
  initial: ScheduleInput;
  title: string;
  accounts: AccountOption[];
  platforms: AccountOption[];
  busy: boolean;
  errorText: string | null;
  onCancel: () => void;
  onSubmit: (input: ScheduleInput) => void;
}) {
  const { t } = useTranslation('automation');
  const [draft, setDraft] = useState<ScheduleInput>(initial);
  const previewMutation = usePreviewMutation();

  const nameValid = isValidScheduleName(draft.name);
  const intervalValid = isValidIntervalSeconds(draft.intervalSeconds);
  const firstDelayValid = isValidFirstDelaySeconds(draft.firstDelaySeconds);
  const jitterValid = isValidJitterSeconds(draft.jitterSeconds);
  const minChatValid = isValidMinimumChatMessages(draft.minimumChatMessages);
  const maxPerHourValid = isValidMaximumSendsPerHour(draft.maximumSendsPerHour);
  const targetsValid = draft.targets.length > 0 && draft.targets.every((t2) => t2.accountId !== '');
  const messagesValid = draft.messages.length > 0 && draft.messages.every((m) => isValidTemplate(m));
  const formValid =
    nameValid && intervalValid && firstDelayValid && jitterValid && minChatValid && maxPerHourValid &&
    targetsValid && messagesValid;

  function updateTarget(index: number, patch: Partial<ChatAutomationTarget>) {
    setDraft((d) => ({
      ...d,
      targets: d.targets.map((t2, i) => (i === index ? { ...t2, ...patch } : t2)),
    }));
  }

  return (
    <Modal open onClose={onCancel} title={title} dismissible={!busy} size="md" footer={
      <>
        <Button onClick={onCancel} disabled={busy}>{t('actions.cancel', { ns: 'common' })}</Button>
        <Button variant="primary" disabled={busy || !formValid} onClick={() => onSubmit(draft)}>
          {t('common.save')}
        </Button>
      </>
    }>
      <div className="space-y-4">
        {errorText !== null && <p role="alert" className="text-sm text-status-error">{errorText}</p>}

        <FormField label={t('schedules.fields.name')} hint={t('schedules.fields.nameHint')}
          error={!nameValid && draft.name !== '' ? t('schedules.fields.nameHint') : undefined}>
          {({ inputId }) => (
            <TextInput id={inputId} value={draft.name} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} />
          )}
        </FormField>

        <ToggleSwitch label={t('schedules.fields.enabled')} description={t('schedules.fields.enabledHint')}
          checked={draft.enabled} onCheckedChange={(v) => setDraft((d) => ({ ...d, enabled: v }))} />

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <FormField label={t('schedules.fields.interval')} hint={t('schedules.fields.intervalHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.intervalSeconds}
                onChange={(e) => setDraft((d) => ({ ...d, intervalSeconds: Number(e.target.value) }))} />
            )}
          </FormField>
          <FormField label={t('schedules.fields.firstDelay')} hint={t('schedules.fields.firstDelayHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.firstDelaySeconds}
                onChange={(e) => setDraft((d) => ({ ...d, firstDelaySeconds: Number(e.target.value) }))} />
            )}
          </FormField>
          <FormField label={t('schedules.fields.jitter')} hint={t('schedules.fields.jitterHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.jitterSeconds}
                onChange={(e) => setDraft((d) => ({ ...d, jitterSeconds: Number(e.target.value) }))} />
            )}
          </FormField>
        </div>

        <ToggleSwitch label={t('schedules.fields.onlyWhileReceiving')} description={t('schedules.fields.onlyWhileReceivingHint')}
          checked={draft.onlyWhileIngestReceiving}
          onCheckedChange={(v) => setDraft((d) => ({ ...d, onlyWhileIngestReceiving: v }))} />

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FormField label={t('schedules.fields.minimumChatMessages')} hint={t('schedules.fields.minimumChatMessagesHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.minimumChatMessages}
                onChange={(e) => setDraft((d) => ({ ...d, minimumChatMessages: Number(e.target.value) }))} />
            )}
          </FormField>
          <FormField label={t('schedules.fields.maximumSendsPerHour')} hint={t('schedules.fields.maximumSendsPerHourHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.maximumSendsPerHour}
                onChange={(e) => setDraft((d) => ({ ...d, maximumSendsPerHour: Number(e.target.value) }))} />
            )}
          </FormField>
        </div>

        <div>
          <p className="text-xs font-medium text-ink-muted">{t('common.targets')}</p>
          <div className="mt-1 space-y-2">
            {draft.targets.map((target, index) => (
              <div key={index} className="flex items-center gap-2">
                <SelectInput
                  aria-label={t('common.account')}
                  options={[{ value: '', label: t('common.account') }, ...accounts.map((a) => ({ value: a.id, label: a.label }))]}
                  value={target.accountId}
                  onChange={(e) => updateTarget(index, { accountId: e.target.value })}
                />
                <SelectInput
                  aria-label={t('common.platformContext')}
                  options={[{ value: '', label: t('common.platformContextNone') }, ...platforms.map((p) => ({ value: p.id, label: p.label }))]}
                  value={target.platformId ?? ''}
                  onChange={(e) => updateTarget(index, { platformId: e.target.value || undefined })}
                />
                <IconButton label={t('common.removeTarget')} icon={<X className="size-4" />}
                  onClick={() => setDraft((d) => ({ ...d, targets: d.targets.filter((_, i) => i !== index) }))} />
              </div>
            ))}
          </div>
          <Button size="sm" className="mt-2" icon={<Plus className="size-4" />}
            onClick={() => setDraft((d) => ({ ...d, targets: [...d.targets, { accountId: '' }] }))}>
            {t('common.addTarget')}
          </Button>
          {accounts.length === 0 && <p className="mt-1 text-[11px] text-status-error">{t('common.noAccounts')}</p>}
        </div>

        <div>
          <p className="text-xs font-medium text-ink-muted">{t('schedules.fields.messages')}</p>
          <p className="text-[11px] text-ink-faint">{t('schedules.fields.messagesHint')}</p>
          <div className="mt-1 space-y-3">
            {draft.messages.map((message, index) => {
              const unknown = unknownPlaceholderNames(message);
              return (
                <div key={index} className="space-y-1">
                  <TextArea
                    value={message}
                    onChange={(e) =>
                      setDraft((d) => ({
                        ...d,
                        messages: d.messages.map((m, i) => (i === index ? e.target.value : m)),
                      }))
                    }
                  />
                  <div className="flex flex-wrap items-center justify-between gap-2 text-[11px] text-ink-faint">
                    <div className="flex flex-wrap gap-1">
                      {KNOWN_PLACEHOLDERS.map((name) => (
                        <button
                          key={name}
                          type="button"
                          className="rounded border border-line px-1.5 py-0.5 hover:bg-surface-hover"
                          onClick={() =>
                            setDraft((d) => ({
                              ...d,
                              messages: d.messages.map((m, i) => (i === index ? m + `{${name}}` : m)),
                            }))
                          }
                        >
                          {`{${name}}`}
                        </button>
                      ))}
                      {draft.messages.length > 1 && (
                        <IconButton label={t('schedules.fields.removeMessage')} icon={<Trash2 className="size-3.5" />}
                          onClick={() => setDraft((d) => ({ ...d, messages: d.messages.filter((_, i) => i !== index) }))} />
                      )}
                    </div>
                    <span>{t('common.renderedCount', { count: codePointLength(message) })}</span>
                  </div>
                  {unknown.length > 0 && (
                    <p className="text-[11px] text-status-error">
                      {t('common.unresolvedWarning', { names: unknown.join(', ') })}
                    </p>
                  )}
                </div>
              );
            })}
          </div>
          {draft.messages.length < MAX_MESSAGES_PER_SCHEDULE && (
            <Button size="sm" className="mt-2" icon={<Plus className="size-4" />}
              onClick={() => setDraft((d) => ({ ...d, messages: [...d.messages, ''] }))}>
              {t('schedules.fields.addMessage')}
            </Button>
          )}
        </div>

        {draft.targets[0]?.accountId !== undefined && draft.targets[0].accountId !== '' && draft.messages[0] !== undefined && (
          <PreviewBlock
            template={draft.messages[0]}
            accountId={draft.targets[0].accountId}
            platformId={draft.targets[0].platformId}
            mutation={previewMutation}
          />
        )}
      </div>
    </Modal>
  );
}

function PreviewBlock({
  template,
  accountId,
  platformId,
  mutation,
}: {
  template: string;
  accountId: string;
  platformId: string | undefined;
  mutation: ReturnType<typeof usePreviewMutation>;
}) {
  const { t } = useTranslation('automation');
  useEffect(() => {
    if (template.trim() === '') return;
    mutation.mutate(platformId !== undefined ? { template, accountId, platformId } : { template, accountId });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [template, accountId, platformId]);

  if (mutation.data === undefined) return null;

  return (
    <div className="rounded-lg border border-line bg-surface-sunken p-3">
      <p className="text-xs font-medium text-ink-muted">{t('common.preview')}</p>
      <p className="mt-1 text-sm text-ink">{mutation.data.renderedText}</p>
      {(mutation.data.unresolvedPlaceholders?.length ?? 0) > 0 && (
        <p className="mt-1 text-[11px] text-status-error">
          {t('common.unresolvedWarning', { names: mutation.data.unresolvedPlaceholders?.join(', ') })}
        </p>
      )}
      {!mutation.data.validForProvider && (mutation.data.unresolvedPlaceholders?.length ?? 0) === 0 && (
        <p className="mt-1 text-[11px] text-status-error">{t('common.tooLongWarning')}</p>
      )}
    </div>
  );
}
