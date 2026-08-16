import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { GoalManager } from '@/components/goals/GoalManager';

/**
 * The Stage 18A goals management page (`/goals`) - goal list/create/
 * edit/delete, Set current/Reset, and each goal's own public widget
 * profiles. Deliberately its own page, mirroring AlertsPage/AudioPage's
 * identical "thin AppShell wrapper around the real content component"
 * shape.
 */
export function GoalsPage() {
  const { t } = useTranslation('goals');

  return (
    <AppShell title={t('page.title')} description={t('page.description')}>
      <GoalManager />
    </AppShell>
  );
}
