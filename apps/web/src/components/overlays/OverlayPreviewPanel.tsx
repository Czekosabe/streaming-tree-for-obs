import { useTranslation } from 'react-i18next';

import type { ChatOverlayEditableFields } from '@/api/chat-overlay-schemas';
import { ChatOverlayRenderer } from '@/components/chat-overlay/ChatOverlayRenderer';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { toPreviewConfig } from '@/models/chat-overlay-config';
import { overlayPreviewFixtures } from '@/models/overlay-preview-fixtures';

/**
 * A local preview of the current draft, using deterministic synthetic
 * fixtures - never published to Twitch, the operator chat projection, or
 * a public SSE stream (Part 19). Updates immediately as the draft
 * changes; the real public overlay only updates after Save.
 */
export function OverlayPreviewPanel({ draft }: { draft: ChatOverlayEditableFields }) {
  const { t } = useTranslation('overlays');

  return (
    <Panel>
      <PanelHeader
        title={t('preview.title')}
        description={t('preview.description')}
        actions={
          <span className="rounded border border-line px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-ink-faint">
            {t('preview.syntheticBadge')}
          </span>
        }
      />
      <PanelBody>
        <div
          className="relative h-[420px] w-full overflow-hidden rounded-lg border border-line bg-[repeating-conic-gradient(#1d2740_0%_25%,transparent_0%_50%)] bg-[length:16px_16px]"
          data-testid="overlay-preview-surface"
        >
          <ChatOverlayRenderer config={toPreviewConfig(draft)} items={overlayPreviewFixtures()} />
        </div>
      </PanelBody>
    </Panel>
  );
}
