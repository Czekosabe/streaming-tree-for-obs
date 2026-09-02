import { ClipboardCheck, Layers, Loader2, Plus, RefreshCw, Settings, Tv } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { AppShell } from '@/components/layout/AppShell';
import { MetadataEditor } from '@/components/metadata/MetadataEditor';
import { OnboardingDashboardBanner } from '@/components/onboarding/OnboardingDashboardBanner';
import { AddPlatformDialog } from '@/components/platforms/AddPlatformDialog';
import { PlatformGrid } from '@/components/platforms/PlatformGrid';
import { PlatformSettingsDialog } from '@/components/platforms/PlatformSettingsDialog';
import { PreflightDialog } from '@/components/preflight/PreflightDialog';
import { StreamSetupsDialog } from '@/components/stream-setup/StreamSetupsDialog';
import { SystemStatusRail } from '@/components/system/SystemStatusRail';
import { Button } from '@/components/ui/Button';
import { Panel, PanelBody } from '@/components/ui/Panel';
import { usePlatformDefinitionsQuery, usePlatformsQuery } from '@/hooks/use-platforms';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

/**
 * Main operator view.
 *
 * All platform data comes from the backend; there is no demo configuration
 * fallback. When the backend is unavailable the shell keeps working and this
 * page shows an explicit error with a retry, rather than pretending saved
 * configurations still exist locally.
 */
export function DashboardPage() {
  const { t } = useTranslation(['dashboard', 'platforms', 'errors']);
  const tErrors = useTranslation('errors').t;
  const navigate = useNavigate();

  const platformsQuery = usePlatformsQuery();
  const definitionsQuery = usePlatformDefinitionsQuery();

  const [addOpen, setAddOpen] = useState(false);
  const [settingsId, setSettingsId] = useState<string | null>(null);
  const [activeMetadataId, setActiveMetadataId] = useState<string | null>(null);
  const [metadataDirty, setMetadataDirty] = useState(false);
  const [streamSetupsOpen, setStreamSetupsOpen] = useState(false);
  const [preflightOpen, setPreflightOpen] = useState(false);
  const metadataRef = useRef<HTMLDivElement>(null);

  // Memoised so the `??` fallback does not produce a new array identity on
  // every render and re-trigger the selection effect below.
  const platforms: ConfiguredPlatform[] = useMemo(
    () => platformsQuery.data ?? [],
    [platformsQuery.data],
  );

  // Keep the selected metadata tab valid as the list changes: pick the first
  // platform initially, and move off a platform that was just deleted.
  useEffect(() => {
    if (platforms.length === 0) {
      if (activeMetadataId !== null) setActiveMetadataId(null);
      return;
    }
    const stillExists = platforms.some((platform) => platform.id === activeMetadataId);
    if (!stillExists) {
      setActiveMetadataId(platforms[0]?.id ?? null);
    }
  }, [platforms, activeMetadataId]);

  const settingsPlatform = platforms.find((platform) => platform.id === settingsId) ?? null;

  const handleEditMetadata = (id: string) => {
    setActiveMetadataId(id);
    metadataRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  const definitions = definitionsQuery.data ?? [];
  const canAddPlatform = definitions.length > 0;

  return (
    <AppShell
      title={t('dashboard:title')}
      description={t('dashboard:description')}
      actions={
        <>
          <Button
            variant="primary"
            icon={<Plus className="size-4" />}
            disabled={!canAddPlatform}
            title={canAddPlatform ? undefined : t('dashboard:definitionsUnavailable')}
            onClick={() => setAddOpen(true)}
          >
            <span className="hidden sm:inline">{t('dashboard:actions.addPlatform')}</span>
            <span className="sm:hidden">{t('dashboard:actions.addPlatformShort')}</span>
          </Button>
          <Button icon={<Layers className="size-4" />} onClick={() => setStreamSetupsOpen(true)}>
            <span className="hidden md:inline">{t('dashboard:actions.streamSetups')}</span>
            <span className="md:hidden">{t('dashboard:actions.streamSetupsShort')}</span>
          </Button>
          <Button icon={<ClipboardCheck className="size-4" />} onClick={() => setPreflightOpen(true)}>
            <span className="hidden md:inline">{t('dashboard:actions.preflight')}</span>
            <span className="md:hidden">{t('dashboard:actions.preflightShort')}</span>
          </Button>
          <Button icon={<Settings className="size-4" />} onClick={() => void navigate('/settings')}>
            <span className="hidden md:inline">{t('dashboard:actions.globalSettings')}</span>
            <span className="md:hidden">{t('dashboard:actions.globalSettingsShort')}</span>
          </Button>
        </>
      }
    >
      <OnboardingDashboardBanner />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_20rem] xl:gap-5">
        <div className="min-w-0 space-y-4 xl:space-y-5">
          <section aria-labelledby="branches-heading">
            <div className="mb-3 flex items-baseline justify-between gap-3">
              <h2 id="branches-heading" className="text-sm font-semibold tracking-tight text-ink">
                {t('dashboard:branches.heading')}
              </h2>
              <p className="text-[11px] text-ink-faint">{t('dashboard:branches.note')}</p>
            </div>

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
                  <p className="text-sm font-medium text-status-error">
                    {t('dashboard:states.loadFailedTitle')}
                  </p>
                  <p className="mx-auto max-w-md text-xs leading-relaxed text-ink-muted">
                    {resolveApiErrorMessage(tErrors, platformsQuery.error)}
                  </p>
                  <p className="mx-auto max-w-md text-[11px] text-ink-faint">
                    {t('dashboard:states.noLocalFallback')}
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
              <PlatformGrid
                platforms={platforms}
                onOpenSettings={setSettingsId}
                onEditMetadata={handleEditMetadata}
              />
            )}
          </section>

          <div ref={metadataRef} className="scroll-mt-20">
            {definitionsQuery.isError ? (
              <Panel>
                <PanelBody className="space-y-2 py-8 text-center">
                  <p className="text-sm font-medium text-status-error">
                    {t('dashboard:definitionsUnavailable')}
                  </p>
                  <Button
                    icon={<RefreshCw className="size-3.5" />}
                    onClick={() => void definitionsQuery.refetch()}
                  >
                    {t('dashboard:states.retry')}
                  </Button>
                </PanelBody>
              </Panel>
            ) : (
              <MetadataEditor
                platforms={platforms}
                activeId={activeMetadataId}
                onSelect={setActiveMetadataId}
                dirty={metadataDirty}
                onDirtyChange={setMetadataDirty}
              />
            )}
          </div>
        </div>

        <SystemStatusRail platforms={platforms} />
      </div>

      <AddPlatformDialog
        open={addOpen}
        onClose={() => setAddOpen(false)}
        definitions={definitions}
      />

      <PlatformSettingsDialog
        platform={settingsPlatform}
        onClose={() => setSettingsId(null)}
        onDeleted={(id) => {
          if (activeMetadataId === id) setActiveMetadataId(null);
          setSettingsId(null);
        }}
      />

      <StreamSetupsDialog
        open={streamSetupsOpen}
        onClose={() => setStreamSetupsOpen(false)}
        platforms={platforms}
        activeMetadataId={activeMetadataId}
        activeMetadataDirty={metadataDirty}
      />

      <PreflightDialog
        open={preflightOpen}
        onClose={() => setPreflightOpen(false)}
        platforms={platforms}
        onOpenDestinationSettings={setSettingsId}
        onEditMetadata={handleEditMetadata}
        onOpenStreamSetups={() => setStreamSetupsOpen(true)}
      />
    </AppShell>
  );
}
