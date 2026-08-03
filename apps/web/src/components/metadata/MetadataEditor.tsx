import { SlidersHorizontal } from 'lucide-react';

import type { PlatformId, PlatformMetadata, StreamPlatform } from '@/models/platform';

import { DemoBadge } from '../ui/DemoBadge';
import { Panel, PanelBody, PanelHeader } from '../ui/Panel';
import { MetadataForm } from './MetadataForm';
import { PlatformTabs } from './PlatformTabs';

type MetadataEditorProps = {
  platforms: readonly StreamPlatform[];
  activeId: PlatformId;
  onSelect: (id: PlatformId) => void;
  onSave: (id: PlatformId, metadata: PlatformMetadata) => void;
};

/**
 * Tabbed metadata editor - one tab per platform branch.
 *
 * The form is keyed by platform id so switching tabs starts a fresh draft from
 * that platform's stored metadata instead of leaking values across platforms.
 */
export function MetadataEditor({ platforms, activeId, onSelect, onSave }: MetadataEditorProps) {
  const activePlatform = platforms.find((platform) => platform.id === activeId);

  return (
    <Panel>
      <PanelHeader
        title="Metadata editor"
        description="Fields are derived from each platform's capability table"
        icon={<SlidersHorizontal className="size-4" />}
        actions={<DemoBadge title="Saved values stay in memory - no platform API is called" />}
      />

      <PlatformTabs platforms={platforms} activeId={activeId} onSelect={onSelect} />

      <PanelBody>
        {activePlatform === undefined ? (
          <p className="text-sm text-ink-muted">Select a platform to edit its metadata.</p>
        ) : (
          <div
            role="tabpanel"
            id={`metadata-panel-${activePlatform.id}`}
            aria-labelledby={`metadata-tab-${activePlatform.id}`}
            tabIndex={0}
            className="animate-fade-rise"
          >
            <MetadataForm
              key={activePlatform.id}
              platform={activePlatform}
              onSave={(metadata) => onSave(activePlatform.id, metadata)}
            />
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}
