import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { OverlayEditor } from '@/components/overlays/OverlayEditor';
import { OverlayListPanel } from '@/components/overlays/OverlayListPanel';
import { useChatOverlaysQuery } from '@/hooks/use-chat-overlay';

export function OverlaysPage() {
  const { t } = useTranslation('overlays');
  const overlaysQuery = useChatOverlaysQuery();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const overlays = overlaysQuery.data ?? [];
  const activeId = selectedId !== null && overlays.some((overlay) => overlay.id === selectedId) ? selectedId : null;

  return (
    <AppShell title={t('page.title')} description={t('page.description')}>
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[360px_1fr] xl:items-start">
        <OverlayListPanel overlays={overlays} selectedId={activeId} onSelect={setSelectedId} />
        {activeId !== null && <OverlayEditor overlayId={activeId} />}
      </div>
    </AppShell>
  );
}
