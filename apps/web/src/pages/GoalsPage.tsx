import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { GoalManager } from '@/components/goals/GoalManager';
import { SupporterWidgetManager } from '@/components/goals/SupporterWidgetManager';
import { cn } from '@/lib/cn';

type Section = 'goals' | 'widgets' | 'dashboards';

/**
 * The Stage 18A/18B goals & widgets management page (`/goals`) - three
 * sections: Goals (goal list/create/edit/delete, Set current/Reset, and
 * each goal's own public widget), Widgets (every Stage 18B event-
 * derived widget kind), and Dashboards (bounded multi-widget
 * composition, docs/supporter-widgets.md §44). Deliberately still one
 * page/one nav item - the existing "Goals" destination widens
 * internally rather than growing new top-level nav entries.
 */
export function GoalsPage() {
  const { t } = useTranslation('goals');
  const [section, setSection] = useState<Section>('goals');

  const sections: Section[] = ['goals', 'widgets', 'dashboards'];

  return (
    <AppShell title={t('page.title')} description={t('page.description')}>
      <div className="mb-4 flex gap-1 rounded-lg border border-line bg-surface-sunken p-1" role="tablist">
        {sections.map((s) => (
          <button
            key={s}
            type="button"
            role="tab"
            aria-selected={section === s}
            onClick={() => setSection(s)}
            className={cn(
              'flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
              section === s ? 'bg-surface text-ink shadow-sm' : 'text-ink-muted hover:text-ink',
            )}
          >
            {t(`sections.${s}`)}
          </button>
        ))}
      </div>

      {section === 'goals' && <GoalManager />}
      {section === 'widgets' && <SupporterWidgetManager dashboardsOnly={false} />}
      {section === 'dashboards' && <SupporterWidgetManager dashboardsOnly />}
    </AppShell>
  );
}
