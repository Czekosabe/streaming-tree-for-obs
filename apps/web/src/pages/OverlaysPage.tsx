import { Loader2, RefreshCw } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { OverlayEditor } from '@/components/overlays/OverlayEditor';
import { OverlayListPanel } from '@/components/overlays/OverlayListPanel';
import { Button } from '@/components/ui/Button';
import { Panel, PanelBody } from '@/components/ui/Panel';
import { useChatOverlaysQuery } from '@/hooks/use-chat-overlay';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

export function OverlaysPage() {
  const { t } = useTranslation('overlays');
  const tErrors = useTranslation('errors').t;
  const overlaysQuery = useChatOverlaysQuery();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const overlays = overlaysQuery.data ?? [];
  const activeId = selectedId !== null && overlays.some((overlay) => overlay.id === selectedId) ? selectedId : null;

  return (
    <AppShell title={t('page.title')} description={t('page.description')}>
      {overlaysQuery.isPending && (
        <Panel>
          <PanelBody className="flex items-center justify-center gap-2 py-12 text-sm text-ink-muted">
            <Loader2 aria-hidden="true" className="size-4 animate-spin" />
            {t('page.loading')}
          </PanelBody>
        </Panel>
      )}

      {overlaysQuery.isError && (
        <Panel>
          <PanelBody className="space-y-3 py-10 text-center">
            <p className="text-sm font-medium text-status-error">{t('page.unavailable')}</p>
            <p className="mx-auto max-w-md text-xs leading-relaxed text-ink-muted">
              {resolveApiErrorMessage(tErrors, overlaysQuery.error)}
            </p>
            <Button
              variant="primary"
              icon={<RefreshCw className="size-3.5" />}
              onClick={() => void overlaysQuery.refetch()}
            >
              {t('page.retryAction')}
            </Button>
          </PanelBody>
        </Panel>
      )}

      {overlaysQuery.isSuccess && (
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-[360px_1fr] xl:items-start">
          <OverlayListPanel overlays={overlays} selectedId={activeId} onSelect={setSelectedId} />
          {activeId !== null && <OverlayEditor overlayId={activeId} />}
        </div>
      )}
    </AppShell>
  );
}
