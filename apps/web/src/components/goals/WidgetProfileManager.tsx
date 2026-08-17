import { Check, Copy, ExternalLink, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { WidgetFontFamily, WidgetOrientation, WidgetProfile, WidgetProfileInput, WidgetTextAlign } from '@/api/goals-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { Panel, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  useCreateWidgetProfileMutation,
  useDeleteWidgetProfileMutation,
  useRotateWidgetProfileSlugMutation,
  useUpdateWidgetProfileMutation,
  useWidgetProfilesQuery,
} from '@/hooks/use-goals';
import {
  WIDGET_FONT_FAMILIES,
  WIDGET_ORIENTATIONS,
  WIDGET_TEXT_ALIGNS,
  defaultWidgetProfileDraft,
  errorMessage,
  isValidWidgetProfileFields,
} from '@/models/goals';

import { GoalWidgetRenderer } from './GoalWidgetRenderer';

function resolveWidgetUrl(publicSlug: string): string {
  return `${window.location.origin}/overlay/widgets/${publicSlug}`;
}

function draftFromProfile(p: WidgetProfile): WidgetProfileInput {
  return {
    kind: 'goal', goalId: p.goalId, name: p.name, enabled: p.enabled,
    providers: [], accounts: [], titleOverride: p.titleOverride,
    showCurrent: p.showCurrent, showTarget: p.showTarget, showPercent: p.showPercent,
    showProvider: false, showTime: false, showMessage: false, maxItems: 0,
    currency: undefined, metric: undefined, eventTypes: [], columns: 0, children: [],
    orientation: p.orientation, textAlign: p.textAlign, fontFamily: p.fontFamily,
    backgroundColor: p.backgroundColor, foregroundColor: p.foregroundColor, fillColor: p.fillColor, borderColor: p.borderColor,
    borderRadiusPx: p.borderRadiusPx, opacity: p.opacity,
  };
}

export function WidgetProfileManager({ goalId }: { goalId: string }) {
  const { t } = useTranslation('goals');
  const profilesQuery = useWidgetProfilesQuery(goalId);
  const deleteMutation = useDeleteWidgetProfileMutation();

  const [creating, setCreating] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<WidgetProfile | null>(null);

  const profiles = profilesQuery.data ?? [];

  return (
    <Panel>
      <PanelHeader
        title={t('widgets.title')}
        actions={
          <Button size="sm" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
            {t('widgets.add')}
          </Button>
        }
      />
      <div className="space-y-3 p-4 sm:p-5">
        {profiles.length === 0 && <p className="text-sm text-ink-muted">{t('widgets.emptyForGoal')}</p>}
        {profiles.map((profile) => (
          <WidgetProfileRow key={profile.id} profile={profile} onDeleteRequested={() => setDeleteTarget(profile)} />
        ))}
      </div>

      {creating && <CreateWidgetProfileModal goalId={goalId} onCancel={() => setCreating(false)} onCreated={() => setCreating(false)} />}

      {deleteTarget !== null && (
        <ConfirmDialog
          open
          title={t('widgets.deleteConfirmTitle')}
          message={t('widgets.deleteConfirmMessage', { name: deleteTarget.name })}
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

function CreateWidgetProfileModal({
  goalId,
  onCancel,
  onCreated,
}: {
  goalId: string;
  onCancel: () => void;
  onCreated: () => void;
}) {
  const { t } = useTranslation('goals');
  const createMutation = useCreateWidgetProfileMutation();
  const [name, setName] = useState('');

  return (
    <Modal
      open
      onClose={onCancel}
      title={t('widgets.createTitle')}
      dismissible={!createMutation.isPending}
      size="sm"
      footer={
        <>
          <Button onClick={onCancel} disabled={createMutation.isPending}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            disabled={createMutation.isPending || name.trim() === ''}
            onClick={() => createMutation.mutate({ ...defaultWidgetProfileDraft(goalId), name }, { onSuccess: onCreated })}
          >
            {t('common.create')}
          </Button>
        </>
      }
    >
      <FormField label={t('widgets.fields.name')}>
        {({ inputId }) => <TextInput id={inputId} value={name} onChange={(e) => setName(e.target.value)} autoFocus />}
      </FormField>
      {createMutation.isError && (
        <p role="alert" className="mt-2 text-sm text-status-error">
          {errorMessage(t, createMutation.error)}
        </p>
      )}
    </Modal>
  );
}

function WidgetProfileRow({
  profile,
  onDeleteRequested,
}: {
  profile: WidgetProfile;
  onDeleteRequested: () => void;
}) {
  const { t } = useTranslation('goals');
  const updateMutation = useUpdateWidgetProfileMutation();
  const rotateMutation = useRotateWidgetProfileSlugMutation();
  const [draft, setDraft] = useState<WidgetProfileInput>(draftFromProfile(profile));
  const [rotateConfirmOpen, setRotateConfirmOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const url = resolveWidgetUrl(profile.publicSlug);
  const formValid = isValidWidgetProfileFields(draft);
  const dirty = JSON.stringify(draft) !== JSON.stringify(draftFromProfile(profile));

  function commit(next: Partial<WidgetProfileInput>) {
    setDraft((d) => ({ ...d, ...next }));
  }

  return (
    <div className="rounded-lg border border-line p-3">
      <div className="flex items-center justify-between gap-2">
        <button type="button" className="min-w-0 flex-1 truncate text-left text-sm font-medium text-ink" onClick={() => setExpanded((v) => !v)}>
          {profile.name}
        </button>
        <IconButton label={t('common.delete')} icon={<Trash2 className="size-4" />} variant="danger" onClick={onDeleteRequested} />
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-2">
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

      {expanded && (
        <div className="mt-4 space-y-3 border-t border-line pt-3">
          <FormField label={t('widgets.fields.name')}>
            {({ inputId }) => <TextInput id={inputId} value={draft.name} onChange={(e) => commit({ name: e.target.value })} />}
          </FormField>
          <FormField label={t('widgets.fields.titleOverride')} hint={t('widgets.fields.titleOverrideHint')}>
            {({ inputId }) => (
              <TextInput id={inputId} value={draft.titleOverride ?? ''} onChange={(e) => commit({ titleOverride: e.target.value })} />
            )}
          </FormField>

          <ToggleSwitch label={t('widgets.fields.enabled')} checked={draft.enabled} onCheckedChange={(v) => commit({ enabled: v })} />
          <div className="flex flex-wrap gap-4">
            <ToggleSwitch label={t('widgets.fields.showCurrent')} checked={draft.showCurrent} onCheckedChange={(v) => commit({ showCurrent: v })} />
            <ToggleSwitch label={t('widgets.fields.showTarget')} checked={draft.showTarget} onCheckedChange={(v) => commit({ showTarget: v })} />
            <ToggleSwitch label={t('widgets.fields.showPercent')} checked={draft.showPercent} onCheckedChange={(v) => commit({ showPercent: v })} />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <FormField label={t('widgets.fields.orientation')}>
              {({ inputId }) => (
                <SelectInput
                  id={inputId}
                  options={WIDGET_ORIENTATIONS.map((v) => ({ value: v, label: t(`widgets.orientation.${v}`) }))}
                  value={draft.orientation}
                  onChange={(e) => commit({ orientation: e.target.value as WidgetOrientation })}
                />
              )}
            </FormField>
            <FormField label={t('widgets.fields.textAlign')}>
              {({ inputId }) => (
                <SelectInput
                  id={inputId}
                  options={WIDGET_TEXT_ALIGNS.map((v) => ({ value: v, label: t(`widgets.textAlign.${v}`) }))}
                  value={draft.textAlign}
                  onChange={(e) => commit({ textAlign: e.target.value as WidgetTextAlign })}
                />
              )}
            </FormField>
            <FormField label={t('widgets.fields.fontFamily')}>
              {({ inputId }) => (
                <SelectInput
                  id={inputId}
                  options={WIDGET_FONT_FAMILIES.map((v) => ({ value: v, label: t(`widgets.fontFamily.${v}`) }))}
                  value={draft.fontFamily}
                  onChange={(e) => commit({ fontFamily: e.target.value as WidgetFontFamily })}
                />
              )}
            </FormField>
          </div>

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
            <GoalWidgetRenderer
              snapshot={{
                revision: 1, kind: 'goal', goalKind: 'followers', title: draft.titleOverride || t('widgets.previewTitle'),
                current: 825, target: 1000, progressBasisPoints: 8250, completed: false,
                presentation: {
                  showCurrent: draft.showCurrent, showTarget: draft.showTarget, showPercent: draft.showPercent,
                  orientation: draft.orientation, textAlign: draft.textAlign, fontFamily: draft.fontFamily,
                  backgroundColor: draft.backgroundColor, foregroundColor: draft.foregroundColor,
                  fillColor: draft.fillColor, borderColor: draft.borderColor,
                  borderRadiusPx: draft.borderRadiusPx, opacity: draft.opacity,
                },
              }}
            />
          </div>

          <div className="flex items-center gap-2 border-t border-line pt-3">
            <Button
              variant="primary"
              disabled={!formValid || !dirty || updateMutation.isPending}
              onClick={() => updateMutation.mutate({ id: profile.id, input: draft })}
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
      )}

      <ConfirmDialog
        open={rotateConfirmOpen}
        title={t('widgets.rotateConfirmTitle')}
        message={t('widgets.rotateConfirmMessage')}
        confirmLabel={t('widgets.rotateAction')}
        busy={rotateMutation.isPending}
        onCancel={() => setRotateConfirmOpen(false)}
        onConfirm={() => rotateMutation.mutate(profile.id, { onSuccess: () => setRotateConfirmOpen(false) })}
      />
    </div>
  );
}
