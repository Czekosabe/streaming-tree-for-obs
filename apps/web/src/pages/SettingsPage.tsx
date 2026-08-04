import { useId } from 'react';
import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { ConnectedAccountsPanel } from '@/components/settings/ConnectedAccountsPanel';
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
  const { t } = useTranslation(['pages']);
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

        <ConnectedAccountsPanel />
      </div>
    </AppShell>
  );
}
