import { Plus, Settings } from 'lucide-react';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import { AppShell } from '@/components/layout/AppShell';
import { MetadataEditor } from '@/components/metadata/MetadataEditor';
import { PlatformGrid } from '@/components/platforms/PlatformGrid';
import { SystemStatusRail } from '@/components/system/SystemStatusRail';
import { Button } from '@/components/ui/Button';
import { PLATFORM_IDS, type PlatformId } from '@/models/platform';
import { useDemoStream } from '@/state/use-demo-stream';

/**
 * Main operator view.
 *
 * Wide screens use a two-column body (branches + metadata on the left, system
 * status rail on the right). Below `xl` the rail moves underneath the content.
 */
export function DashboardPage() {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { platforms, startPlatform, stopPlatform, updateMetadata } = useDemoStream();

  const [activePlatformId, setActivePlatformId] = useState<PlatformId>(PLATFORM_IDS[0]);
  const metadataRef = useRef<HTMLDivElement>(null);

  /** The card's gear icon focuses the matching tab in the metadata editor. */
  const handleConfigure = (id: PlatformId) => {
    setActivePlatformId(id);
    metadataRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  return (
    <AppShell
      title={t('title')}
      description={t('description')}
      actions={
        <>
          <Button
            variant="primary"
            icon={<Plus className="size-4" />}
            onClick={() => void navigate('/platforms')}
          >
            <span className="hidden sm:inline">{t('actions.addPlatform')}</span>
            <span className="sm:hidden">{t('actions.addPlatformShort')}</span>
          </Button>
          <Button
            icon={<Settings className="size-4" />}
            onClick={() => void navigate('/settings')}
          >
            <span className="hidden md:inline">{t('actions.globalSettings')}</span>
            <span className="md:hidden">{t('actions.globalSettingsShort')}</span>
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_20rem] xl:gap-5">
        <div className="min-w-0 space-y-4 xl:space-y-5">
          <section aria-labelledby="branches-heading">
            <div className="mb-3 flex items-baseline justify-between gap-3">
              <h2 id="branches-heading" className="text-sm font-semibold tracking-tight text-ink">
                {t('branches.heading')}
              </h2>
              <p className="text-[11px] text-ink-faint">{t('branches.note')}</p>
            </div>

            <PlatformGrid
              platforms={platforms}
              onStart={startPlatform}
              onStop={stopPlatform}
              onConfigure={handleConfigure}
            />
          </section>

          <div ref={metadataRef} className="scroll-mt-20">
            <MetadataEditor
              platforms={platforms}
              activeId={activePlatformId}
              onSelect={setActivePlatformId}
              onSave={updateMetadata}
            />
          </div>
        </div>

        <SystemStatusRail platforms={platforms} />
      </div>
    </AppShell>
  );
}
