import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { ProfileManager } from '@/components/alerts/ProfileManager';

/**
 * The Stage 12A alert management page (`/alerts`) - profile list/
 * create/rename/delete/rotate-URL, live queue state and controls, and
 * the rule list/editor for whichever profile is selected. Deliberately
 * its own page, not folded into Chat, Engagement, or Automation (Part
 * 35).
 */
export function AlertsPage() {
  const { t } = useTranslation('alerts');

  return (
    <AppShell title={t('page.title')} description={t('page.description')}>
      <ProfileManager />
    </AppShell>
  );
}
