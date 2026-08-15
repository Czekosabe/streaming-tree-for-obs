import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { AudioQueuePanel } from '@/components/audio/AudioQueuePanel';
import { AudioSettingsPanel } from '@/components/audio/AudioSettingsPanel';

/**
 * The Stage 17A audio/TTS management page (`/audio`) - provider/voice
 * settings, event/provider/source filters, text preprocessing,
 * cooldowns, the Browser Source URL, and the live runtime queue/
 * status/pending-approval/Test Speak view. Deliberately its own page,
 * mirroring AlertsPage's own reasoning for not folding into Chat or
 * Engagement.
 */
export function AudioPage() {
  const { t } = useTranslation('audio');

  return (
    <AppShell title={t('page.title')} description={t('page.description')}>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <AudioSettingsPanel />
        <AudioQueuePanel />
      </div>
    </AppShell>
  );
}
