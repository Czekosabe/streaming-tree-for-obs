import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { OperatorChatPreferences } from '@/api/operator-chat-schemas';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import type { ChatKey } from '@/models/operator-chat-presentation';

type ToggleField = keyof OperatorChatPreferences;

const TOGGLE_ORDER: { field: ToggleField; labelKey: ChatKey }[] = [
  { field: 'showPlatformIcon', labelKey: 'settings.showPlatformIcon' },
  { field: 'showPlatformName', labelKey: 'settings.showPlatformName' },
  { field: 'showAccountLabel', labelKey: 'settings.showAccountLabel' },
  { field: 'showBadges', labelKey: 'settings.showBadges' },
  { field: 'showTimestamps', labelKey: 'settings.showTimestamps' },
  { field: 'showActivityEvents', labelKey: 'settings.showActivityEvents' },
  { field: 'showDeletedMessages', labelKey: 'settings.showDeletedMessages' },
  { field: 'hideCommandMessages', labelKey: 'settings.hideCommandMessages' },
  { field: 'compactMode', labelKey: 'settings.compactMode' },
];

type ChatSettingsPanelProps = {
  open: boolean;
  onClose: () => void;
  preferences: OperatorChatPreferences;
  /** Applied immediately as the operator toggles, so filtering feels
   * instant - persistence to the backend happens only on Save (Part 3 /
   * Part 19's "persistent prefs save deliberately"). */
  onPreview: (next: OperatorChatPreferences) => void;
  onSave: (next: OperatorChatPreferences) => void;
  saving: boolean;
};

export function ChatSettingsPanel({
  open,
  onClose,
  preferences,
  onPreview,
  onSave,
  saving,
}: ChatSettingsPanelProps) {
  const { t } = useTranslation(['chat', 'common']);
  const [draft, setDraft] = useState(preferences);

  useEffect(() => {
    if (open) setDraft(preferences);
  }, [open, preferences]);

  function setField(field: ToggleField, value: boolean) {
    const next = { ...draft, [field]: value };
    setDraft(next);
    onPreview(next);
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('settings.title')}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common:actions.cancel')}
          </Button>
          <Button variant="primary" disabled={saving} onClick={() => onSave(draft)}>
            {t('chat:settings.saveAction')}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        {TOGGLE_ORDER.map(({ field, labelKey }) => (
          <ToggleSwitch
            key={field}
            label={t(labelKey)}
            checked={draft[field]}
            onCheckedChange={(checked) => setField(field, checked)}
          />
        ))}
      </div>
    </Modal>
  );
}
