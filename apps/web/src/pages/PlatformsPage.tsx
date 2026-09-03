import { Loader2, Plus, RefreshCw, Tv, Users } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { AppShell } from '@/components/layout/AppShell';
import { AddPlatformDialog } from '@/components/platforms/AddPlatformDialog';
import { PlatformGrid } from '@/components/platforms/PlatformGrid';
import { PlatformSettingsDialog } from '@/components/platforms/PlatformSettingsDialog';
import { Button } from '@/components/ui/Button';
import { Panel, PanelBody } from '@/components/ui/Panel';
import { useBranchRuntimeQuery } from '@/hooks/use-branches';
import { usePlatformsConfiguredCount } from '@/hooks/use-credentials';
import { usePlatformDefinitionsQuery, usePlatformsQuery } from '@/hooks/use-platforms';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

/**
 * Canonical detailed destination-management surface (Stage 20E "complete
 * Platforms/Metadata"). Reuses exactly the same query hooks, `PlatformGrid`,
 * `AddPlatformDialog` and `PlatformSettingsDialog` the Dashboard already
 * uses for its own platform cards - there is no second, competing
 * destination-management implementation, so a change made here is the same
 * change Dashboard sees, and vice versa.
 *
 * Dashboard stays the quick operational overview; this is where an operator
 * goes to see every destination at once, understand its real configuration/
 * credential/enabled/active state, and reach every management action
 * without hunting through Dashboard's cards.
 */
export function PlatformsPage() {
  const { t } = useTranslation(['platforms', 'dashboard', 'errors', 'pages']);
  const tErrors = useTranslation('errors').t;
  const navigate = useNavigate();

  const platformsQuery = usePlatformsQuery();
  const definitionsQuery = usePlatformDefinitionsQuery();
  const branchesQuery = useBranchRuntimeQuery();

  const [addOpen, setAddOpen] = useState(false);
  const [settingsId, setSettingsId] = useState<string | null>(null);

  const platforms: ConfiguredPlatform[] = useMemo(
    () => platformsQuery.data ?? [],
    [platformsQuery.data],
  );
  const platformIds = useMemo(() => platforms.map((platform) => platform.id), [platforms]);

  // Real credential-configured count (a stored stream key), never merely how
  // many destination cards exist - the same distinction Stage 20E's
  // onboarding-summary fix established; see hooks/use-credentials.ts's own
  // doc comment.
  const { configuredCount } = usePlatformsConfiguredCount(platformIds);
  const enabledCount = platforms.filter((platform) => platform.enabled).length;
  const activeCount = (branchesQuery.data ?? []).filter((branch) => branch.state === 'live').length;

  const settingsPlatform = platforms.find((platform) => platform.id === settingsId) ?? null;

  // Hands the destination off to the Metadata page, pre-selected there -
  // metadata editing itself now lives on /metadata (see MetadataPage), not
  // inline here, so this page stays focused on destination management.
  const handleEditMetadata = (id: string) => {
    void navigate('/metadata', { state: { platformId: id } });
  };

  const definitions = definitionsQuery.data ?? [];
  const canAddPlatform = definitions.length > 0;

  const summary = [
    { key: 'total', value: platforms.length, label: t('platforms:managementPage.summary.total') },
    { key: 'configured', value: configuredCount, label: t('platforms:managementPage.summary.configured') },
    { key: 'enabled', value: enabledCount, label: t('platforms:managementPage.summary.enabled') },
    { key: 'active', value: activeCount, label: t('platforms:managementPage.summary.active') },
  ] as const;

  return (
    <AppShell
      title={t('pages:platforms.title')}
      description={t('pages:platforms.description')}
      actions={
        <>
          <Button
            variant="primary"
            icon={<Plus className="size-4" />}
            disabled={!canAddPlatform}
            title={canAddPlatform ? undefined : t('dashboard:definitionsUnavailable')}
            onClick={() => setAddOpen(true)}
          >
            {t('dashboard:actions.addPlatform')}
          </Button>
          <Button icon={<Users className="size-4" />} onClick={() => void navigate('/settings')}>
            {t('platforms:managementPage.manageAccounts')}
          </Button>
        </>
      }
    >
      {platformsQuery.isSuccess && platforms.length > 0 && (
        <div
          role="group"
          aria-label={t('platforms:managementPage.summary.heading')}
          className="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-4 xl:mb-5"
        >
          {summary.map((stat) => (
            <div key={stat.key} className="rounded-xl border border-line bg-surface-sunken/70 px-3 py-2.5">
              <p className="font-mono text-xl leading-none tabular-nums text-ink">{stat.value}</p>
              <p className="mt-1.5 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
                {stat.label}
              </p>
            </div>
          ))}
        </div>
      )}

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
              <p className="text-sm font-medium text-ink">{t('dashboard:states.emptyTitle')}</p>
              <p className="mx-auto max-w-sm text-xs leading-relaxed text-ink-muted">
                {t('dashboard:states.emptyMessage')}
              </p>
            </div>
            <Button
              variant="primary"
              icon={<Plus className="size-3.5" />}
              disabled={!canAddPlatform}
              onClick={() => setAddOpen(true)}
            >
              {t('dashboard:actions.addPlatform')}
            </Button>
          </PanelBody>
        </Panel>
      )}

      {platformsQuery.isSuccess && platforms.length > 0 && (
        <PlatformGrid platforms={platforms} onOpenSettings={setSettingsId} onEditMetadata={handleEditMetadata} />
      )}

      <AddPlatformDialog open={addOpen} onClose={() => setAddOpen(false)} definitions={definitions} />

      <PlatformSettingsDialog
        platform={settingsPlatform}
        onClose={() => setSettingsId(null)}
        onDeleted={() => setSettingsId(null)}
      />
    </AppShell>
  );
}
