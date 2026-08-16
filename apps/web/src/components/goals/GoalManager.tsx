import { Plus, RotateCcw, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { Goal, GoalInput, GoalKind, GoalProvider } from '@/api/goals-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { Panel, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useAccountsQuery } from '@/hooks/use-accounts';
import { useDonationSourcesQuery } from '@/hooks/use-donationsources';
import {
  useCreateGoalMutation,
  useDeleteGoalMutation,
  useGoalsQuery,
  useResetGoalMutation,
  useSetGoalCurrentMutation,
  useUpdateGoalMutation,
} from '@/hooks/use-goals';
import { cn } from '@/lib/cn';
import { formatAmountMicros, parseAmountMicros } from '@/models/alerts';
import {
  GOAL_KINDS,
  GOAL_PROVIDERS,
  emptyGoalDraft,
  errorMessage,
  isValidGoalCurrency,
  isValidGoalName,
  isValidGoalTarget,
  isValidGoalValue,
  normalizeCurrencyCode,
} from '@/models/goals';

import { WidgetProfileManager } from './WidgetProfileManager';

function draftFromGoal(goal: Goal): GoalInput {
  return {
    name: goal.name, kind: goal.kind, enabled: goal.enabled, target: goal.target, baseline: goal.baseline,
    currency: goal.currency, providers: goal.providers as GoalProvider[], accounts: goal.accounts,
    configRevision: goal.configRevision,
  };
}

/** Observed progress, never a provider-canonical total - see
 * docs/goals-widgets.md §1. Every place this component shows `current`
 * uses wording that says so, so an operator never mistakes it for "the
 * current follower count on Twitch." */
export function GoalManager() {
  const { t } = useTranslation('goals');
  const goalsQuery = useGoalsQuery();
  const deleteMutation = useDeleteGoalMutation();

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Goal | null>(null);

  const goals = goalsQuery.data ?? [];
  const selected = goals.find((g) => g.id === selectedId) ?? null;

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[320px_1fr] xl:items-start">
      <Panel>
        <PanelHeader
          title={t('page.title')}
          actions={
            <Button size="sm" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
              {t('common.create')}
            </Button>
          }
        />
        <div className="p-2">
          {goals.length === 0 ? (
            <p className="p-3 text-sm text-ink-muted">{t('goals.empty')}</p>
          ) : (
            <ul className="space-y-1">
              {goals.map((goal) => (
                <li key={goal.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(goal.id)}
                    aria-current={goal.id === selectedId}
                    className={cn(
                      'flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm transition-colors',
                      goal.id === selectedId ? 'bg-accent/15 text-ink' : 'text-ink-muted hover:bg-surface-hover',
                    )}
                  >
                    <span className="min-w-0">
                      <span className="block truncate">{goal.name}</span>
                      <span className="block text-[11px] text-ink-faint">{t(`goals.kind.${goal.kind}`)}</span>
                    </span>
                    {!goal.enabled && (
                      <span className="shrink-0 rounded-full border border-line px-1.5 py-0.5 text-[10px] text-ink-faint">
                        {t('goals.fields.enabled')}: off
                      </span>
                    )}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Panel>

      {selected !== null && (
        <div className="space-y-4">
          <GoalEditor key={selected.id} goal={selected} onDeleteRequested={() => setDeleteTarget(selected)} />
          <WidgetProfileManager goalId={selected.id} />
        </div>
      )}

      {creating && (
        <CreateGoalModal
          onCancel={() => setCreating(false)}
          onCreated={(goal) => {
            setCreating(false);
            setSelectedId(goal.id);
          }}
        />
      )}

      {deleteTarget !== null && (
        <ConfirmDialog
          open
          title={t('goals.deleteConfirmTitle')}
          message={t('goals.deleteConfirmMessage', { name: deleteTarget.name })}
          confirmLabel={t('common.delete')}
          destructive
          busy={deleteMutation.isPending}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => {
            deleteMutation.mutate(deleteTarget.id, {
              onSuccess: () => {
                setDeleteTarget(null);
                if (selectedId === deleteTarget.id) setSelectedId(null);
              },
            });
          }}
        />
      )}
      {deleteMutation.isError && (
        <p role="alert" className="text-sm text-status-error">
          {errorMessage(t, deleteMutation.error)}
        </p>
      )}
    </div>
  );
}

function CreateGoalModal({ onCancel, onCreated }: { onCancel: () => void; onCreated: (goal: Goal) => void }) {
  const { t } = useTranslation('goals');
  const createMutation = useCreateGoalMutation();
  const [name, setName] = useState('');
  const [kind, setKind] = useState<GoalKind>('followers');

  const nameValid = isValidGoalName(name);

  return (
    <Modal
      open
      onClose={onCancel}
      title={t('goals.createTitle')}
      dismissible={!createMutation.isPending}
      size="sm"
      footer={
        <>
          <Button onClick={onCancel} disabled={createMutation.isPending}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            disabled={createMutation.isPending || !nameValid}
            onClick={() => createMutation.mutate({ ...emptyGoalDraft(kind), name }, { onSuccess: onCreated })}
          >
            {t('common.create')}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <FormField label={t('goals.fields.name')}>
          {({ inputId }) => <TextInput id={inputId} value={name} onChange={(e) => setName(e.target.value)} autoFocus />}
        </FormField>
        <FormField label={t('goals.fields.kind')} hint={t('goals.fields.kindHint')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              options={GOAL_KINDS.map((k) => ({ value: k, label: t(`goals.kind.${k}`) }))}
              value={kind}
              onChange={(e) => setKind(e.target.value as GoalKind)}
            />
          )}
        </FormField>
      </div>
      {createMutation.isError && (
        <p role="alert" className="mt-2 text-sm text-status-error">
          {errorMessage(t, createMutation.error)}
        </p>
      )}
    </Modal>
  );
}

function GoalEditor({ goal, onDeleteRequested }: { goal: Goal; onDeleteRequested: () => void }) {
  const { t } = useTranslation('goals');
  const updateMutation = useUpdateGoalMutation();
  const setCurrentMutation = useSetGoalCurrentMutation();
  const resetMutation = useResetGoalMutation();
  const accountsQuery = useAccountsQuery();
  const donationSourcesQuery = useDonationSourcesQuery();

  const [draft, setDraft] = useState<GoalInput>(draftFromGoal(goal));
  const isMoney = draft.kind === 'donations';

  // Local decimal-string display state for money fields - mirrors
  // RuleManager.tsx's own identical amount-threshold pattern exactly
  // (the draft itself always stores integer micros).
  const [targetText, setTargetText] = useState(() => (isMoney ? formatAmountMicros(draft.target) : ''));
  const [baselineText, setBaselineText] = useState(() => (isMoney ? formatAmountMicros(draft.baseline) : ''));
  const [setCurrentText, setSetCurrentText] = useState('');

  const filterableAccounts = (accountsQuery.data ?? []).filter((a) => a.providerId === 'twitch' || a.providerId === 'youtube');
  const donationSources = (donationSourcesQuery.data ?? []).map((s) => ({ id: s.id, displayName: s.label }));
  const filterableAccountOptions = [...filterableAccounts.map((a) => ({ id: a.id, displayName: a.displayName })), ...donationSources];

  const nameValid = isValidGoalName(draft.name);
  const targetValid = isMoney ? targetText.trim() !== '' && parseAmountMicros(targetText) !== null : isValidGoalTarget(draft.kind, draft.target);
  const baselineValid = isMoney
    ? baselineText.trim() === '' || parseAmountMicros(baselineText) !== null
    : isValidGoalValue(draft.kind, draft.baseline);
  const currencyValid = isValidGoalCurrency(draft.kind, draft.currency);
  const formValid = nameValid && targetValid && baselineValid && currencyValid;
  const dirty = JSON.stringify(draft) !== JSON.stringify(draftFromGoal(goal));

  const setCurrentValid = isMoney ? setCurrentText.trim() !== '' && parseAmountMicros(setCurrentText) !== null : /^\d+$/.test(setCurrentText);

  function commit(next: Partial<GoalInput>) {
    setDraft((d) => ({ ...d, ...next }));
  }

  return (
    <Panel>
      <PanelHeader
        title={goal.name}
        actions={<IconButton label={t('common.delete')} icon={<Trash2 className="size-4" />} variant="danger" onClick={onDeleteRequested} />}
      />
      <div className="space-y-4 p-4 sm:p-5">
        <div className="rounded-lg border border-line bg-surface-sunken p-3">
          <p className="text-xs font-medium text-ink-muted">{t('goals.progress.label')}</p>
          <p className="mt-1 text-lg font-semibold text-ink">
            {isMoney ? formatAmountMicros(goal.current) : goal.current.toLocaleString()}
            {' / '}
            {isMoney ? formatAmountMicros(goal.target) : goal.target.toLocaleString()}
            {goal.currency ? ` ${goal.currency}` : ''}
          </p>
          <p className="mt-0.5 text-[11px] text-ink-faint">{t('goals.progress.hint')}</p>
          {goal.completed && <p className="mt-1 text-xs font-medium text-status-success">{t('goals.progress.completed')}</p>}

          <div className="mt-3 flex flex-wrap items-end gap-2">
            <FormField label={t('goals.actions.setCurrent')} className="max-w-[180px]">
              {({ inputId }) => (
                <TextInput id={inputId} value={setCurrentText} onChange={(e) => setSetCurrentText(e.target.value)} placeholder="0" />
              )}
            </FormField>
            <Button
              size="sm"
              disabled={!setCurrentValid || setCurrentMutation.isPending}
              onClick={() => {
                const value = isMoney ? parseAmountMicros(setCurrentText) : Number(setCurrentText);
                if (value === null || Number.isNaN(value)) return;
                setCurrentMutation.mutate({ id: goal.id, current: value }, { onSuccess: () => setSetCurrentText('') });
              }}
            >
              {t('goals.actions.apply')}
            </Button>
            <Button
              size="sm"
              icon={<RotateCcw className="size-4" />}
              disabled={resetMutation.isPending}
              onClick={() => resetMutation.mutate(goal.id)}
            >
              {t('goals.actions.reset')}
            </Button>
          </div>
          {(setCurrentMutation.isError || resetMutation.isError) && (
            <p role="alert" className="mt-1 text-xs text-status-error">
              {errorMessage(t, setCurrentMutation.error ?? resetMutation.error)}
            </p>
          )}
        </div>

        <FormField label={t('goals.fields.name')}>
          {({ inputId }) => <TextInput id={inputId} value={draft.name} onChange={(e) => commit({ name: e.target.value })} />}
        </FormField>

        <ToggleSwitch
          label={t('goals.fields.enabled')}
          description={t('goals.fields.enabledHint')}
          checked={draft.enabled}
          onCheckedChange={(v) => commit({ enabled: v })}
        />

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FormField label={t('goals.fields.target')} {...(isMoney ? { hint: t('goals.fields.moneyHint') } : {})}>
            {({ inputId }) =>
              isMoney ? (
                <TextInput
                  id={inputId}
                  value={targetText}
                  onChange={(e) => {
                    setTargetText(e.target.value);
                    const parsed = parseAmountMicros(e.target.value);
                    if (parsed !== null) commit({ target: parsed });
                  }}
                  placeholder="0.00"
                />
              ) : (
                <TextInput
                  id={inputId}
                  value={String(draft.target)}
                  onChange={(e) => commit({ target: Number(e.target.value) || 0 })}
                />
              )
            }
          </FormField>
          <FormField label={t('goals.fields.baseline')} hint={t('goals.fields.baselineHint')}>
            {({ inputId }) =>
              isMoney ? (
                <TextInput
                  id={inputId}
                  value={baselineText}
                  onChange={(e) => {
                    setBaselineText(e.target.value);
                    const parsed = parseAmountMicros(e.target.value);
                    if (parsed !== null) commit({ baseline: parsed });
                  }}
                  placeholder="0.00"
                />
              ) : (
                <TextInput
                  id={inputId}
                  value={String(draft.baseline)}
                  onChange={(e) => commit({ baseline: Number(e.target.value) || 0 })}
                />
              )
            }
          </FormField>
        </div>

        {isMoney && (
          <FormField label={t('goals.fields.currency')} hint={t('goals.fields.currencyHint')}>
            {({ inputId }) => (
              <TextInput
                id={inputId}
                value={draft.currency ?? ''}
                onChange={(e) => commit({ currency: normalizeCurrencyCode(e.target.value) })}
                placeholder="USD"
              />
            )}
          </FormField>
        )}

        <div>
          <p className="text-xs font-medium text-ink-muted">{t('goals.fields.providers')}</p>
          <p className="text-[11px] text-ink-faint">{t('goals.fields.providersHint')}</p>
          <div className="mt-2 flex flex-wrap gap-3">
            {GOAL_PROVIDERS.map((provider) => (
              <label key={provider} className="flex items-center gap-1.5 text-xs text-ink">
                <input
                  type="checkbox"
                  checked={draft.providers.includes(provider)}
                  onChange={(e) =>
                    commit({ providers: e.target.checked ? [...draft.providers, provider] : draft.providers.filter((p) => p !== provider) })
                  }
                />
                {t(`goals.provider.${provider}`)}
              </label>
            ))}
          </div>
        </div>

        <div>
          <p className="text-xs font-medium text-ink-muted">{t('goals.fields.accounts')}</p>
          <p className="text-[11px] text-ink-faint">{t('goals.fields.accountsHint')}</p>
          <div className="mt-2 space-y-2">
            {draft.accounts.map((accountId, index) => (
              <div key={index} className="flex items-center gap-2">
                <SelectInput
                  aria-label={t('goals.fields.accounts')}
                  options={[
                    { value: '', label: t('goals.fields.selectAccount') },
                    ...filterableAccountOptions.map((a) => ({ value: a.id, label: a.displayName })),
                  ]}
                  value={accountId}
                  onChange={(e) => commit({ accounts: draft.accounts.map((a, i) => (i === index ? e.target.value : a)) })}
                />
                <IconButton
                  label={t('common.delete')}
                  icon={<Trash2 className="size-4" />}
                  variant="danger"
                  onClick={() => commit({ accounts: draft.accounts.filter((_, i) => i !== index) })}
                />
              </div>
            ))}
            <Button size="sm" icon={<Plus className="size-4" />} onClick={() => commit({ accounts: [...draft.accounts, ''] })}>
              {t('goals.fields.addAccount')}
            </Button>
            {filterableAccountOptions.length === 0 && <p className="mt-1 text-[11px] text-status-error">{t('common.noAccounts')}</p>}
          </div>
        </div>

        <div className="flex items-center gap-2 border-t border-line pt-4">
          <Button
            variant="primary"
            disabled={!formValid || !dirty || updateMutation.isPending}
            onClick={() => updateMutation.mutate({ id: goal.id, input: draft })}
          >
            {t('common.save')}
          </Button>
          {updateMutation.isError && (
            <p role="alert" className="text-sm text-status-error">
              {errorMessage(t, updateMutation.error)}
            </p>
          )}
        </div>
      </div>
    </Panel>
  );
}
