import { Check, Copy, ExternalLink, RefreshCw } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { useRotateChatOverlayPublicSlugMutation } from '@/hooks/use-chat-overlay';

function overlayUrl(publicSlug: string): string {
  return `${window.location.origin}/overlay/chat/${publicSlug}`;
}

/**
 * The stable, copyable Browser Source URL, plus explicit-confirmation
 * rotation (Part 5: "confirmation-protected in frontend"). Rotating
 * invalidates the current URL immediately and returns a new one - no
 * other settings are affected.
 */
export function OverlayUrlPanel({ overlayId, publicSlug }: { overlayId: string; publicSlug: string }) {
  const { t } = useTranslation('overlays');
  const [copied, setCopied] = useState(false);
  const [confirmingRotate, setConfirmingRotate] = useState(false);
  const rotate = useRotateChatOverlayPublicSlugMutation();

  const url = overlayUrl(publicSlug);

  async function handleCopy() {
    await navigator.clipboard.writeText(url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <Panel>
      <PanelHeader title={t('url.title')} description={t('url.description')} />
      <PanelBody className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded-lg border border-line bg-surface-sunken px-3 py-2 text-xs text-ink">
            {url}
          </code>
          <Button size="sm" icon={copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />} onClick={handleCopy}>
            {t('url.copyAction')}
          </Button>
          <Button size="sm" variant="ghost" icon={<ExternalLink className="size-3.5" />} onClick={() => window.open(url, '_blank', 'noopener,noreferrer')}>
            {t('url.openAction')}
          </Button>
          <Button size="sm" variant="secondary" icon={<RefreshCw className="size-3.5" />} onClick={() => setConfirmingRotate(true)}>
            {t('url.rotateAction')}
          </Button>
        </div>
        {copied && <p className="text-xs text-status-live">{t('url.copied')}</p>}
      </PanelBody>

      <ConfirmDialog
        open={confirmingRotate}
        title={t('url.rotateConfirmTitle')}
        message={t('url.rotateConfirmBody')}
        confirmLabel={t('url.rotateConfirmAction')}
        busy={rotate.isPending}
        onCancel={() => setConfirmingRotate(false)}
        onConfirm={() => {
          rotate.mutate(overlayId, { onSuccess: () => setConfirmingRotate(false) });
        }}
      />
    </Panel>
  );
}
