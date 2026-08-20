import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import type { ChatOverlayEditableFields } from '@/api/chat-overlay-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useChatOverlayQuery, useReplaceChatOverlayMutation } from '@/hooks/use-chat-overlay';
import { useVisualDesignQuery } from '@/hooks/use-visual-design';
import { editableFieldsEqual, toEditableFields } from '@/models/chat-overlay-config';

import { OverlayAccountsPanel } from './OverlayAccountsPanel';
import { OverlayActivityTypesPanel } from './OverlayActivityTypesPanel';
import { OverlayBlockedTermsPanel } from './OverlayBlockedTermsPanel';
import { OverlayHiddenUsersPanel } from './OverlayHiddenUsersPanel';
import { OverlayPreviewPanel } from './OverlayPreviewPanel';
import { OverlaySettingsForm } from './OverlaySettingsForm';
import { OverlaySetupPanel } from './OverlaySetupPanel';
import { OverlayUrlPanel } from './OverlayUrlPanel';
import { RemoteOverlayPanel } from './RemoteOverlayPanel';

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
  const designQuery = useVisualDesignQuery('chat-overlays', overlayId);
  const designActive = designQuery.data?.persisted === true;

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
      <RemoteOverlayPanel domain="chat-overlay" localSlug={profileQuery.data.publicSlug} />
      <DesignerLinkBanner overlayId={overlayId} />
      <OverlayPreviewPanel draft={draft} />

      <Panel>
        <PanelHeader title={t('settings.title')} />
        <PanelBody>
          <OverlaySettingsForm draft={draft} onChange={setDraft} designActive={designActive} />
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

/** Stage 13B task Part 20/31 (mirrors alerts' own RuleManager.tsx
 * DesignerLinkBanner exactly): the overlay editor must clearly state
 * whether presentation is controlled by the Designer, and always offer
 * a link to it - never two independently-editable panels that appear
 * to control the same real output. Every field in `OverlaySettingsForm`
 * below (behavior/filter/lifecycle settings: account selection, hidden
 * users, blocked terms, bot/command filtering, activity-type selection,
 * maxVisibleItems, message lifetime, stack direction) remains real,
 * always-editable overlay behavior regardless of design mode - only the
 * *visual* fields there (font, colors, bubble, animations) become
 * legacy-fallback-only once a design is saved, per docs/visual-designs.md
 * §23. */
function DesignerLinkBanner({ overlayId }: { overlayId: string }) {
  const { t } = useTranslation('chatOverlayDesigner');
  const navigate = useNavigate();
  const designQuery = useVisualDesignQuery('chat-overlays', overlayId);
  const persisted = designQuery.data?.persisted === true;

  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-line bg-surface-sunken p-3">
      <p className="text-sm font-medium text-ink">
        {persisted ? t('designerLink.controlledByDesigner') : t('designerLink.legacyDescription')}
      </p>
      <Button variant="secondary" onClick={() => navigate(`/overlays/${overlayId}/designer`)}>
        {t('designerLink.openDesigner')}
      </Button>
    </div>
  );
}
