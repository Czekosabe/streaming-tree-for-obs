import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChatOverlayEditableFields } from '@/api/chat-overlay-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useChatOverlayQuery, useReplaceChatOverlayMutation } from '@/hooks/use-chat-overlay';
import { editableFieldsEqual, toEditableFields } from '@/models/chat-overlay-config';

import { OverlayAccountsPanel } from './OverlayAccountsPanel';
import { OverlayActivityTypesPanel } from './OverlayActivityTypesPanel';
import { OverlayBlockedTermsPanel } from './OverlayBlockedTermsPanel';
import { OverlayHiddenUsersPanel } from './OverlayHiddenUsersPanel';
import { OverlayPreviewPanel } from './OverlayPreviewPanel';
import { OverlaySettingsForm } from './OverlaySettingsForm';
import { OverlaySetupPanel } from './OverlaySetupPanel';
import { OverlayUrlPanel } from './OverlayUrlPanel';

/**
 * The full editor for one overlay profile: identity (name/enabled), the
 * visual/filtering settings form, and its four child lists - all wrapped
 * in a single draft-then-explicit-save state machine (Part 19).
 * `draft` starts as a copy of the loaded profile's editable fields and is
 * never sent anywhere until Save; the preview panel renders it
 * immediately, the real public overlay only after Save succeeds (a
 * successful save also triggers the runtime rebuild - see
 * internal/httpapi's own `rebuildChatOverlayRuntime`).
 */
export function OverlayEditor({ overlayId }: { overlayId: string }) {
  const { t } = useTranslation('overlays');
  const profileQuery = useChatOverlayQuery(overlayId);
  const replace = useReplaceChatOverlayMutation(overlayId);

  const [draft, setDraft] = useState<ChatOverlayEditableFields | null>(null);
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  const [justSaved, setJustSaved] = useState(false);

  // Re-seed the draft whenever a different overlay is selected, or on
  // first load - never overwrite in-progress edits on a background
  // refetch of the same overlay.
  useEffect(() => {
    setDraft(profileQuery.data === undefined ? null : toEditableFields(profileQuery.data));
    setJustSaved(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- re-seed only on overlay change, not on every background refetch
  }, [overlayId, profileQuery.data?.id]);

  if (profileQuery.data === undefined || draft === null) {
    return null;
  }

  const savedFields = toEditableFields(profileQuery.data);
  const hasUnsavedChanges = !editableFieldsEqual(draft, savedFields);

  function handleSave() {
    if (draft === null) return;
    replace.mutate(draft, {
      onSuccess: () => {
        setJustSaved(true);
        window.setTimeout(() => setJustSaved(false), 2000);
      },
    });
  }

  function discardChanges() {
    setDraft(savedFields);
    setConfirmingDiscard(false);
  }

  return (
    <div className="space-y-4">
      <Panel>
        <PanelHeader
          title={draft.name === '' ? t('page.title') : draft.name}
          actions={
            <div className="flex items-center gap-2">
              {hasUnsavedChanges && <span className="text-xs text-status-warning">{t('detail.unsavedChanges')}</span>}
              {justSaved && !hasUnsavedChanges && <span className="text-xs text-status-live">{t('detail.saved')}</span>}
              <Button
                variant="ghost"
                disabled={!hasUnsavedChanges || replace.isPending}
                onClick={() => setConfirmingDiscard(true)}
              >
                {t('detail.discardConfirmAction')}
              </Button>
              <Button variant="primary" disabled={!hasUnsavedChanges || replace.isPending} onClick={handleSave}>
                {t('detail.saveAction')}
              </Button>
            </div>
          }
        />
        <PanelBody className="grid grid-cols-1 gap-4 sm:grid-cols-[2fr_auto]">
          <div className="max-w-sm space-y-1.5">
            <label className="text-xs font-medium text-ink-muted" htmlFor="overlay-name">
              {t('list.createPlaceholder')}
            </label>
            <TextInput
              id="overlay-name"
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
            />
          </div>
          <ToggleSwitch
            label={t('detail.enabledToggle')}
            checked={draft.enabled}
            onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
          />
        </PanelBody>
      </Panel>

      <OverlayUrlPanel overlayId={overlayId} publicSlug={profileQuery.data.publicSlug} />
      <OverlayPreviewPanel draft={draft} />

      <Panel>
        <PanelHeader title={t('settings.title')} />
        <PanelBody>
          <OverlaySettingsForm draft={draft} onChange={setDraft} />
        </PanelBody>
      </Panel>

      <OverlayAccountsPanel overlayId={overlayId} />
      <OverlayActivityTypesPanel overlayId={overlayId} />
      <OverlayHiddenUsersPanel overlayId={overlayId} />
      <OverlayBlockedTermsPanel overlayId={overlayId} />
      <OverlaySetupPanel />

      <ConfirmDialog
        open={confirmingDiscard}
        title={t('detail.discardConfirmTitle')}
        message={t('detail.discardConfirmBody')}
        confirmLabel={t('detail.discardConfirmAction')}
        destructive
        onCancel={() => setConfirmingDiscard(false)}
        onConfirm={discardChanges}
      />
    </div>
  );
}
