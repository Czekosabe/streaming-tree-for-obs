import { ChevronDown, Radio } from 'lucide-react';
import { useId, useState } from 'react';
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
 *
 * Collapsible (Stage 20E defect B): with fourteen nav items and this section
 * both competing for sidebar height, the heading, live status dot and state
 * line stay visible always, but the controls/server/stream-key detail below
 * them collapse by default so the primary navigation gets materially more
 * room. A real, current error always stays visible regardless of collapse
 * state - this never hides something the operator needs to see. Collapse
 * state is plain component state, not a persisted preference; it survives
 * route navigation for free because `ShellLayout` (unlike the old per-page
 * `AppShell`) never remounts this component while navigating.
 */
export function SidebarFooter() {
  const { t } = useTranslation(['runtime', 'navigation', 'about']);
  const tAbout = useTranslation('about').t;
  const runtimeQuery = useRuntimeQuery();
  const aboutQuery = useAboutQuery();
  const [expanded, setExpanded] = useState(false);
  const detailsId = useId();

  const snapshot = runtimeQuery.data;
  const mediaMtx = snapshot?.mediaMtx;
  const ingest = snapshot?.ingest;
  const connection = snapshot?.connection;

  const lastError = runtimeErrorMessage(t, mediaMtx?.lastError);

  return (
    <div data-testid="sidebar-footer" className="mt-auto space-y-3 border-t border-line p-3">
      <section
        aria-label={t('runtime:ingest.heading')}
        className="rounded-lg border border-line bg-surface-sunken p-3"
      >
        {/*
         * "OBS Connection" is the primary, product-facing heading and the
         * real ingest state is the primary status line - MediaMTX is the
         * ingest engine that makes this work, not the concept the operator
         * came here to check. Its name/version moves to a small secondary
         * caption at the bottom of this section instead of leading it.
         */}
        <div className="flex items-center gap-1.5">
          <Radio aria-hidden="true" className="size-3.5 shrink-0 text-accent-soft" />
          <span className="text-xs font-semibold text-ink">{t('runtime:ingest.heading')}</span>

          {mediaMtx !== undefined && ingest !== undefined && (
            <button
              type="button"
              onClick={() => setExpanded((current) => !current)}
              aria-expanded={expanded}
              aria-controls={detailsId}
              className="ml-auto inline-flex size-6 shrink-0 items-center justify-center rounded-md text-ink-faint transition-colors hover:bg-surface-hover hover:text-ink"
            >
              <ChevronDown
                aria-hidden="true"
                className={cn('size-3.5 transition-transform duration-150', expanded && 'rotate-180')}
              />
              <span className="sr-only">
                {t(expanded ? 'runtime:ingest.collapseDetails' : 'runtime:ingest.expandDetails')}
              </span>
            </button>
          )}
        </div>

        {runtimeQuery.isPending && (
          <p className="mt-2 text-xs text-ink-muted">{t('runtime:system.checking')}</p>
        )}

        {runtimeQuery.isError && (
          <p className="mt-2 text-xs text-status-error">
            {t('runtime:system.runtimeUnavailable')}
          </p>
        )}

        {mediaMtx !== undefined && ingest !== undefined && (
          <>
            {/* Real OBS/ingest state - this is what "OBS Connection" answers.
                Always visible: this is the compact summary. */}
            <p
              className={cn(
                'mt-2 flex items-center gap-2 text-sm font-medium',
                ingest.state === 'receiving' ? 'text-status-live' : 'text-ink',
              )}
            >
              <StatusDot status={ingestTone(ingest.state)} />
              {t(ingestStateKey(ingest.state))}
            </p>

            {/* A real error never hides behind the collapsed state. */}
            {lastError !== null && (
              <p className="mt-1.5 rounded border border-status-error/30 bg-status-error/10 px-1.5 py-1 text-[10px] leading-relaxed text-status-error">
                {lastError}
              </p>
            )}

            <div id={detailsId} hidden={!expanded} className={expanded ? 'mt-1.5' : undefined}>
              {ingest.state === 'receiving' && ingest.trackCount !== null && (
                <p className="text-[11px] text-ink-faint">
                  {t('runtime:ingest.trackCount', { count: ingest.trackCount })}
                  {ingest.tracks.length > 0 && <> &middot; {ingest.tracks.join(', ')}</>}
                </p>
              )}

              {/* Ingest-engine service state - secondary, smaller than the
                  real connection state above. */}
              <p className="mt-1.5 flex items-center gap-1.5 text-[11px] text-ink-faint">
                <StatusDot status={mediaMtxTone(mediaMtx.state)} />
                {t(mediaMtxStateKey(mediaMtx.state))}
              </p>

              <div className="mt-2.5">
                <RuntimeControls mediaMtx={mediaMtx} compact />
              </div>

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
                  {/* Stated explicitly so nobody treats the local route name
                      like a destination platform key. */}
                  <p className="text-[10px] leading-relaxed text-ink-faint">
                    {t('runtime:connection.notASecret')}
                  </p>
                </div>
              )}

              <p className="mt-2.5 border-t border-line pt-2 text-[10px] text-ink-faint">
                {t('runtime:mediamtx.componentName')} {mediaMtx.supportedVersion}
              </p>
            </div>
          </>
        )}
      </section>

      {aboutQuery.data !== undefined && (
        <p className="px-1 text-[10px] text-ink-faint">{aboutVersionLine(tAbout, aboutQuery.data)}</p>
      )}
    </div>
  );
}
