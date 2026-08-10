import type { TFunction } from 'i18next';
import { Plus, Trash2, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { AlertEventTypeCapability, AlertRule, AlertRuleInput } from '@/api/alerts-schemas';
import { AlertRenderer } from '@/components/alerts/AlertRenderer';
import { Button, IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { Panel, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextArea, TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useAccountsQuery } from '@/hooks/use-accounts';
import { ApiError } from '@/lib/api-client';
import {
  useAlertEventTypesQuery,
  useAlertPreviewMutation,
  useAlertRulesQuery,
  useCreateAlertRuleMutation,
  useDeleteAlertRuleMutation,
  useTestAlertRuleMutation,
  useUpdateAlertRuleMutation,
} from '@/hooks/use-alerts';
import {
  ALERT_ANIMATIONS,
  ALERT_EVENT_TYPES,
  ALERT_ROLES,
  DEFAULT_GROUP_WINDOW_MS,
  codePointLength,
  extractPlaceholderNames,
  insertPlaceholder,
  isValidAlertName,
  isValidAlertTemplate,
  isValidAnimationDurationMs,
  isValidDurationMs,
  isValidGroupWindowMs,
  isValidPriority,
  isValidThresholdRange,
  unknownPlaceholderNames,
  unsupportedPlaceholderNames,
} from '@/models/alerts';

function errorMessage(t: TFunction<'alerts'>, error: unknown): string {
  if (error instanceof ApiError && error.code !== null) {
    const key = `errors.${error.code}` as never;
    const translated = t(key, { defaultValue: '' });
    return translated === '' ? t('errors.generic') : translated;
  }
  return t('errors.generic');
}

function emptyDraft(defaultEventType: AlertRuleInput['eventType']): AlertRuleInput {
  return {
    name: '', enabled: true, eventType: defaultEventType, priority: 50, durationMs: 5000,
    minimumQuantity: null, maximumQuantity: null, requiredRole: 'everyone',
    showPlatform: true, showUsername: true, showMessage: false, showQuantity: false,
    textTemplate: '', entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    providers: [], accounts: [],
    allowGrouping: false, groupWindowMs: DEFAULT_GROUP_WINDOW_MS,
    interruptMode: 'never', interruptible: true,
  };
}

function draftFromRule(rule: AlertRule): AlertRuleInput {
  return {
    name: rule.name, enabled: rule.enabled, eventType: rule.eventType, priority: rule.priority,
    durationMs: rule.durationMs, minimumQuantity: rule.minimumQuantity ?? null, maximumQuantity: rule.maximumQuantity ?? null,
    requiredRole: rule.requiredRole, showPlatform: rule.showPlatform, showUsername: rule.showUsername,
    showMessage: rule.showMessage, showQuantity: rule.showQuantity, textTemplate: rule.textTemplate,
    entryAnimation: rule.entryAnimation, exitAnimation: rule.exitAnimation, animationDurationMs: rule.animationDurationMs,
    providers: rule.providers, accounts: rule.accounts,
    allowGrouping: rule.allowGrouping, groupWindowMs: rule.groupWindowMs,
    interruptMode: rule.interruptMode, interruptible: rule.interruptible,
  };
}

export function RuleManager({ profileId }: { profileId: string }) {
  const { t } = useTranslation('alerts');
  const rulesQuery = useAlertRulesQuery(profileId);
  const eventTypesQuery = useAlertEventTypesQuery();
  const deleteMutation = useDeleteAlertRuleMutation(profileId);
  const testMutation = useTestAlertRuleMutation();

  const [editingId, setEditingId] = useState<string | null | 'new'>(null);
  const [deleteTarget, setDeleteTarget] = useState<AlertRule | null>(null);

  const rules = rulesQuery.data?.rules ?? [];
  const overlapWarnings = rulesQuery.data?.overlapWarnings ?? [];
  const eventTypes = eventTypesQuery.data ?? [];
  const editingRule = typeof editingId === 'string' && editingId !== 'new' ? (rules.find((r) => r.id === editingId) ?? null) : null;

  return (
    <Panel>
      <PanelHeader
        title={t('rules.title')}
        actions={
          <Button size="sm" icon={<Plus className="size-4" />} onClick={() => setEditingId('new')} disabled={eventTypes.length === 0}>
            {t('common.create')}
          </Button>
        }
      />
      <div className="p-4 sm:p-5">
        {rules.length === 0 ? (
          <p className="text-sm text-ink-muted">{t('rules.empty')}</p>
        ) : (
          <ul className="space-y-2">
            {rules.map((rule) => {
              const overlap = overlapWarnings.find((w) => w.ruleId === rule.id || w.otherRuleId === rule.id);
              const otherId = overlap === undefined ? null : overlap.ruleId === rule.id ? overlap.otherRuleId : overlap.ruleId;
              const otherName = otherId === null ? '' : (rules.find((r) => r.id === otherId)?.name ?? otherId);
              return (
                <li key={rule.id}>
                  <div className="rounded-lg border border-line p-3">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium text-ink">{rule.name}</span>
                          <span className="rounded-full border border-line bg-surface-sunken px-2 py-0.5 text-[11px] text-ink-muted">
                            {t(`rules.eventType.${rule.eventType}`)}
                          </span>
                          {!rule.enabled && (
                            <span className="rounded-full border border-line px-2 py-0.5 text-[11px] text-ink-faint">
                              {t('profiles.fields.enabled')}: off
                            </span>
                          )}
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button size="sm" onClick={() => testMutation.mutate({ id: rule.id })} disabled={testMutation.isPending}>
                          {t('rules.testAction')}
                        </Button>
                        <Button size="sm" onClick={() => setEditingId(rule.id)}>
                          {t('common.edit')}
                        </Button>
                        <IconButton
                          label={t('common.delete')}
                          icon={<Trash2 className="size-4" />}
                          variant="danger"
                          onClick={() => setDeleteTarget(rule)}
                        />
                      </div>
                    </div>
                    {overlap !== undefined && (
                      <p className="mt-2 text-[11px] text-status-error">
                        {t('rules.overlapWarning', { other: otherName })}
                      </p>
                    )}
                    {testMutation.isSuccess && testMutation.variables?.id === rule.id && (
                      <p role="status" className="mt-2 text-[11px] text-ink-muted">
                        {t('rules.testSentNotice')}
                      </p>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {editingId !== null && eventTypes.length > 0 && (
        <RuleFormModal
          profileId={profileId}
          initial={editingRule !== null ? draftFromRule(editingRule) : emptyDraft(eventTypes[0]!.eventType)}
          title={editingRule !== null ? t('rules.editTitle') : t('rules.createTitle')}
          eventTypes={eventTypes}
          onCancel={() => setEditingId(null)}
          editingRuleId={editingRule?.id ?? null}
          onSaved={() => setEditingId(null)}
        />
      )}

      {deleteTarget !== null && (
        <ConfirmDialog
          open
          title={t('rules.deleteConfirmTitle')}
          message={t('rules.deleteConfirmMessage', { name: deleteTarget.name })}
          confirmLabel={t('common.delete')}
          destructive
          busy={deleteMutation.isPending}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => deleteMutation.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) })}
        />
      )}
    </Panel>
  );
}

function RuleFormModal({
  profileId,
  initial,
  title,
  eventTypes,
  editingRuleId,
  onCancel,
  onSaved,
}: {
  profileId: string;
  initial: AlertRuleInput;
  title: string;
  eventTypes: AlertEventTypeCapability[];
  editingRuleId: string | null;
  onCancel: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation('alerts');
  const accountsQuery = useAccountsQuery();
  const createMutation = useCreateAlertRuleMutation(profileId);
  const updateMutation = useUpdateAlertRuleMutation(profileId);
  const previewMutation = useAlertPreviewMutation();
  const [draft, setDraft] = useState<AlertRuleInput>(initial);

  const twitchAccounts = (accountsQuery.data ?? []).filter((a) => a.providerId === 'twitch');
  const capability = eventTypes.find((e) => e.eventType === draft.eventType);

  const nameValid = isValidAlertName(draft.name);
  const priorityValid = isValidPriority(draft.priority);
  const durationValid = isValidDurationMs(draft.durationMs);
  const animationDurationValid = isValidAnimationDurationMs(draft.animationDurationMs);
  const thresholdValid = isValidThresholdRange(draft.minimumQuantity ?? null, draft.maximumQuantity ?? null);
  const templateValid = isValidAlertTemplate(draft.textTemplate);
  const unsupported = unsupportedPlaceholderNames(draft.textTemplate, capability);
  const unknown = unknownPlaceholderNames(draft.textTemplate);
  const groupWindowValid = isValidGroupWindowMs(draft.groupWindowMs);
  const groupingTemplateUnsafe =
    draft.allowGrouping && capability?.groupingRequiresHiddenMessage === true &&
    extractPlaceholderNames(draft.textTemplate).includes('message');
  const formValid =
    nameValid && priorityValid && durationValid && animationDurationValid && thresholdValid &&
    templateValid && unsupported.length === 0 && unknown.length === 0 &&
    groupWindowValid && !groupingTemplateUnsafe;

  const isPending = createMutation.isPending || updateMutation.isPending;
  const errorText = createMutation.isError
    ? errorMessage(t, createMutation.error)
    : updateMutation.isError
      ? errorMessage(t, updateMutation.error)
      : null;

  function updateAccount(index: number, accountId: string) {
    setDraft((d) => ({ ...d, accounts: d.accounts.map((a, i) => (i === index ? accountId : a)) }));
  }

  return (
    <Modal
      open
      onClose={onCancel}
      title={title}
      dismissible={!isPending}
      size="md"
      footer={
        <>
          <Button onClick={onCancel} disabled={isPending}>{t('common.cancel')}</Button>
          <Button
            variant="primary"
            disabled={isPending || !formValid}
            onClick={() => {
              if (editingRuleId !== null) {
                updateMutation.mutate({ id: editingRuleId, input: draft }, { onSuccess: onSaved });
              } else {
                createMutation.mutate(draft, { onSuccess: onSaved });
              }
            }}
          >
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {errorText !== null && <p role="alert" className="text-sm text-status-error">{errorText}</p>}

        <FormField label={t('rules.fields.name')}>
          {({ inputId }) => (
            <TextInput id={inputId} value={draft.name} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} />
          )}
        </FormField>

        <ToggleSwitch
          label={t('rules.fields.enabled')}
          checked={draft.enabled}
          onCheckedChange={(v) => setDraft((d) => ({ ...d, enabled: v }))}
        />

        <FormField label={t('rules.fields.eventType')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              options={ALERT_EVENT_TYPES.map((v) => ({ value: v, label: t(`rules.eventType.${v}`) }))}
              value={draft.eventType}
              onChange={(e) =>
                setDraft((d) => ({
                  ...d,
                  eventType: e.target.value as AlertRuleInput['eventType'],
                  minimumQuantity: null, maximumQuantity: null, showMessage: false, showQuantity: false,
                  allowGrouping: false,
                }))
              }
            />
          )}
        </FormField>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FormField label={t('rules.fields.priority')} hint={t('rules.fields.priorityHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.priority}
                onChange={(e) => setDraft((d) => ({ ...d, priority: Number(e.target.value) }))} />
            )}
          </FormField>
          <FormField label={t('rules.fields.duration')} hint={t('rules.fields.durationHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.durationMs}
                onChange={(e) => setDraft((d) => ({ ...d, durationMs: Number(e.target.value) }))} />
            )}
          </FormField>
        </div>

        {capability?.hasQuantity === true && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <FormField label={t('rules.fields.minimumQuantity')} hint={t('rules.fields.quantityHint')}>
              {({ inputId }) => (
                <TextInput id={inputId} type="number" value={draft.minimumQuantity ?? ''}
                  onChange={(e) => setDraft((d) => ({ ...d, minimumQuantity: e.target.value === '' ? null : Number(e.target.value) }))} />
              )}
            </FormField>
            <FormField label={t('rules.fields.maximumQuantity')}>
              {({ inputId }) => (
                <TextInput id={inputId} type="number" value={draft.maximumQuantity ?? ''}
                  onChange={(e) => setDraft((d) => ({ ...d, maximumQuantity: e.target.value === '' ? null : Number(e.target.value) }))} />
              )}
            </FormField>
          </div>
        )}

        {capability?.hasRoles === true ? (
          <FormField label={t('rules.fields.requiredRole')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={ALERT_ROLES.map((v) => ({ value: v, label: t(`rules.role.${v}`) }))}
                value={draft.requiredRole}
                onChange={(e) => setDraft((d) => ({ ...d, requiredRole: e.target.value as AlertRuleInput['requiredRole'] }))}
              />
            )}
          </FormField>
        ) : (
          <p className="text-[11px] text-ink-faint">{t('rules.roleUnavailableHint')}</p>
        )}

        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <ToggleSwitch label={t('rules.fields.showPlatform')} checked={draft.showPlatform}
            onCheckedChange={(v) => setDraft((d) => ({ ...d, showPlatform: v }))} />
          {capability?.hasUser === true && (
            <ToggleSwitch label={t('rules.fields.showUsername')} checked={draft.showUsername}
              onCheckedChange={(v) => setDraft((d) => ({ ...d, showUsername: v }))} />
          )}
          {capability?.hasMessage === true && (
            <ToggleSwitch
              label={t('rules.fields.showMessage')}
              checked={draft.showMessage}
              disabled={draft.allowGrouping && capability.groupingRequiresHiddenMessage}
              onCheckedChange={(v) => setDraft((d) => ({ ...d, showMessage: v }))}
            />
          )}
          {capability?.hasQuantity === true && (
            <ToggleSwitch label={t('rules.fields.showQuantity')} checked={draft.showQuantity}
              onCheckedChange={(v) => setDraft((d) => ({ ...d, showQuantity: v }))} />
          )}
        </div>

        {capability?.groupable === true ? (
          <div className="space-y-2 rounded-lg border border-line p-3">
            <ToggleSwitch
              label={t('rules.fields.allowGrouping')}
              description={t('rules.fields.allowGroupingHint')}
              checked={draft.allowGrouping}
              onCheckedChange={(v) =>
                setDraft((d) => ({
                  ...d,
                  allowGrouping: v,
                  showMessage: v && capability.groupingRequiresHiddenMessage ? false : d.showMessage,
                }))
              }
            />
            {draft.allowGrouping && (
              <>
                {capability.groupingRequiresHiddenMessage && (
                  <p className="text-[11px] text-ink-faint">{t('rules.fields.groupingHidesMessageHint')}</p>
                )}
                <FormField label={t('rules.fields.groupWindowMs')} hint={t('rules.fields.groupWindowMsHint')}>
                  {({ inputId }) => (
                    <TextInput id={inputId} type="number" value={draft.groupWindowMs}
                      onChange={(e) => setDraft((d) => ({ ...d, groupWindowMs: Number(e.target.value) }))} />
                  )}
                </FormField>
                {groupingTemplateUnsafe && (
                  <p className="text-[11px] text-status-error">{t('rules.fields.groupingTemplateUnsafe')}</p>
                )}
              </>
            )}
          </div>
        ) : (
          <p className="text-[11px] text-ink-faint">{t('rules.groupingUnavailableHint')}</p>
        )}

        <div>
          <FormField
            label={t('rules.fields.textTemplate')}
            counter={t('common.renderedCount', { count: codePointLength(draft.textTemplate) })}
          >
            {({ inputId }) => (
              <TextArea id={inputId} value={draft.textTemplate}
                onChange={(e) => setDraft((d) => ({ ...d, textTemplate: e.target.value }))} />
            )}
          </FormField>
          <div className="mt-1 flex flex-wrap gap-1">
            {(capability?.availablePlaceholders ?? []).map((name) => (
              <button
                key={name}
                type="button"
                className="rounded border border-line px-1.5 py-0.5 text-[11px] hover:bg-surface-hover"
                onClick={() =>
                  setDraft((d) => {
                    const { text } = insertPlaceholder(d.textTemplate, d.textTemplate.length, name);
                    return { ...d, textTemplate: text };
                  })
                }
              >
                {`{${name}}`}
              </button>
            ))}
          </div>
          {(unknown.length > 0 || unsupported.length > 0) && (
            <p className="mt-1 text-[11px] text-status-error">
              {t('common.unresolvedWarning', { names: [...unknown, ...unsupported].join(', ') })}
            </p>
          )}
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <FormField label={t('rules.fields.entryAnimation')}>
            {({ inputId }) => (
              <SelectInput id={inputId} options={ALERT_ANIMATIONS.map((v) => ({ value: v, label: t(`rules.animation.${v}`) }))}
                value={draft.entryAnimation}
                onChange={(e) => setDraft((d) => ({ ...d, entryAnimation: e.target.value as AlertRuleInput['entryAnimation'] }))} />
            )}
          </FormField>
          <FormField label={t('rules.fields.exitAnimation')}>
            {({ inputId }) => (
              <SelectInput id={inputId} options={ALERT_ANIMATIONS.map((v) => ({ value: v, label: t(`rules.animation.${v}`) }))}
                value={draft.exitAnimation}
                onChange={(e) => setDraft((d) => ({ ...d, exitAnimation: e.target.value as AlertRuleInput['exitAnimation'] }))} />
            )}
          </FormField>
          <FormField label={t('rules.fields.animationDuration')}>
            {({ inputId }) => (
              <TextInput id={inputId} type="number" value={draft.animationDurationMs}
                onChange={(e) => setDraft((d) => ({ ...d, animationDurationMs: Number(e.target.value) }))} />
            )}
          </FormField>
        </div>

        <div className="space-y-2 rounded-lg border border-line p-3">
          <p className="text-xs font-medium text-ink-muted">{t('rules.interruption.title')}</p>
          <ToggleSwitch
            label={t('rules.fields.interruptEnabled')}
            description={t('rules.fields.interruptEnabledHint')}
            checked={draft.interruptMode === 'lower_priority'}
            onCheckedChange={(v) =>
              setDraft((d) => ({ ...d, interruptMode: (v ? 'lower_priority' : 'never') as AlertRuleInput['interruptMode'] }))
            }
          />
          <ToggleSwitch
            label={t('rules.fields.interruptible')}
            description={t('rules.fields.interruptibleHint')}
            checked={draft.interruptible}
            onCheckedChange={(v) => setDraft((d) => ({ ...d, interruptible: v }))}
          />
        </div>

        <ToggleSwitch
          label={t('rules.fields.providers')}
          description={t('rules.fields.providersHint')}
          checked={draft.providers.includes('twitch')}
          onCheckedChange={(v) => setDraft((d) => ({ ...d, providers: v ? ['twitch'] : [] }))}
        />

        <div>
          <p className="text-xs font-medium text-ink-muted">{t('rules.fields.accounts')}</p>
          <p className="text-[11px] text-ink-faint">{t('rules.fields.accountsHint')}</p>
          <div className="mt-1 space-y-2">
            {draft.accounts.map((accountId, index) => (
              <div key={index} className="flex items-center gap-2">
                <SelectInput
                  aria-label={t('rules.fields.accounts')}
                  options={[{ value: '', label: '' }, ...twitchAccounts.map((a) => ({ value: a.id, label: a.displayName || a.login }))]}
                  value={accountId}
                  onChange={(e) => updateAccount(index, e.target.value)}
                />
                <IconButton label={t('rules.fields.removeAccount')} icon={<X className="size-4" />}
                  onClick={() => setDraft((d) => ({ ...d, accounts: d.accounts.filter((_, i) => i !== index) }))} />
              </div>
            ))}
          </div>
          <Button size="sm" className="mt-2" icon={<Plus className="size-4" />}
            onClick={() => setDraft((d) => ({ ...d, accounts: [...d.accounts, ''] }))}>
            {t('rules.fields.addAccount')}
          </Button>
          {twitchAccounts.length === 0 && <p className="mt-1 text-[11px] text-status-error">{t('common.noAccounts')}</p>}
        </div>

        <EditorPreview template={draft.textTemplate} eventType={draft.eventType} mutation={previewMutation} />
      </div>
    </Modal>
  );
}

function EditorPreview({
  template,
  eventType,
  mutation,
}: {
  template: string;
  eventType: AlertRuleInput['eventType'];
  mutation: ReturnType<typeof useAlertPreviewMutation>;
}) {
  const { t } = useTranslation('alerts');

  useEffect(() => {
    if (template.trim() === '') return;
    mutation.mutate({ template, eventType });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire on template/eventType identity change only, never resend the queue
  }, [template, eventType]);

  if (mutation.data === undefined) return null;

  return (
    <div className="rounded-lg border border-line bg-surface-sunken p-3">
      <p className="text-xs font-medium text-ink-muted">{t('common.preview')}</p>
      <div className="relative mt-2 h-24 overflow-hidden rounded-lg border border-line/60 bg-canvas/40">
        <AlertRenderer
          config={{ schemaVersion: 1, theme: 'minimal', position: 'center', textAlign: 'center', language: 'en' }}
          current={{
            schemaVersion: 1, alertId: 'preview', eventType, providerId: 'twitch', synthetic: true, replayed: false,
            renderedText: mutation.data.renderedText, durationMs: 999999, entryAnimation: 'none', exitAnimation: 'none',
            animationDurationMs: 0, groupCount: 1,
          }}
        />
      </div>
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
