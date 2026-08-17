import { Check, Copy, ExternalLink, Plus, RefreshCw, RotateCcw, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type {
  DashboardChild,
  GoalProvider,
  PublicWidgetSnapshot,
  SessionMetric,
  SupporterEventType,
  WidgetProfile,
  WidgetProfileInput,
  WidgetProfileKind,
} from '@/api/goals-schemas';
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
  useAllWidgetProfilesQuery,
  useCreateWidgetProfileMutation,
  useDeleteWidgetProfileMutation,
  useResetWidgetRuntimeMutation,
  useRotateWidgetProfileSlugMutation,
  useUpdateWidgetProfileMutation,
  useWidgetRuntimeStatusQuery,
} from '@/hooks/use-goals';
import { cn } from '@/lib/cn';
import {
  GOAL_PROVIDERS,
  MAX_DASHBOARD_CHILDREN,
  MAX_DASHBOARD_COLUMNS,
  MAX_EVENT_TICKER_ITEMS,
  MAX_RECENT_SUPPORTERS,
  MIN_DASHBOARD_COLUMNS,
  MIN_MAX_ITEMS,
  SESSION_METRICS,
  SUPPORTER_EVENT_TYPES,
  SUPPORTER_WIDGET_KINDS,
  defaultWidgetProfileDraftOfKind,
  emptyDashboardChild,
  errorMessage,
  isValidWidgetProfileFields,
  normalizeCurrencyCode,
  widgetKindHasOwnFilters,
  widgetKindIsDashboard,
  widgetKindRequiresCurrency,
  widgetKindRequiresMaxItems,
} from '@/models/goals';

import { GoalWidgetRenderer } from './GoalWidgetRenderer';

function resolveWidgetUrl(publicSlug: string): string {
  return `${window.location.origin}/overlay/widgets/${publicSlug}`;
}

function draftFromProfile(p: WidgetProfile): WidgetProfileInput {
  return {
    kind: p.kind, goalId: undefined, name: p.name, enabled: p.enabled,
    providers: (p.providers ?? []) as GoalProvider[], accounts: p.accounts ?? [],
    titleOverride: p.titleOverride,
    showCurrent: p.showCurrent, showTarget: p.showTarget, showPercent: p.showPercent,
    showProvider: p.showProvider ?? true, showTime: p.showTime ?? true, showMessage: p.showMessage ?? false,
    maxItems: p.maxItems ?? 0, currency: p.currency, metric: p.metric, eventTypes: p.eventTypes ?? [],
    columns: p.columns ?? 0, children: p.children ?? [],
    orientation: p.orientation, textAlign: p.textAlign, fontFamily: p.fontFamily,
    backgroundColor: p.backgroundColor, foregroundColor: p.foregroundColor, fillColor: p.fillColor, borderColor: p.borderColor,
    borderRadiusPx: p.borderRadiusPx, opacity: p.opacity,
  };
}

/** Builds a preview PublicWidgetSnapshot straight from a draft (never a
 * server round trip) so the in-editor preview reflects style changes
 * immediately - runtime content (latest/recent/ticker/counter) is
 * populated separately from real runtime status when available. */
function previewSnapshot(draft: WidgetProfileInput, title: string, runtimeExtra: Partial<PublicWidgetSnapshot>): PublicWidgetSnapshot {
  return {
    revision: 1, kind: draft.kind, title,
    presentation: {
      showCurrent: draft.showCurrent, showTarget: draft.showTarget, showPercent: draft.showPercent,
      showProvider: draft.showProvider, showTime: draft.showTime, columns: draft.columns,
      orientation: draft.orientation, textAlign: draft.textAlign, fontFamily: draft.fontFamily,
      backgroundColor: draft.backgroundColor, foregroundColor: draft.foregroundColor,
      fillColor: draft.fillColor, borderColor: draft.borderColor,
      borderRadiusPx: draft.borderRadiusPx, opacity: draft.opacity,
    },
    ...runtimeExtra,
  };
}

/**
 * Management UI for every Stage 18B widget-profile kind other than
 * `goal` (which keeps its own existing per-goal WidgetProfileManager
 * UI) - `dashboardsOnly` switches between the "Widgets" list (every
 * event-derived kind) and the "Dashboards" list (docs/supporter-
 * widgets.md §44).
 */
export function SupporterWidgetManager({ dashboardsOnly }: { dashboardsOnly: boolean }) {
  const { t } = useTranslation('goals');
  const profilesQuery = useAllWidgetProfilesQuery();
  const deleteMutation = useDeleteWidgetProfileMutation();

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<WidgetProfile | null>(null);

  const all = profilesQuery.data ?? [];
  const profiles = all.filter((p) => (dashboardsOnly ? widgetKindIsDashboard(p.kind) : SUPPORTER_WIDGET_KINDS.includes(p.kind)));
  const selected = profiles.find((p) => p.id === selectedId) ?? null;
  const availableChildren = all.filter((p) => !widgetKindIsDashboard(p.kind));

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[320px_1fr] xl:items-start">
      <Panel>
        <PanelHeader
          title={dashboardsOnly ? t('dashboards.title') : t('widgets.title')}
          actions={
            <Button size="sm" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
              {dashboardsOnly ? t('dashboards.add') : t('widgets.add')}
            </Button>
          }
        />
        <div className="p-2">
          {profiles.length === 0 ? (
            <p className="p-3 text-sm text-ink-muted">{dashboardsOnly ? t('dashboards.empty') : t('widgets.emptyForGoal')}</p>
          ) : (
            <ul className="space-y-1">
              {profiles.map((p) => (
                <li key={p.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(p.id)}
                    aria-current={p.id === selectedId}
                    className={cn(
                      'flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm transition-colors',
                      p.id === selectedId ? 'bg-accent/15 text-ink' : 'text-ink-muted hover:bg-surface-hover',
                    )}
                  >
                    <span className="min-w-0">
                      <span className="block truncate">{p.name}</span>
                      <span className="block text-[11px] text-ink-faint">{t(`widgets.kind.${p.kind}`)}</span>
                    </span>
                    {!p.enabled && <span className="shrink-0 rounded-full border border-line px-1.5 py-0.5 text-[10px] text-ink-faint">off</span>}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Panel>

      {selected !== null && (
        <SupporterWidgetEditor
          key={selected.id}
          profile={selected}
          availableChildren={availableChildren}
          onDeleteRequested={() => setDeleteTarget(selected)}
        />
      )}

      {creating && (
        <CreateSupporterWidgetModal
          dashboardsOnly={dashboardsOnly}
          onCancel={() => setCreating(false)}
          onCreated={(p) => {
            setCreating(false);
            setSelectedId(p.id);
          }}
        />
      )}

      {deleteTarget !== null && (
        <ConfirmDialog
          open
          title={t('widgets.deleteConfirmTitle')}
          message={t('widgets.deleteConfirmMessage', { name: deleteTarget.name })}
          confirmLabel={t('common.delete')}
          destructive
          busy={deleteMutation.isPending}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() =>
            deleteMutation.mutate(deleteTarget.id, {
              onSuccess: () => {
                setDeleteTarget(null);
                if (selectedId === deleteTarget.id) setSelectedId(null);
              },
            })
          }
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

function CreateSupporterWidgetModal({
  dashboardsOnly,
  onCancel,
  onCreated,
}: {
  dashboardsOnly: boolean;
  onCancel: () => void;
  onCreated: (p: WidgetProfile) => void;
}) {
  const { t } = useTranslation('goals');
  const createMutation = useCreateWidgetProfileMutation();
  const [name, setName] = useState('');
  const [kind, setKind] = useState<WidgetProfileKind>(dashboardsOnly ? 'dashboard' : 'latest_follower');

  const nameValid = name.trim() !== '';

  return (
    <Modal
      open
      onClose={onCancel}
      title={dashboardsOnly ? t('dashboards.createTitle') : t('widgets.createTitle')}
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
            onClick={() => createMutation.mutate({ ...defaultWidgetProfileDraftOfKind(kind), name }, { onSuccess: onCreated })}
          >
            {t('common.create')}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <FormField label={t('widgets.fields.name')}>
          {({ inputId }) => <TextInput id={inputId} value={name} onChange={(e) => setName(e.target.value)} autoFocus />}
        </FormField>
        {!dashboardsOnly && (
          <FormField label={t('goals.fields.kind')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={SUPPORTER_WIDGET_KINDS.map((k) => ({ value: k, label: t(`widgets.kind.${k}`) }))}
                value={kind}
                onChange={(e) => setKind(e.target.value as WidgetProfileKind)}
              />
            )}
          </FormField>
        )}
      </div>
      {createMutation.isError && (
        <p role="alert" className="mt-2 text-sm text-status-error">
          {errorMessage(t, createMutation.error)}
        </p>
      )}
    </Modal>
  );
}

function SupporterWidgetEditor({
  profile,
  availableChildren,
  onDeleteRequested,
}: {
  profile: WidgetProfile;
  availableChildren: WidgetProfile[];
  onDeleteRequested: () => void;
}) {
  const { t } = useTranslation('goals');
  const updateMutation = useUpdateWidgetProfileMutation();
  const rotateMutation = useRotateWidgetProfileSlugMutation();
  const resetRuntimeMutation = useResetWidgetRuntimeMutation();
  const accountsQuery = useAccountsQuery();
  const donationSourcesQuery = useDonationSourcesQuery();
  const runtimeStatusQuery = useWidgetRuntimeStatusQuery(widgetKindHasOwnFilters(profile.kind) ? profile.id : null);

  const [draft, setDraft] = useState<WidgetProfileInput>(draftFromProfile(profile));
  const [rotateConfirmOpen, setRotateConfirmOpen] = useState(false);
  const [resetConfirmOpen, setResetConfirmOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const url = resolveWidgetUrl(profile.publicSlug);
  const formValid = isValidWidgetProfileFields(draft);
  const dirty = JSON.stringify(draft) !== JSON.stringify(draftFromProfile(profile));
  const isDashboard = widgetKindIsDashboard(profile.kind);
  const hasFilters = widgetKindHasOwnFilters(profile.kind);

  const filterableAccounts = (accountsQuery.data ?? []).filter((a) => a.providerId === 'twitch' || a.providerId === 'youtube');
  const donationSources = (donationSourcesQuery.data ?? []).map((s) => ({ id: s.id, displayName: s.label }));
  const filterableAccountOptions = [...filterableAccounts.map((a) => ({ id: a.id, displayName: a.displayName })), ...donationSources];

  function commit(next: Partial<WidgetProfileInput>) {
    setDraft((d) => ({ ...d, ...next }));
  }

  const runtimeExtra: Partial<PublicWidgetSnapshot> = runtimeStatusQuery.data
    ? {
        latest: runtimeStatusQuery.data.latest,
        largest: runtimeStatusQuery.data.largest,
        recent: runtimeStatusQuery.data.recent,
        ticker: runtimeStatusQuery.data.ticker,
        counter: runtimeStatusQuery.data.counter,
      }
    : {};

  return (
    <div className="space-y-4">
      <Panel>
        <PanelHeader
          title={profile.name}
          actions={<IconButton label={t('common.delete')} icon={<Trash2 className="size-4" />} variant="danger" onClick={onDeleteRequested} />}
        />
        <div className="space-y-4 p-4 sm:p-5">
          <div className="rounded-lg border border-line bg-surface-sunken p-3">
            <p className="text-xs font-medium text-ink-muted">{t(`widgets.kind.${profile.kind}`)}</p>
            {hasFilters && <p className="mt-1 text-[11px] text-ink-faint">{t('widgets.runtimeNote')}</p>}
            {hasFilters && (
              <div className="mt-2 flex items-center gap-2">
                <Button size="sm" icon={<RotateCcw className="size-4" />} onClick={() => setResetConfirmOpen(true)}>
                  {t('widgets.resetRuntime')}
                </Button>
              </div>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded-lg border border-line bg-surface-sunken px-2 py-1.5 text-xs text-ink">{url}</code>
            <Button
              size="sm"
              icon={copied ? <Check className="size-4" /> : <Copy className="size-4" />}
              onClick={() => {
                void navigator.clipboard.writeText(url);
                setCopied(true);
                window.setTimeout(() => setCopied(false), 2000);
              }}
            >
              {copied ? t('widgets.copied') : t('widgets.copyUrl')}
            </Button>
            <Button size="sm" icon={<ExternalLink className="size-4" />} onClick={() => window.open(url, '_blank', 'noopener,noreferrer')}>
              {t('widgets.openUrl')}
            </Button>
            <Button size="sm" icon={<RefreshCw className="size-4" />} onClick={() => setRotateConfirmOpen(true)}>
              {t('widgets.rotateAction')}
            </Button>
          </div>

          <FormField label={t('widgets.fields.name')}>
            {({ inputId }) => <TextInput id={inputId} value={draft.name} onChange={(e) => commit({ name: e.target.value })} />}
          </FormField>
          <FormField label={t('widgets.fields.titleOverride')} hint={t('widgets.fields.titleOverrideHint')}>
            {({ inputId }) => <TextInput id={inputId} value={draft.titleOverride ?? ''} onChange={(e) => commit({ titleOverride: e.target.value })} />}
          </FormField>
          <ToggleSwitch label={t('widgets.fields.enabled')} checked={draft.enabled} onCheckedChange={(v) => commit({ enabled: v })} />

          {hasFilters && (
            <SupporterWidgetKindFields draft={draft} commit={commit} />
          )}

          {isDashboard && (
            <DashboardChildrenEditor draft={draft} commit={commit} availableChildren={availableChildren} />
          )}

          {hasFilters && (
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
          )}

          {hasFilters && (
            <div>
              <p className="text-xs font-medium text-ink-muted">{t('goals.fields.accounts')}</p>
              <p className="text-[11px] text-ink-faint">{t('goals.fields.accountsHint')}</p>
              <div className="mt-2 space-y-2">
                {draft.accounts.map((accountId, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <SelectInput
                      aria-label={t('goals.fields.accounts')}
                      options={[{ value: '', label: t('goals.fields.selectAccount') }, ...filterableAccountOptions.map((a) => ({ value: a.id, label: a.displayName }))]}
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
              </div>
            </div>
          )}

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <FormField label={t('widgets.fields.backgroundColor')}>
              {({ inputId }) => <TextInput id={inputId} value={draft.backgroundColor} onChange={(e) => commit({ backgroundColor: e.target.value })} />}
            </FormField>
            <FormField label={t('widgets.fields.foregroundColor')}>
              {({ inputId }) => <TextInput id={inputId} value={draft.foregroundColor} onChange={(e) => commit({ foregroundColor: e.target.value })} />}
            </FormField>
            <FormField label={t('widgets.fields.fillColor')}>
              {({ inputId }) => <TextInput id={inputId} value={draft.fillColor} onChange={(e) => commit({ fillColor: e.target.value })} />}
            </FormField>
            <FormField label={t('widgets.fields.borderColor')}>
              {({ inputId }) => <TextInput id={inputId} value={draft.borderColor} onChange={(e) => commit({ borderColor: e.target.value })} />}
            </FormField>
          </div>

          <div className="rounded-lg border border-line bg-surface-sunken p-3">
            <p className="mb-2 text-xs font-medium text-ink-muted">{t('widgets.preview')}</p>
            <GoalWidgetRenderer snapshot={previewSnapshot(draft, draft.titleOverride || profile.name, runtimeExtra)} />
          </div>

          <div className="flex items-center gap-2 border-t border-line pt-3">
            <Button variant="primary" disabled={!formValid || !dirty || updateMutation.isPending} onClick={() => updateMutation.mutate({ id: profile.id, input: draft })}>
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

      <ConfirmDialog
        open={rotateConfirmOpen}
        title={t('widgets.rotateConfirmTitle')}
        message={t('widgets.rotateConfirmMessage')}
        confirmLabel={t('widgets.rotateAction')}
        busy={rotateMutation.isPending}
        onCancel={() => setRotateConfirmOpen(false)}
        onConfirm={() => rotateMutation.mutate(profile.id, { onSuccess: () => setRotateConfirmOpen(false) })}
      />
      <ConfirmDialog
        open={resetConfirmOpen}
        title={t('widgets.resetRuntimeConfirmTitle')}
        message={t('widgets.resetRuntimeConfirmMessage')}
        confirmLabel={t('widgets.resetRuntime')}
        busy={resetRuntimeMutation.isPending}
        onCancel={() => setResetConfirmOpen(false)}
        onConfirm={() => resetRuntimeMutation.mutate(profile.id, { onSuccess: () => setResetConfirmOpen(false) })}
      />
    </div>
  );
}

function SupporterWidgetKindFields({
  draft,
  commit,
}: {
  draft: WidgetProfileInput;
  commit: (next: Partial<WidgetProfileInput>) => void;
}) {
  const { t } = useTranslation('goals');

  return (
    <>
      <div className="flex flex-wrap gap-4">
        <ToggleSwitch label={t('widgets.fields.showProvider')} checked={draft.showProvider} onCheckedChange={(v) => commit({ showProvider: v })} />
        <ToggleSwitch label={t('widgets.fields.showTime')} checked={draft.showTime} onCheckedChange={(v) => commit({ showTime: v })} />
        {draft.kind === 'latest_donation' && (
          <ToggleSwitch
            label={t('widgets.fields.showMessage')}
            description={t('widgets.fields.showMessageHint')}
            checked={draft.showMessage}
            onCheckedChange={(v) => commit({ showMessage: v })}
          />
        )}
      </div>

      {widgetKindRequiresMaxItems(draft.kind) && (
        <FormField label={t('widgets.fields.maxItems')}>
          {({ inputId }) => (
            <TextInput
              id={inputId}
              value={String(draft.maxItems)}
              onChange={(e) => {
                const max = draft.kind === 'event_ticker' ? MAX_EVENT_TICKER_ITEMS : MAX_RECENT_SUPPORTERS;
                const n = Math.min(max, Math.max(MIN_MAX_ITEMS, Number(e.target.value) || 0));
                commit({ maxItems: n });
              }}
            />
          )}
        </FormField>
      )}

      {widgetKindRequiresCurrency(draft.kind) && (
        <FormField label={t('widgets.fields.currency')} hint={t('widgets.fields.currencyHint')}>
          {({ inputId }) => (
            <TextInput id={inputId} value={draft.currency ?? ''} onChange={(e) => commit({ currency: normalizeCurrencyCode(e.target.value) })} placeholder="USD" />
          )}
        </FormField>
      )}

      {draft.kind === 'session_counter' && (
        <>
          <FormField label={t('widgets.fields.metric')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={SESSION_METRICS.map((m) => ({ value: m, label: t(`widgets.metric.${m}`) }))}
                value={draft.metric ?? 'follows'}
                onChange={(e) => {
                  const metric = e.target.value as SessionMetric;
                  commit({ metric, currency: metric === 'support_amount' ? (draft.currency ?? 'USD') : undefined });
                }}
              />
            )}
          </FormField>
          {draft.metric === 'support_amount' && (
            <FormField label={t('widgets.fields.currency')} hint={t('widgets.fields.currencyHint')}>
              {({ inputId }) => (
                <TextInput id={inputId} value={draft.currency ?? ''} onChange={(e) => commit({ currency: normalizeCurrencyCode(e.target.value) })} placeholder="USD" />
              )}
            </FormField>
          )}
        </>
      )}

      {draft.kind === 'event_ticker' && (
        <div>
          <p className="text-xs font-medium text-ink-muted">{t('widgets.fields.eventTypes')}</p>
          <div className="mt-2 flex flex-wrap gap-3">
            {SUPPORTER_EVENT_TYPES.map((eventType) => (
              <label key={eventType} className="flex items-center gap-1.5 text-xs text-ink">
                <input
                  type="checkbox"
                  checked={draft.eventTypes.includes(eventType)}
                  onChange={(e) => {
                    const types: SupporterEventType[] = e.target.checked
                      ? [...draft.eventTypes, eventType]
                      : draft.eventTypes.filter((t2) => t2 !== eventType);
                    commit({ eventTypes: types });
                  }}
                />
                {t(`widgets.eventType.${eventType}`)}
              </label>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

function DashboardChildrenEditor({
  draft,
  commit,
  availableChildren,
}: {
  draft: WidgetProfileInput;
  commit: (next: Partial<WidgetProfileInput>) => void;
  availableChildren: WidgetProfile[];
}) {
  const { t } = useTranslation('goals');
  const unusedChildren = availableChildren.filter((c) => !draft.children.some((dc) => dc.widgetProfileId === c.id));

  function updateChild(index: number, next: Partial<DashboardChild>) {
    commit({ children: draft.children.map((c, i) => (i === index ? { ...c, ...next } : c)) });
  }

  return (
    <div className="space-y-3 rounded-lg border border-line p-3">
      <FormField label={t('widgets.fields.columns')}>
        {({ inputId }) => (
          <TextInput
            id={inputId}
            value={String(draft.columns)}
            onChange={(e) => {
              const n = Math.min(MAX_DASHBOARD_COLUMNS, Math.max(MIN_DASHBOARD_COLUMNS, Number(e.target.value) || MIN_DASHBOARD_COLUMNS));
              commit({ columns: n });
            }}
          />
        )}
      </FormField>

      <p className="text-xs font-medium text-ink-muted">{t('dashboards.children')}</p>
      {draft.children.length === 0 && <p className="text-[11px] text-ink-faint">{t('widgets.empty.dashboard')}</p>}
      <div className="space-y-2">
        {draft.children.map((child, index) => {
          const childProfile = availableChildren.find((c) => c.id === child.widgetProfileId);
          return (
            <div key={index} className="flex flex-wrap items-center gap-2 rounded-lg border border-line p-2 text-xs">
              <span className="min-w-0 flex-1 truncate font-medium">{childProfile?.name ?? child.widgetProfileId}</span>
              <label className="flex items-center gap-1">
                col
                <TextInput
                  aria-label="column"
                  className="w-14"
                  value={String(child.column)}
                  onChange={(e) => updateChild(index, { column: Number(e.target.value) || 1 })}
                />
              </label>
              <label className="flex items-center gap-1">
                span
                <TextInput
                  aria-label="columnSpan"
                  className="w-14"
                  value={String(child.columnSpan)}
                  onChange={(e) => updateChild(index, { columnSpan: Number(e.target.value) || 1 })}
                />
              </label>
              <label className="flex items-center gap-1">
                row
                <TextInput
                  aria-label="row"
                  className="w-14"
                  value={String(child.row)}
                  onChange={(e) => updateChild(index, { row: Number(e.target.value) || 1 })}
                />
              </label>
              <IconButton
                label={t('common.delete')}
                icon={<Trash2 className="size-4" />}
                variant="danger"
                onClick={() => commit({ children: draft.children.filter((_, i) => i !== index) })}
              />
            </div>
          );
        })}
      </div>

      {draft.children.length < MAX_DASHBOARD_CHILDREN && (
        unusedChildren.length === 0 ? (
          <p className="text-[11px] text-ink-faint">{t('dashboards.noAvailableChildren')}</p>
        ) : (
          <SelectInput
            aria-label={t('dashboards.addChild')}
            options={[{ value: '', label: t('dashboards.addChild') }, ...unusedChildren.map((c) => ({ value: c.id, label: c.name }))]}
            value=""
            onChange={(e) => {
              if (e.target.value === '') return;
              commit({ children: [...draft.children, emptyDashboardChild(e.target.value)] });
            }}
          />
        )
      )}
    </div>
  );
}
