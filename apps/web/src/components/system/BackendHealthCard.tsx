import { PlugZap, RefreshCw, ServerCrash, ServerCog } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { useHealthQuery } from '@/hooks/use-health-query';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { cn } from '@/lib/cn';
import { toDurationParts } from '@/lib/format';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';
import { StatusBadge } from '../ui/StatusBadge';

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3 text-xs">
      <span className="text-ink-muted">{label}</span>
      <span className="truncate font-mono text-ink" title={value}>
        {value}
      </span>
    </div>
  );
}

/**
 * Live view of `GET /api/health` - the only real backend data in this stage.
 *
 * An unreachable backend is a normal, fully handled state: the card switches to
 * "Backend unavailable" and offers a retry instead of throwing.
 */
export function BackendHealthCard() {
  const { t } = useTranslation(['dashboard', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;
  const { data, error, isPending, isError, isFetching, refetch } = useHealthQuery();

  const uptime = data?.uptimeSeconds === undefined ? null : toDurationParts(data.uptimeSeconds);

  return (
    <Panel>
      <PanelHeader
        title={t('dashboard:backend.heading')}
        description={t('dashboard:backend.description')}
        icon={<ServerCog className="size-4" />}
        headingLevel={3}
        actions={
          <button
            type="button"
            onClick={() => void refetch()}
            aria-label={t('dashboard:backend.refresh')}
            title={t('dashboard:backend.refreshShort')}
            className="inline-flex size-7 items-center justify-center rounded-lg border border-line text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink"
          >
            <RefreshCw
              aria-hidden="true"
              className={cn('size-3.5', isFetching && 'animate-spin')}
            />
          </button>
        }
      />
      <PanelBody className="space-y-3">
        {isPending && (
          <p className="flex items-center gap-2 text-xs text-ink-muted">
            <PlugZap aria-hidden="true" className="size-3.5 animate-pulse" />
            {t('dashboard:backend.pending')}
          </p>
        )}

        {isError && (
          <div className="space-y-2">
            <StatusBadge status="error" label={t('dashboard:backend.unavailable')} />
            <p className="flex gap-2 text-xs leading-relaxed text-ink-muted">
              <ServerCrash aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
              <span>{resolveApiErrorMessage(tErrors, error)}</span>
            </p>
            <p className="text-[11px] text-ink-faint">{t('dashboard:backend.unaffected')}</p>
          </div>
        )}

        {data !== undefined && !isError && (
          <div className="space-y-2">
            <StatusBadge
              status={data.status === 'ok' ? 'live' : 'error'}
              label={data.status === 'ok' ? t('dashboard:backend.connected') : data.status}
            />
            <div className="space-y-1.5 border-t border-line pt-2">
              {/* Service name and version are API identifiers, not prose. */}
              <Row label={t('dashboard:backend.service')} value={data.service} />
              <Row label={t('dashboard:backend.version')} value={data.version} />
              {uptime !== null && (
                <Row
                  label={t('dashboard:backend.uptime')}
                  value={t(`common:duration.${uptime.unit}`, {
                    hours: uptime.hours,
                    minutes: uptime.minutes,
                    seconds: uptime.seconds,
                  })}
                />
              )}
            </div>
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}
