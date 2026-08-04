import { Loader2, Radio, RefreshCw, Server, Tv } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { RuntimeSnapshot } from '@/api/runtime-schemas';
import { AppShell } from '@/components/layout/AppShell';
import { CopyableValue } from '@/components/runtime/CopyableValue';
import { RuntimeControls } from '@/components/runtime/RuntimeControls';
import { runtimeErrorMessage } from '@/components/runtime/runtime-error-message';
import { Button } from '@/components/ui/Button';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { useRuntimeQuery } from '@/hooks/use-runtime';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import {
  ingestStateKey,
  ingestTone,
  mediaMtxStateKey,
  mediaMtxTone,
} from '@/models/runtime-presentation';

/** One label/value row. */
function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3 text-xs">
      <span className="text-ink-muted">{label}</span>
      <span className="truncate text-right font-mono text-ink" title={value}>
        {value}
      </span>
    </div>
  );
}

function IngestServicePanel({ snapshot }: { snapshot: RuntimeSnapshot }) {
  const { t } = useTranslation('runtime');
  const { mediaMtx, ingest } = snapshot;

  const lastError = runtimeErrorMessage(t, mediaMtx.lastError);

  const sourceLabel =
    mediaMtx.source === 'managed'
      ? t('mediamtx.sourceManaged')
      : mediaMtx.source === 'override'
        ? t('mediamtx.sourceOverride')
        : t('mediamtx.sourceMissing');

  return (
    <Panel>
      <PanelHeader
        title={t('page.componentHeading')}
        description={t('mediamtx.componentName')}
        icon={<Server className="size-4" />}
        actions={<StatusBadge status={mediaMtxTone(mediaMtx.state)} label={t(mediaMtxStateKey(mediaMtx.state))} />}
      />
      <PanelBody className="space-y-4">
        <div className="space-y-1.5">
          <Row label={t('mediamtx.supportedVersion')} value={mediaMtx.supportedVersion} />
          {mediaMtx.installedVersion !== undefined && mediaMtx.installedVersion !== '' && (
            <Row label={t('mediamtx.installedVersion')} value={mediaMtx.installedVersion} />
          )}
          <Row label={t('mediamtx.source')} value={sourceLabel} />
          {mediaMtx.startedAt !== undefined && mediaMtx.startedAt !== '' && (
            <Row label={t('mediamtx.startedAt')} value={mediaMtx.startedAt} />
          )}
          <Row label={t('mediamtx.restartCount')} value={String(mediaMtx.restartCount)} />
        </div>

        <div className="space-y-1 border-t border-line pt-3 text-[11px] text-ink-faint">
          <p>
            {t('mediamtx.autoStart')}: {mediaMtx.autoStart ? 'on' : 'off'}
          </p>
          <p>
            {t('mediamtx.autoRestart')}: {mediaMtx.autoRestart ? 'on' : 'off'}
          </p>
        </div>

        <div className="border-t border-line pt-3">
          <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('page.lastError')}
          </p>
          {lastError === null ? (
            <p className="mt-1 text-xs text-ink-muted">{t('page.noError')}</p>
          ) : (
            <p className="mt-1 rounded-lg border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-xs leading-relaxed text-status-error">
              {lastError}
            </p>
          )}
        </div>

        <div className="border-t border-line pt-3">
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('controls.heading')}
          </p>
          <RuntimeControls mediaMtx={mediaMtx} />
        </div>

        {/* Ingest is only meaningful once the service is running. */}
        <div className="border-t border-line pt-3">
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('ingest.heading')}
          </p>
          <div className="flex items-center gap-2">
            <StatusBadge status={ingestTone(ingest.state)} label={t(ingestStateKey(ingest.state))} />
          </div>

          <div className="mt-2 space-y-1.5">
            <Row label={t('ingest.path')} value={ingest.path} />
            {/* Only values MediaMTX actually reported are shown. */}
            {ingest.sourceType !== undefined && ingest.sourceType !== '' && (
              <Row label={t('ingest.sourceType')} value={ingest.sourceType} />
            )}
            {ingest.connectedAt !== undefined && ingest.connectedAt !== '' && (
              <Row label={t('ingest.connectedAt')} value={ingest.connectedAt} />
            )}
            {ingest.trackCount !== null && (
              <Row
                label={t('ingest.tracks')}
                value={
                  ingest.tracks.length > 0
                    ? ingest.tracks.join(', ')
                    : t('ingest.trackCount', { count: ingest.trackCount })
                }
              />
            )}
          </div>

          <p className="mt-2 text-[11px] leading-relaxed text-ink-faint">
            {t('ingest.publisherNote')}
          </p>
        </div>
      </PanelBody>
    </Panel>
  );
}

function ObsSettingsPanel({ snapshot }: { snapshot: RuntimeSnapshot }) {
  const { t } = useTranslation('runtime');
  const { connection } = snapshot;

  return (
    <Panel>
      <PanelHeader
        title={t('connection.heading')}
        description={t('connection.instructions')}
        icon={<Radio className="size-4" />}
      />
      <PanelBody className="space-y-3">
        <CopyableValue
          label={t('connection.serverLabel')}
          value={connection.serverUrl}
          copyLabel={t('connection.copyServer')}
        />
        <CopyableValue
          label={t('connection.streamKeyLabel')}
          value={connection.streamKey}
          copyLabel={t('connection.copyStreamKey')}
        />
        <CopyableValue
          label={t('connection.publishUrlLabel')}
          value={connection.publishUrl}
          copyLabel={t('connection.copyServer')}
        />
        <p className="rounded-lg border border-line bg-surface-sunken px-3 py-2 text-[11px] leading-relaxed text-ink-faint">
          {t('connection.notASecret')}
        </p>
      </PanelBody>
    </Panel>
  );
}

/**
 * Local ingest status page.
 *
 * Shows the real state of the MediaMTX component and the OBS connection values.
 * Outgoing platform branches stay marked as a later stage, because no FFmpeg
 * process exists yet.
 */
export function StreamsPage() {
  const { t } = useTranslation(['runtime', 'errors']);
  const tErrors = useTranslation('errors').t;
  const runtimeQuery = useRuntimeQuery();

  return (
    <AppShell title={t('runtime:page.title')} description={t('runtime:page.description')}>
      <div className="mx-auto max-w-3xl space-y-4">
        {runtimeQuery.isPending && (
          <Panel>
            <PanelBody className="flex items-center justify-center gap-2 py-12 text-sm text-ink-muted">
              <Loader2 aria-hidden="true" className="size-4 animate-spin" />
              {t('runtime:system.checking')}
            </PanelBody>
          </Panel>
        )}

        {runtimeQuery.isError && (
          <Panel>
            <PanelBody className="space-y-3 py-10 text-center">
              <p className="text-sm font-medium text-status-error">
                {t('runtime:system.runtimeUnavailable')}
              </p>
              <p className="mx-auto max-w-md text-xs leading-relaxed text-ink-muted">
                {resolveApiErrorMessage(tErrors, runtimeQuery.error)}
              </p>
              <Button
                variant="primary"
                icon={<RefreshCw className="size-3.5" />}
                onClick={() => void runtimeQuery.refetch()}
              >
                {t('runtime:controls.restart')}
              </Button>
            </PanelBody>
          </Panel>
        )}

        {runtimeQuery.data !== undefined && (
          <>
            <IngestServicePanel snapshot={runtimeQuery.data} />
            <ObsSettingsPanel snapshot={runtimeQuery.data} />
          </>
        )}

        <Panel>
          <PanelHeader
            title={t('runtime:page.destinationsHeading')}
            icon={<Tv className="size-4" />}
          />
          <PanelBody>
            <p className="text-xs leading-relaxed text-ink-muted">
              {t('runtime:page.destinationsPlanned')}
            </p>
          </PanelBody>
        </Panel>
      </div>
    </AppShell>
  );
}
