import { Check, Copy, ExternalLink, Globe, RefreshCw, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import type { RemoteOverlayDomain } from '@/api/remote-overlay-schemas';
import {
  isRemoteOverlayUnavailable,
  useDisableRemoteOverlayMutation,
  useEnableRemoteOverlayMutation,
  useRemoteOverlayStatusQuery,
  useRotateRemoteOverlayMutation,
} from '@/hooks/use-remote-overlay';

/**
 * Stage 20D2C remote-overlay capability controls (docs/remote-ingest.md
 * §12/§28) - one shared, domain-parameterized panel rendered inside
 * each overlay/profile's own existing management surface (chat, alert,
 * audio, widget), next to that surface's own local Browser Source URL
 * display, never a separate overlay-management application.
 *
 * Renders nothing on a deployment with no remote overlay origin
 * configured - the status query's own 404 is treated as "this feature
 * does not exist here."
 */
export function RemoteOverlayPanel({ domain, localSlug }: { domain: RemoteOverlayDomain; localSlug: string }) {
  const { t } = useTranslation('overlays');
  const status = useRemoteOverlayStatusQuery(domain, localSlug);
  const enableMutation = useEnableRemoteOverlayMutation(domain, localSlug);
  const rotateMutation = useRotateRemoteOverlayMutation(domain, localSlug);
  const disableMutation = useDisableRemoteOverlayMutation(domain, localSlug);

  const [copied, setCopied] = useState(false);
  const [confirmingRotate, setConfirmingRotate] = useState(false);
  const [confirmingDisable, setConfirmingDisable] = useState(false);

  if (isRemoteOverlayUnavailable(status.error)) return null;
  if (status.isLoading) return null;
  if (!status.data) return null;

  const data = status.data;
  const busy = enableMutation.isPending || rotateMutation.isPending || disableMutation.isPending;

  async function handleCopy() {
    if (!data.url) return;
    await navigator.clipboard.writeText(data.url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <Panel>
      <PanelHeader title={t('remote.title')} description={t('remote.description')} />
      <PanelBody className="space-y-3">
        {data.enabled && data.url ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded-lg border border-line bg-surface-sunken px-3 py-2 text-xs text-ink">
                {data.url}
              </code>
              <Button size="sm" icon={copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />} onClick={handleCopy}>
                {t('remote.copyAction')}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                icon={<ExternalLink className="size-3.5" />}
                onClick={() => window.open(data.url, '_blank', 'noopener,noreferrer')}
              >
                {t('remote.openAction')}
              </Button>
            </div>
            {copied && <p className="text-xs text-status-live">{t('remote.copied')}</p>}
            <p className="text-xs text-ink-muted">{t('remote.enabledHint')}</p>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                icon={<RefreshCw className="size-3.5" />}
                disabled={busy}
                onClick={() => setConfirmingRotate(true)}
              >
                {t('remote.rotateAction')}
              </Button>
              <Button
                size="sm"
                variant="danger"
                icon={<Trash2 className="size-3.5" />}
                disabled={busy}
                onClick={() => setConfirmingDisable(true)}
              >
                {t('remote.disableAction')}
              </Button>
            </div>
          </>
        ) : (
          <>
            <p className="text-xs text-ink-muted">{t('remote.disabledHint')}</p>
            <Button
              size="sm"
              icon={<Globe className="size-3.5" />}
              disabled={busy}
              onClick={() => enableMutation.mutate()}
            >
              {t('remote.enableAction')}
            </Button>
          </>
        )}
      </PanelBody>

      <ConfirmDialog
        open={confirmingRotate}
        title={t('remote.rotateConfirmTitle')}
        message={t('remote.rotateConfirmBody')}
        confirmLabel={t('remote.rotateConfirmAction')}
        busy={rotateMutation.isPending}
        onCancel={() => setConfirmingRotate(false)}
        onConfirm={() => {
          rotateMutation.mutate(undefined, { onSettled: () => setConfirmingRotate(false) });
        }}
      />

      <ConfirmDialog
        open={confirmingDisable}
        title={t('remote.disableConfirmTitle')}
        message={t('remote.disableConfirmBody')}
        confirmLabel={t('remote.disableConfirmAction')}
        destructive
        busy={disableMutation.isPending}
        onCancel={() => setConfirmingDisable(false)}
        onConfirm={() => {
          disableMutation.mutate(undefined, { onSettled: () => setConfirmingDisable(false) });
        }}
      />
    </Panel>
  );
}
