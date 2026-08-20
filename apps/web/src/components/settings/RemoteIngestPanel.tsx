import { Check, Copy, KeyRound, RefreshCw, ShieldAlert, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Modal } from '@/components/ui/Modal';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import {
  isRemoteIngestUnavailable,
  useProvisionRemoteIngestMutation,
  useRemoteIngestStatusQuery,
  useRevokeRemoteIngestMutation,
  useRotateRemoteIngestMutation,
} from '@/hooks/use-remote-ingest';
import { ApiError } from '@/lib/api-client';

/**
 * Stage 20D2C remote-ingest credential management (docs/remote-ingest.md
 * §8/§28). Renders nothing on a deployment where `--remote-ingest` is not
 * active - the status query's own 404 is treated as "this feature does
 * not exist here," never an error banner.
 */
export function RemoteIngestPanel() {
  const { t } = useTranslation('pages');
  const status = useRemoteIngestStatusQuery();
  const provision = useProvisionRemoteIngestMutation();
  const rotate = useRotateRemoteIngestMutation();
  const revoke = useRevokeRemoteIngestMutation();

  const [oneTimeSecret, setOneTimeSecret] = useState<string | null>(null);
  const [confirmingRotate, setConfirmingRotate] = useState(false);
  const [confirmingRevoke, setConfirmingRevoke] = useState(false);
  const [copied, setCopied] = useState(false);

  // Clear the one-time secret from React state whenever this panel
  // unmounts - it must never outlive the dialog that shows it once
  // (docs/remote-ingest.md §6/§8).
  useEffect(() => () => setOneTimeSecret(null), []);

  if (isRemoteIngestUnavailable(status.error)) return null;
  if (status.isLoading) return null;
  if (!status.data) return null;

  const data = status.data;

  function actionErrorMessage(error: unknown): string {
    if (error instanceof ApiError) {
      if (error.code === 'streaming_active') return t('settings.remoteIngest.errorStreamingActive');
      if (error.code === 'already_provisioned') return t('settings.remoteIngest.errorAlreadyProvisioned');
    }
    return t('settings.remoteIngest.errorGeneric');
  }

  async function handleCopySecret() {
    if (!oneTimeSecret) return;
    await navigator.clipboard.writeText(oneTimeSecret);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <Panel>
      <PanelHeader
        title={t('settings.remoteIngest.title')}
        description={t('settings.remoteIngest.description')}
      />
      <PanelBody className="space-y-3">
        <dl className="grid grid-cols-1 gap-2 text-xs sm:grid-cols-2">
          <div>
            <dt className="text-ink-faint">{t('settings.remoteIngest.endpointLabel')}</dt>
            <dd className="font-mono text-ink">{data.rtmpsAddress}</dd>
          </div>
          <div>
            <dt className="text-ink-faint">{t('settings.remoteIngest.pathLabel')}</dt>
            <dd className="font-mono text-ink">{data.ingestPath}</dd>
          </div>
          <div>
            <dt className="text-ink-faint">{t('settings.remoteIngest.credentialLabel')}</dt>
            <dd className="text-ink">
              {data.configured
                ? t('settings.remoteIngest.credentialConfigured')
                : t('settings.remoteIngest.credentialNotConfigured')}
            </dd>
          </div>
          <div>
            <dt className="text-ink-faint">{t('settings.remoteIngest.receivingLabel')}</dt>
            <dd className="text-ink">
              {data.receiving
                ? t('settings.remoteIngest.receivingYes')
                : t('settings.remoteIngest.receivingNo')}
            </dd>
          </div>
        </dl>

        {(provision.isError || rotate.isError || revoke.isError) && (
          <p className="text-xs text-status-error">
            {actionErrorMessage(provision.error ?? rotate.error ?? revoke.error)}
          </p>
        )}

        <div className="flex flex-wrap items-center gap-2">
          {!data.configured ? (
            <Button
              size="sm"
              icon={<KeyRound className="size-3.5" />}
              disabled={provision.isPending}
              onClick={() => {
                provision.mutate(undefined, {
                  onSuccess: (result) => setOneTimeSecret(result.secret),
                });
              }}
            >
              {t('settings.remoteIngest.provisionAction')}
            </Button>
          ) : (
            <>
              <Button
                size="sm"
                variant="secondary"
                icon={<RefreshCw className="size-3.5" />}
                onClick={() => setConfirmingRotate(true)}
              >
                {t('settings.remoteIngest.rotateAction')}
              </Button>
              <Button
                size="sm"
                variant="danger"
                icon={<Trash2 className="size-3.5" />}
                onClick={() => setConfirmingRevoke(true)}
              >
                {t('settings.remoteIngest.revokeAction')}
              </Button>
            </>
          )}
        </div>
      </PanelBody>

      <ConfirmDialog
        open={confirmingRotate}
        title={t('settings.remoteIngest.rotateConfirmTitle')}
        message={t('settings.remoteIngest.rotateConfirmBody')}
        confirmLabel={t('settings.remoteIngest.rotateConfirmAction')}
        busy={rotate.isPending}
        onCancel={() => setConfirmingRotate(false)}
        onConfirm={() => {
          rotate.mutate(undefined, {
            onSuccess: (result) => {
              setConfirmingRotate(false);
              setOneTimeSecret(result.secret);
            },
            onError: () => setConfirmingRotate(false),
          });
        }}
      />

      <ConfirmDialog
        open={confirmingRevoke}
        title={t('settings.remoteIngest.revokeConfirmTitle')}
        message={t('settings.remoteIngest.revokeConfirmBody')}
        confirmLabel={t('settings.remoteIngest.revokeConfirmAction')}
        destructive
        busy={revoke.isPending}
        onCancel={() => setConfirmingRevoke(false)}
        onConfirm={() => {
          revoke.mutate(undefined, { onSettled: () => setConfirmingRevoke(false) });
        }}
      />

      <Modal
        open={oneTimeSecret !== null}
        onClose={() => setOneTimeSecret(null)}
        title={t('settings.remoteIngest.secretModalTitle')}
        size="md"
        footer={
          <Button type="button" onClick={() => setOneTimeSecret(null)}>
            {t('settings.remoteIngest.secretModalDone')}
          </Button>
        }
      >
        <div className="space-y-3">
          <p className="flex items-start gap-2 text-sm font-medium text-status-error">
            <ShieldAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            {t('settings.remoteIngest.secretWarning')}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <code className="min-w-0 flex-1 break-all rounded-lg border border-line bg-surface-sunken px-3 py-2 text-xs text-ink">
              {oneTimeSecret}
            </code>
            <Button
              size="sm"
              icon={copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
              onClick={handleCopySecret}
            >
              {t('settings.remoteIngest.secretCopyAction')}
            </Button>
          </div>
          <p className="text-xs text-ink-muted">{t('settings.remoteIngest.secretUsageHint')}</p>
        </div>
      </Modal>
    </Panel>
  );
}
