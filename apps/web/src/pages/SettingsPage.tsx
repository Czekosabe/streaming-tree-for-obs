import { ChevronRight, Info } from 'lucide-react';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

import { AppShell } from '@/components/layout/AppShell';
import { OnboardingReopenCard } from '@/components/onboarding/OnboardingReopenCard';
import { BackupRestorePanel } from '@/components/settings/BackupRestorePanel';
import { ConnectedAccountsPanel } from '@/components/settings/ConnectedAccountsPanel';
import { RemoteIngestPanel } from '@/components/settings/RemoteIngestPanel';
import { YouTubeAccountsPanel } from '@/components/settings/YouTubeAccountsPanel';
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';

/**
 * Settings: interface language and connected provider accounts.
 *
 * Real, working sections - not a placeholder. What remains planned here
 * (ingest tuning, managed-binary paths, backend address, a generic
 * credential-store view) is listed as such rather than implied to exist.
 */
export function SettingsPage() {
  const { t } = useTranslation(['pages', 'about']);
  const labelId = useId();

  return (
    <AppShell title={t('pages:settings.title')} description={t('pages:settings.description')}>
      <div className="mx-auto max-w-2xl space-y-4">
        <Panel>
          <PanelHeader
            title={t('pages:settings.language.heading')}
            description={t('pages:settings.language.description')}
          />
          <PanelBody className="space-y-2">
            <span id={labelId} className="sr-only">
              {t('pages:settings.language.heading')}
            </span>
            <LanguageSwitcher labelledBy={labelId} className="w-48 max-w-full" />
            <p className="text-[11px] text-ink-faint">{t('pages:settings.language.note')}</p>
          </PanelBody>
        </Panel>

        <OnboardingReopenCard />

        <ConnectedAccountsPanel />
        <YouTubeAccountsPanel />
        <RemoteIngestPanel />
        <BackupRestorePanel />

        <Panel>
          <PanelBody>
            <Link
              to="/settings/about"
              className="flex items-center justify-between gap-3 rounded-lg -m-1 p-1 transition-colors hover:bg-surface-hover"
            >
              <span className="flex items-center gap-3">
                <span
                  aria-hidden="true"
                  className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-raised text-accent-soft"
                >
                  <Info className="size-4" />
                </span>
                <span>
                  <span className="block text-sm font-semibold text-ink">
                    {t('about:settingsCard.heading')}
                  </span>
                  <span className="block text-xs text-ink-muted">
                    {t('about:settingsCard.description')}
                  </span>
                </span>
              </span>
              <ChevronRight aria-hidden="true" className="size-4 shrink-0 text-ink-faint" />
            </Link>
          </PanelBody>
        </Panel>
      </div>
    </AppShell>
  );
}
