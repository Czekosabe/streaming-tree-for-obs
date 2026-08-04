import { FileText, Settings, SlidersHorizontal, Tv } from 'lucide-react';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';

import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';

import { PlaceholderPage } from './PlaceholderPage';

/**
 * Placeholder routes.
 *
 * Grouped in one file because each is a single call with different copy; they
 * will be replaced by real pages one stage at a time.
 */

export function PlatformsPage() {
  return (
    <PlaceholderPage
      titleKey="platforms.title"
      descriptionKey="platforms.description"
      icon={Tv}
      plannedKeys={[
        'platforms.planned.manageBranches',
        'platforms.planned.oauth',
        'platforms.planned.credentials',
        'platforms.planned.encoding',
      ]}
    />
  );
}

// StreamsPage is no longer a placeholder: it shows the real local ingest state
// and lives in its own file, `StreamsPage.tsx`.

export function MetadataPage() {
  return (
    <PlaceholderPage
      titleKey="metadata.title"
      descriptionKey="metadata.description"
      icon={SlidersHorizontal}
      plannedKeys={[
        'metadata.planned.presets',
        'metadata.planned.overrides',
        'metadata.planned.push',
        'metadata.planned.history',
      ]}
    />
  );
}

/**
 * Settings is the one placeholder route with a working section: the interface
 * language. It is genuinely implemented, so it is shown as a real panel above
 * the "planned" card rather than being listed as a future feature.
 */
export function SettingsPage() {
  const { t } = useTranslation('pages');
  const labelId = useId();

  return (
    <PlaceholderPage
      titleKey="settings.title"
      descriptionKey="settings.description"
      icon={Settings}
      plannedKeys={[
        'settings.planned.ingest',
        'settings.planned.binaries',
        'settings.planned.backendAddress',
        'settings.planned.credentialStore',
      ]}
    >
      <Panel>
        <PanelHeader
          title={t('settings.language.heading')}
          description={t('settings.language.description')}
        />
        <PanelBody className="space-y-2">
          <span id={labelId} className="sr-only">
            {t('settings.language.heading')}
          </span>
          <LanguageSwitcher labelledBy={labelId} className="w-48 max-w-full" />
          <p className="text-[11px] text-ink-faint">{t('settings.language.note')}</p>
        </PanelBody>
      </Panel>
    </PlaceholderPage>
  );
}

export function LogsPage() {
  return (
    <PlaceholderPage
      titleKey="logs.title"
      descriptionKey="logs.description"
      icon={FileText}
      plannedKeys={[
        'logs.planned.backendLogs',
        'logs.planned.ffmpegOutput',
        'logs.planned.diagnosticBundle',
      ]}
    />
  );
}
