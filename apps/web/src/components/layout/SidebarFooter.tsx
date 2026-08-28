import { Radio } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { CopyableValue } from '@/components/runtime/CopyableValue';
import { RuntimeControls } from '@/components/runtime/RuntimeControls';
import { runtimeErrorMessage } from '@/components/runtime/runtime-error-message';
import { StatusDot } from '@/components/ui/StatusBadge';
import { useAboutQuery } from '@/hooks/use-about-query';
import { useRuntimeQuery } from '@/hooks/use-runtime';
import { cn } from '@/lib/cn';
import { aboutVersionLine } from '@/models/about-presentation';
import { ingestStateKey, ingestTone, mediaMtxStateKey, mediaMtxTone } from '@/models/runtime-presentation';

/**
 * Bottom block of the sidebar: the real state of the local ingest service and
 * the values OBS needs.
 *
 * Everything here comes from `GET /api/runtime`. Nothing is a placeholder any
 * more, and no bitrate, resolution or frame rate is shown, because the runtime
 * API does not report any - inventing them would make the panel untrustworthy.
 */
export function SidebarFooter() {
  const { t } = useTranslation(['runtime', 'navigation', 'about']);
  const tAbout = useTranslation('about').t;
  const runtimeQuery = useRuntimeQuery();
  const aboutQuery = useAboutQuery();

  const snapshot = runtimeQuery.data;
  const mediaMtx = snapshot?.mediaMtx;
  const ingest = snapshot?.ingest;
  const connection = snapshot?.connection;

  const lastError = runtimeErrorMessage(t, mediaMtx?.lastError);

  return (
    <div className="mt-auto space-y-3 border-t border-line p-3">
      <section
        aria-label={t('runtime:ingest.heading')}
        className="rounded-lg border border-line bg-surface-sunken p-3"
      >
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            <Radio aria-hidden="true" className="size-3" />
            {t('runtime:mediamtx.componentName')}
          </span>
          {mediaMtx !== undefined && (
            <span className="text-[10px] text-ink-faint">{mediaMtx.supportedVersion}</span>
          )}
        </div>

        {runtimeQuery.isPending && (
          <p className="mt-1.5 text-xs text-ink-muted">{t('runtime:system.checking')}</p>
        )}

        {runtimeQuery.isError && (
          <p className="mt-1.5 text-xs text-status-error">
            {t('runtime:system.runtimeUnavailable')}
          </p>
        )}

        {mediaMtx !== undefined && ingest !== undefined && (
          <>
            {/* Service state */}
            <p className="mt-1.5 flex items-center gap-2 text-xs font-medium text-ink">
              <StatusDot status={mediaMtxTone(mediaMtx.state)} />
              {t(mediaMtxStateKey(mediaMtx.state))}
            </p>

            {/* Ingest state, only meaningful once the service runs */}
            <p
              className={cn(
                'mt-1 flex items-center gap-2 text-[11px]',
                ingest.state === 'receiving' ? 'text-status-live' : 'text-ink-muted',
              )}
            >
              <StatusDot status={ingestTone(ingest.state)} />
              {t(ingestStateKey(ingest.state))}
            </p>

            {ingest.state === 'receiving' && ingest.trackCount !== null && (
              <p className="mt-1 text-[11px] text-ink-faint">
                {t('runtime:ingest.trackCount', { count: ingest.trackCount })}
                {ingest.tracks.length > 0 && <> &middot; {ingest.tracks.join(', ')}</>}
              </p>
            )}

            {lastError !== null && (
              <p className="mt-1.5 rounded border border-status-error/30 bg-status-error/10 px-1.5 py-1 text-[10px] leading-relaxed text-status-error">
                {lastError}
              </p>
            )}

            <div className="mt-2.5">
              <RuntimeControls mediaMtx={mediaMtx} compact />
            </div>
          </>
        )}

        {connection !== undefined && (
          <div className="mt-3 space-y-2 border-t border-line pt-2.5">
            <CopyableValue
              label={t('runtime:connection.serverLabel')}
              value={connection.serverUrl}
              copyLabel={t('runtime:connection.copyServer')}
            />
            <CopyableValue
              label={t('runtime:connection.streamKeyLabel')}
              value={connection.streamKey}
              copyLabel={t('runtime:connection.copyStreamKey')}
            />
            {/* Stated explicitly so nobody treats the local route name like a
                destination platform key. */}
            <p className="text-[10px] leading-relaxed text-ink-faint">
              {t('runtime:connection.notASecret')}
            </p>
          </div>
        )}
      </section>

      {aboutQuery.data !== undefined && (
        <p className="px-1 text-[10px] text-ink-faint">{aboutVersionLine(tAbout, aboutQuery.data)}</p>
      )}
    </div>
  );
}
