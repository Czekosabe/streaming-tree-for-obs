import type { TFunction } from 'i18next';
import { Check, Copy, ExternalLink, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { AlertProfile, AlertProfileInput } from '@/api/alerts-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { RemoteOverlayPanel } from '@/components/overlays/RemoteOverlayPanel';
import { Panel, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  useAlertProfilesQuery,
  useCreateAlertProfileMutation,
  useDeleteAlertProfileMutation,
  useRotateAlertProfileSlugMutation,
  useUpdateAlertProfileMutation,
} from '@/hooks/use-alerts';
import { ApiError } from '@/lib/api-client';
import { cn } from '@/lib/cn';
import {
  ALERT_LANGUAGES,
  ALERT_POSITIONS,
  ALERT_TEXT_ALIGNS,
  ALERT_THEMES,
  isValidAlertName,
  isValidMaxQueueItems,
  isValidMaximumQueueAgeSeconds,
} from '@/models/alerts';

import { RuleManager } from './RuleManager';
import { QueuePanel } from './QueuePanel';

function errorMessage(t: TFunction<'alerts'>, error: unknown): string {
  if (error instanceof ApiError && error.code !== null) {
    const key = `errors.${error.code}` as never;
    const translated = t(key, { defaultValue: '' });
    return translated === '' ? t('errors.generic') : translated;
  }
  return t('errors.generic');
}

function resolveBrowserSourceUrl(publicSlug: string): string {
  return `${window.location.origin}/overlay/alerts/${publicSlug}`;
}

function draftFromProfile(profile: AlertProfile): AlertProfileInput {
  return {
    name: profile.name, enabled: profile.enabled, language: profile.language,
    theme: profile.theme, position: profile.position, textAlign: profile.textAlign,
    maxQueueItems: profile.maxQueueItems, maximumQueueAgeSeconds: profile.maximumQueueAgeSeconds,
  };
}

export function ProfileManager() {
  const { t } = useTranslation('alerts');
  const profilesQuery = useAlertProfilesQuery();
  const deleteMutation = useDeleteAlertProfileMutation();

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<AlertProfile | null>(null);

  const profiles = profilesQuery.data ?? [];
  const selected = profiles.find((p) => p.id === selectedId) ?? null;

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
          {profiles.length === 0 ? (
            <p className="p-3 text-sm text-ink-muted">{t('profiles.empty')}</p>
          ) : (
            <ul className="space-y-1">
              {profiles.map((profile) => (
                <li key={profile.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(profile.id)}
                    aria-current={profile.id === selectedId}
                    className={cn(
                      'flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm transition-colors',
                      profile.id === selectedId ? 'bg-accent/15 text-ink' : 'text-ink-muted hover:bg-surface-hover',
                    )}
                  >
                    <span className="truncate">{profile.name}</span>
                    {!profile.enabled && (
                      <span className="shrink-0 rounded-full border border-line px-1.5 py-0.5 text-[10px] text-ink-faint">
                        {t('profiles.fields.enabled')}: off
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
          <ProfileEditor
            key={selected.id}
            profile={selected}
            onDeleteRequested={() => setDeleteTarget(selected)}
          />
          <QueuePanel profileId={selected.id} />
          <RuleManager profileId={selected.id} />
        </div>
      )}

      {creating && (
        <CreateProfileModal
          onCancel={() => setCreating(false)}
          onCreated={(profile) => {
            setCreating(false);
            setSelectedId(profile.id);
          }}
        />
      )}

      {deleteTarget !== null && (
        <ConfirmDialog
          open
          title={t('profiles.deleteConfirmTitle')}
          message={t('profiles.deleteConfirmMessage', { name: deleteTarget.name })}
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
    </div>
  );
}

/**
 * A dedicated component with its own local `name` state - deliberately
 * NOT lifted into ProfileManager's own state (see
 * docs/progress.md's Stage 12A frontend entry for why): keeping the
 * draft here means typing never re-renders ProfileManager itself, so
 * the `Modal`'s `onClose` prop stays referentially stable across
 * keystrokes. If it were recreated every render (an inline arrow
 * function closing over a parent-held `name` state), Modal's own
 * focus-management effect - which depends on `onClose` - would re-run
 * on every keystroke and steal focus back to the panel's first
 * focusable element mid-type. Mirrors
 * components/automation/ScheduleManager.tsx's own `ScheduleFormModal`,
 * which already keeps its draft locally for the same reason.
 */
function CreateProfileModal({
  onCancel,
  onCreated,
}: {
  onCancel: () => void;
  onCreated: (profile: AlertProfile) => void;
}) {
  const { t } = useTranslation('alerts');
  const createMutation = useCreateAlertProfileMutation();
  const [name, setName] = useState('');

  return (
    <Modal
      open
      onClose={onCancel}
      title={t('profiles.createTitle')}
      dismissible={!createMutation.isPending}
      size="sm"
      footer={
        <>
          <Button onClick={onCancel} disabled={createMutation.isPending}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            disabled={createMutation.isPending || !isValidAlertName(name)}
            onClick={() => createMutation.mutate(name, { onSuccess: onCreated })}
          >
            {t('common.create')}
          </Button>
        </>
      }
    >
      <FormField label={t('profiles.fields.name')}>
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

function ProfileEditor({
  profile,
  onDeleteRequested,
}: {
  profile: AlertProfile;
  onDeleteRequested: () => void;
}) {
  const { t } = useTranslation('alerts');
  const updateMutation = useUpdateAlertProfileMutation();
  const rotateMutation = useRotateAlertProfileSlugMutation();
  const [draft, setDraft] = useState<AlertProfileInput>(draftFromProfile(profile));
  const [rotateConfirmOpen, setRotateConfirmOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const url = resolveBrowserSourceUrl(profile.publicSlug);
  const nameValid = isValidAlertName(draft.name);
  const maxQueueValid = isValidMaxQueueItems(draft.maxQueueItems);
  const maxAgeValid = isValidMaximumQueueAgeSeconds(draft.maximumQueueAgeSeconds);
  const formValid = nameValid && maxQueueValid && maxAgeValid;
  const dirty = JSON.stringify(draft) !== JSON.stringify(draftFromProfile(profile));

  return (
    <>
    <Panel>
      <PanelHeader
        title={profile.name}
        actions={
          <IconButton label={t('common.delete')} icon={<Trash2 className="size-4" />} variant="danger" onClick={onDeleteRequested} />
        }
      />
      <div className="space-y-4 p-4 sm:p-5">
        <div>
          <p className="text-xs font-medium text-ink-muted">{t('profiles.browserSourceUrl')}</p>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded-lg border border-line bg-surface-sunken px-2 py-1.5 text-xs text-ink">
              {url}
            </code>
            <Button
              size="sm"
              icon={copied ? <Check className="size-4" /> : <Copy className="size-4" />}
              onClick={() => {
                void navigator.clipboard.writeText(url);
                setCopied(true);
                window.setTimeout(() => setCopied(false), 2000);
              }}
            >
              {copied ? t('profiles.copied') : t('profiles.copyUrl')}
            </Button>
            <Button size="sm" icon={<ExternalLink className="size-4" />} onClick={() => window.open(url, '_blank', 'noopener,noreferrer')}>
              {t('profiles.openUrl')}
            </Button>
            <Button size="sm" icon={<RefreshCw className="size-4" />} onClick={() => setRotateConfirmOpen(true)}>
              {t('profiles.rotateAction')}
            </Button>
          </div>
        </div>

        <FormField label={t('profiles.fields.name')}>
          {({ inputId }) => (
            <TextInput id={inputId} value={draft.name} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} />
          )}
        </FormField>

        <ToggleSwitch
          label={t('profiles.fields.enabled')}
          description={t('profiles.fields.enabledHint')}
          checked={draft.enabled}
          onCheckedChange={(v) => setDraft((d) => ({ ...d, enabled: v }))}
        />

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FormField label={t('profiles.fields.language')} hint={t('profiles.fields.languageHint')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={ALERT_LANGUAGES.map((v) => ({ value: v, label: v.toUpperCase() }))}
                value={draft.language}
                onChange={(e) => setDraft((d) => ({ ...d, language: e.target.value as AlertProfileInput['language'] }))}
              />
            )}
          </FormField>
          <FormField label={t('profiles.fields.theme')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={ALERT_THEMES.map((v) => ({ value: v, label: t(`profiles.theme.${v}`) }))}
                value={draft.theme}
                onChange={(e) => setDraft((d) => ({ ...d, theme: e.target.value as AlertProfileInput['theme'] }))}
              />
            )}
          </FormField>
          <FormField label={t('profiles.fields.position')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={ALERT_POSITIONS.map((v) => ({ value: v, label: t(`profiles.position.${v}`) }))}
                value={draft.position}
                onChange={(e) => setDraft((d) => ({ ...d, position: e.target.value as AlertProfileInput['position'] }))}
              />
            )}
          </FormField>
          <FormField label={t('profiles.fields.textAlign')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={ALERT_TEXT_ALIGNS.map((v) => ({ value: v, label: t(`profiles.textAlign.${v}`) }))}
                value={draft.textAlign}
                onChange={(e) => setDraft((d) => ({ ...d, textAlign: e.target.value as AlertProfileInput['textAlign'] }))}
              />
            )}
          </FormField>
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FormField label={t('profiles.fields.maxQueueItems')} hint={t('profiles.fields.maxQueueItemsHint')}>
            {({ inputId }) => (
              <TextInput
                id={inputId}
                type="number"
                value={draft.maxQueueItems}
                onChange={(e) => setDraft((d) => ({ ...d, maxQueueItems: Number(e.target.value) }))}
              />
            )}
          </FormField>
          <FormField label={t('profiles.fields.maximumQueueAgeSeconds')} hint={t('profiles.fields.maximumQueueAgeSecondsHint')}>
            {({ inputId }) => (
              <TextInput
                id={inputId}
                type="number"
                value={draft.maximumQueueAgeSeconds}
                onChange={(e) => setDraft((d) => ({ ...d, maximumQueueAgeSeconds: Number(e.target.value) }))}
              />
            )}
          </FormField>
        </div>

        {updateMutation.isError && (
          <p role="alert" className="text-sm text-status-error">
            {errorMessage(t, updateMutation.error)}
          </p>
        )}

        <div className="flex justify-end">
          <Button
            variant="primary"
            disabled={!formValid || !dirty || updateMutation.isPending}
            onClick={() => updateMutation.mutate({ id: profile.id, input: draft })}
          >
            {t('common.save')}
          </Button>
        </div>
      </div>

      {rotateConfirmOpen && (
        <ConfirmDialog
          open
          title={t('profiles.rotateConfirmTitle')}
          message={t('profiles.rotateConfirmMessage', { name: profile.name })}
          confirmLabel={t('profiles.rotateAction')}
          busy={rotateMutation.isPending}
          onCancel={() => setRotateConfirmOpen(false)}
          onConfirm={() => {
            rotateMutation.mutate(profile.id, { onSuccess: () => setRotateConfirmOpen(false) });
          }}
        />
      )}
    </Panel>
    <RemoteOverlayPanel domain="alert-profile" localSlug={profile.publicSlug} />
    </>
  );
}
