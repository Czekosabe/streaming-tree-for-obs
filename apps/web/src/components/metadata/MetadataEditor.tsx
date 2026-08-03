import { SlidersHorizontal } from 'lucide-react';
import { useTranslation } from 'react-i18next';

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
  const { t } = useTranslation('metadata');
  const activePlatform = platforms.find((platform) => platform.id === activeId);

  return (
    <Panel>
      <PanelHeader
        title={t('editor.heading')}
        description={t('editor.description')}
        icon={<SlidersHorizontal className="size-4" />}
        actions={<DemoBadge title={t('editor.demoTooltip')} />}
      />

      <PlatformTabs platforms={platforms} activeId={activeId} onSelect={onSelect} />

      <PanelBody>
        {activePlatform === undefined ? (
          <p className="text-sm text-ink-muted">{t('editor.empty')}</p>
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
