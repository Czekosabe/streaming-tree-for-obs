import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { CommandManager } from '@/components/automation/CommandManager';
import { ScheduleManager } from '@/components/automation/ScheduleManager';
import { cn } from '@/lib/cn';

type Tab = 'schedules' | 'commands';

export function AutomationPage() {
  const { t } = useTranslation('automation');
  const [tab, setTab] = useState<Tab>('schedules');

  return (
    <AppShell title={t('page.title')} description={t('page.description')}>
      <div role="tablist" aria-label={t('page.title')} className="mb-4 flex gap-1 border-b border-line">
        {(['schedules', 'commands'] as const).map((value) => (
          <button
            key={value}
            type="button"
            role="tab"
            aria-selected={tab === value}
            className={cn(
              'border-b-2 px-3 py-2 text-sm font-medium transition-colors',
              tab === value
                ? 'border-accent text-ink'
                : 'border-transparent text-ink-muted hover:text-ink',
            )}
            onClick={() => setTab(value)}
          >
            {t(`tabs.${value}`)}
          </button>
        ))}
      </div>

      <div role="tabpanel">
        {tab === 'schedules' ? <ScheduleManager /> : <CommandManager />}
      </div>
    </AppShell>
  );
}
