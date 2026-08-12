import type { TFunction } from 'i18next';
import { Plus, Trash2, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChatAutomationTarget, Command, CommandInput, CommandRole } from '@/api/chat-automation-schemas';
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
  useCommandsQuery,
  useCreateCommandMutation,
  useDeleteCommandMutation,
  usePreviewMutation,
  useUpdateCommandMutation,
} from '@/hooks/use-chat-automation';
import { usePlatformsQuery } from '@/hooks/use-platforms';
import { ApiError } from '@/lib/api-client';
import {
  COMMAND_ROLES,
  KNOWN_PLACEHOLDERS,
  codePointLength,
  isValidCommandName,
  isValidCooldownSeconds,
  isValidTemplate,
  normalizeCommandName,
  unknownPlaceholderNames,
} from '@/models/chat-automation';

function emptyDraft(): CommandInput {
  return {
    name: '', enabled: true, responseTemplate: '', requiredRole: 'everyone',
    globalCooldownSeconds: 0, userCooldownSeconds: 0, aliases: [], targets: [],
  };
}

function draftFromCommand(command: Command): CommandInput {
  return {
    name: command.name, enabled: command.enabled, responseTemplate: command.responseTemplate,
    requiredRole: command.requiredRole, globalCooldownSeconds: command.globalCooldownSeconds,
    userCooldownSeconds: command.userCooldownSeconds, aliases: command.aliases, targets: command.targets,
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

type AccountOption = { id: string; label: string };

export function CommandManager() {
  const { t } = useTranslation('automation');
  const commandsQuery = useCommandsQuery();
  const accountsQuery = useAccountsQuery();
  const platformsQuery = usePlatformsQuery();
  const createMutation = useCreateCommandMutation();
  const updateMutation = useUpdateCommandMutation();
  const deleteMutation = useDeleteCommandMutation();

  const [editing, setEditing] = useState<{ id: string | null } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Command | null>(null);

  const commands = commandsQuery.data ?? [];
  const automationAccounts = (accountsQuery.data ?? []).filter(
    (a) => a.providerId === 'twitch' || a.providerId === 'youtube',
  );
  const automationPlatforms = (platformsQuery.data ?? []).filter(
    (p) => p.providerId === 'twitch' || p.providerId === 'youtube',
  );
  const editingCommand = editing?.id !== null && editing?.id !== undefined
    ? (commands.find((c) => c.id === editing.id) ?? null)
    : null;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="max-w-2xl text-xs text-ink-faint">{t('commands.prefixNotice')}</p>
        <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setEditing({ id: null })}>
          {t('common.create')}
        </Button>
      </div>

      {commands.length === 0 ? (
        <Panel><p className="text-sm text-ink-muted">{t('commands.empty')}</p></Panel>
      ) : (
        <ul className="space-y-2">
          {commands.map((command) => (
            <li key={command.id}>
              <Panel className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-medium text-ink">{t('commands.fields.namePrefixPreview', { name: command.name })}</span>
                    {!command.enabled && (
                      <span className="rounded-full border border-line bg-surface-sunken px-2 py-0.5 text-[11px] text-ink-muted">
                        {t('common.disable')}
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-[11px] text-ink-faint">
                    {t('commands.matchCount', { count: command.matchCount })} · {t('commands.responseCount', { count: command.responseCount })}
                    {command.aliases.length > 0 ? ` · ${command.aliases.map((a) => `!${a}`).join(', ')}` : ''}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <Button size="sm" onClick={() => setEditing({ id: command.id })}>{t('common.edit')}</Button>
                  <IconButton label={t('common.delete')} icon={<Trash2 className="size-4" />} variant="danger"
                    onClick={() => setDeleteTarget(command)} />
                </div>
              </Panel>
            </li>
          ))}
        </ul>
      )}

      {editing !== null && (
        <CommandFormModal
          initial={editingCommand !== null ? draftFromCommand(editingCommand) : emptyDraft()}
          title={editingCommand !== null ? t('commands.editTitle') : t('commands.createTitle')}
          accounts={automationAccounts.map((a) => ({ id: a.id, label: a.displayName || a.login }))}
          platforms={automationPlatforms.map((p) => ({ id: p.id, label: p.displayName }))}
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
            if (editingCommand !== null) {
              updateMutation.mutate({ id: editingCommand.id, input }, { onSuccess: onDone });
            } else {
              createMutation.mutate(input, { onSuccess: onDone });
            }
          }}
        />
      )}

      {deleteTarget !== null && (
        <ConfirmDialog
          open
          title={t('commands.deleteConfirmTitle')}
          message={t('commands.deleteConfirmMessage', { name: deleteTarget.name })}
          confirmLabel={t('common.delete')}
          destructive
          busy={deleteMutation.isPending}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => deleteMutation.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) })}
        />
      )}
    </div>
  );
}

function CommandFormModal({
  initial,
  title,
  accounts,
  platforms,
  busy,
  errorText,
  onCancel,
  onSubmit,
}: {
  initial: CommandInput;
  title: string;
  accounts: AccountOption[];
  platforms: AccountOption[];
  busy: boolean;
  errorText: string | null;
  onCancel: () => void;
  onSubmit: (input: CommandInput) => void;
}) {
  const { t } = useTranslation('automation');
  const [draft, setDraft] = useState<CommandInput>(initial);
  const previewMutation = usePreviewMutation();

  const normalizedName = normalizeCommandName(draft.name);
  const nameValid = isValidCommandName(normalizedName);
  const responseValid = isValidTemplate(draft.responseTemplate);
  const cooldownsValid = isValidCooldownSeconds(draft.globalCooldownSeconds, draft.userCooldownSeconds);
  const targetsValid = draft.targets.length > 0 && draft.targets.every((t2) => t2.accountId !== '');
  const aliasesValid = draft.aliases.every((a) => isValidCommandName(normalizeCommandName(a)));
  const formValid = nameValid && responseValid && cooldownsValid && targetsValid && aliasesValid;
  const unknown = unknownPlaceholderNames(draft.responseTemplate);

  function updateTarget(index: number, patch: Partial<ChatAutomationTarget>) {
    setDraft((d) => ({ ...d, targets: d.targets.map((t2, i) => (i === index ? { ...t2, ...patch } : t2)) }));
  }

  return (
    <Modal open onClose={onCancel} title={title} dismissible={!busy} size="md" footer={
      <>
        <Button onClick={onCancel} disabled={busy}>{t('actions.cancel', { ns: 'common' })}</Button>
        <Button variant="primary" disabled={busy || !formValid}
          onClick={() => onSubmit({ ...draft, name: normalizedName, aliases: draft.aliases.map(normalizeCommandName) })}>
          {t('common.save')}
        </Button>
      </>
    }>
      <div className="space-y-4">
        {errorText !== null && <p role="alert" className="text-sm text-status-error">{errorText}</p>}

        <FormField label={t('commands.fields.name')} hint={t('commands.fields.nameHint')}
          error={draft.name !== '' && !nameValid ? t('commands.fields.nameHint') : undefined}>
          {({ inputId }) => (
            <TextInput id={inputId} value={draft.name} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} />
          )}
        </FormField>

        <ToggleSwitch label={t('commands.fields.enabled')} checked={draft.enabled}
          onCheckedChange={(v) => setDraft((d) => ({ ...d, enabled: v }))} />

        <div>
          <p className="text-xs font-medium text-ink-muted">{t('commands.fields.aliases')}</p>
          <div className="mt-1 space-y-2">
            {draft.aliases.map((alias, index) => (
              <div key={index} className="flex items-center gap-2">
                <TextInput value={alias} onChange={(e) =>
                  setDraft((d) => ({ ...d, aliases: d.aliases.map((a, i) => (i === index ? e.target.value : a)) }))} />
                <IconButton label={t('commands.fields.removeAlias')} icon={<X className="size-4" />}
                  onClick={() => setDraft((d) => ({ ...d, aliases: d.aliases.filter((_, i) => i !== index) }))} />
              </div>
            ))}
          </div>
          <Button size="sm" className="mt-2" icon={<Plus className="size-4" />}
            onClick={() => setDraft((d) => ({ ...d, aliases: [...d.aliases, ''] }))}>
            {t('commands.fields.addAlias')}
          </Button>
        </div>

        <FormField label={t('commands.fields.requiredRole')}>
          {({ inputId }) => (
            <SelectInput id={inputId}
              options={COMMAND_ROLES.map((role) => ({ value: role, label: t(`commands.role.${role}`) }))}
              value={draft.requiredRole}
              onChange={(e) => setDraft((d) => ({ ...d, requiredRole: e.target.value as CommandRole }))} />
          )}
        </FormField>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FormField label={t('commands.fields.globalCooldown')} hint={t('commands.fields.globalCooldownHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.globalCooldownSeconds}
                onChange={(e) => setDraft((d) => ({ ...d, globalCooldownSeconds: Number(e.target.value) }))} />
            )}
          </FormField>
          <FormField label={t('commands.fields.userCooldown')} hint={t('commands.fields.userCooldownHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.userCooldownSeconds}
                onChange={(e) => setDraft((d) => ({ ...d, userCooldownSeconds: Number(e.target.value) }))} />
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

        <FormField label={t('commands.fields.responseTemplate')} counter={t('common.renderedCount', { count: codePointLength(draft.responseTemplate) })}>
          {({ inputId }) => (
            <TextArea id={inputId} value={draft.responseTemplate}
              onChange={(e) => setDraft((d) => ({ ...d, responseTemplate: e.target.value }))} />
          )}
        </FormField>
        <div className="flex flex-wrap gap-1">
          {KNOWN_PLACEHOLDERS.map((name) => (
            <button key={name} type="button" className="rounded border border-line px-1.5 py-0.5 text-[11px] hover:bg-surface-hover"
              onClick={() => setDraft((d) => ({ ...d, responseTemplate: d.responseTemplate + `{${name}}` }))}>
              {`{${name}}`}
            </button>
          ))}
        </div>
        {unknown.length > 0 && (
          <p className="text-[11px] text-status-error">{t('common.unresolvedWarning', { names: unknown.join(', ') })}</p>
        )}

        {draft.targets[0]?.accountId !== undefined && draft.targets[0].accountId !== '' && (
          <CommandPreviewBlock
            template={draft.responseTemplate}
            accountId={draft.targets[0].accountId}
            platformId={draft.targets[0].platformId}
            mutation={previewMutation}
          />
        )}
      </div>
    </Modal>
  );
}

function CommandPreviewBlock({
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
    </div>
  );
}
