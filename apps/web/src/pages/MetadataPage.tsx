import { Loader2, RefreshCw, SlidersHorizontal, Tv } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { AppShell } from '@/components/layout/AppShell';
import { MetadataEditor } from '@/components/metadata/MetadataEditor';
import { Button } from '@/components/ui/Button';
import { Panel, PanelBody } from '@/components/ui/Panel';
import { useActiveMetadataSelection, usePlatformsQuery } from '@/hooks/use-platforms';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

type MetadataPageLocationState = { platformId?: string };

/**
 * Canonical detailed stream-metadata surface (Stage 20E "complete Platforms/
 * Metadata"). Renders the exact same `MetadataEditor` the Dashboard already
 * embeds inline - same tabs, same form, same validation, same Save/Reset/
 * Save-as-preset actions, same Manage/Apply-preset dialogs - so metadata
 * edited here is the one canonical destination metadata state, never a
 * second copy that could drift from Dashboard.
 *
 * `PlatformsPage`'s "Edit metadata" action on a destination card navigates
 * here with that destination's id (`location.state.platformId`), so the
 * hop between the two pages lands on the right tab instead of always
 * resetting to the first destination.
 */
export function MetadataPage() {
  const { t } = useTranslation(['metadata', 'dashboard', 'errors', 'pages']);
  const tErrors = useTranslation('errors').t;
  const navigate = useNavigate();
  const location = useLocation();

  const platformsQuery = usePlatformsQuery();
  const [dirty, setDirty] = useState(false);

  const platforms: ConfiguredPlatform[] = useMemo(
    () => platformsQuery.data ?? [],
    [platformsQuery.data],
  );

  const requestedId = (location.state as MetadataPageLocationState | null)?.platformId ?? null;
  const { activeId, setActiveId } = useActiveMetadataSelection(platforms, requestedId);

  return (
    <AppShell
      title={t('pages:metadata.title')}
      description={t('pages:metadata.description')}
      actions={
        <Button icon={<SlidersHorizontal className="size-4" />} onClick={() => void navigate('/platforms')}>
          {t('metadata:managementPage.manageDestinations')}
        </Button>
      }
    >
      {platformsQuery.isPending && (
        <Panel>
          <PanelBody className="flex items-center justify-center gap-2 py-12 text-sm text-ink-muted">
            <Loader2 aria-hidden="true" className="size-4 animate-spin" />
            {t('dashboard:states.loading')}
          </PanelBody>
        </Panel>
      )}

      {platformsQuery.isError && (
        <Panel>
          <PanelBody className="space-y-3 py-10 text-center">
            <p className="text-sm font-medium text-status-error">{t('dashboard:states.loadFailedTitle')}</p>
            <p className="mx-auto max-w-md text-xs leading-relaxed text-ink-muted">
              {resolveApiErrorMessage(tErrors, platformsQuery.error)}
            </p>
            <Button
              variant="primary"
              icon={<RefreshCw className="size-3.5" />}
              onClick={() => void platformsQuery.refetch()}
            >
              {t('dashboard:states.retry')}
            </Button>
          </PanelBody>
        </Panel>
      )}

      {platformsQuery.isSuccess && platforms.length === 0 && (
        <Panel>
          <PanelBody className="flex flex-col items-center gap-4 py-12 text-center">
            <span
              aria-hidden="true"
              className="flex size-12 items-center justify-center rounded-xl border border-line bg-surface-raised text-accent-soft"
            >
              <Tv className="size-5" />
            </span>
            <div className="space-y-1">
              <p className="text-sm font-medium text-ink">{t('metadata:managementPage.emptyTitle')}</p>
              <p className="mx-auto max-w-sm text-xs leading-relaxed text-ink-muted">
                {t('metadata:managementPage.emptyMessage')}
              </p>
            </div>
            <Button variant="primary" onClick={() => void navigate('/platforms')}>
              {t('metadata:managementPage.goToPlatforms')}
            </Button>
          </PanelBody>
        </Panel>
      )}

      {platformsQuery.isSuccess && platforms.length > 0 && (
        <MetadataEditor
          platforms={platforms}
          activeId={activeId}
          onSelect={setActiveId}
          dirty={dirty}
          onDirtyChange={setDirty}
        />
      )}
    </AppShell>
  );
}
