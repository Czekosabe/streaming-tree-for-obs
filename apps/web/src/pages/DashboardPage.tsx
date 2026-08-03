import { Plus, Settings } from 'lucide-react';
import { useRef, useState } from 'react';
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
      title="Dashboard"
      description="One OBS output, several independent platform branches."
      actions={
        <>
          <Button
            variant="primary"
            icon={<Plus className="size-4" />}
            onClick={() => void navigate('/platforms')}
          >
            <span className="hidden sm:inline">Add Platform</span>
            <span className="sm:hidden">Add</span>
          </Button>
          <Button
            icon={<Settings className="size-4" />}
            onClick={() => void navigate('/settings')}
          >
            <span className="hidden md:inline">Global Settings</span>
            <span className="md:hidden">Settings</span>
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_20rem] xl:gap-5">
        <div className="min-w-0 space-y-4 xl:space-y-5">
          <section aria-labelledby="branches-heading">
            <div className="mb-3 flex items-baseline justify-between gap-3">
              <h2 id="branches-heading" className="text-sm font-semibold tracking-tight text-ink">
                Platform branches
              </h2>
              <p className="text-[11px] text-ink-faint">
                Each branch runs independently - one failure does not stop the others.
              </p>
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
