import { PlugZap, RefreshCw, ServerCrash, ServerCog } from 'lucide-react';

import { useHealthQuery } from '@/hooks/use-health-query';
import { ApiError } from '@/lib/api-client';
import { cn } from '@/lib/cn';
import { formatUptime } from '@/lib/format';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';
import { StatusBadge } from '../ui/StatusBadge';

/** Turns an unknown query error into one actionable sentence. */
function describeError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.kind) {
      case 'network':
        return 'The backend is not reachable. Start it with `go run ./cmd/server` in apps/server.';
      case 'timeout':
        return 'The backend did not answer in time. Check whether the process is still running.';
      case 'http':
        return `The backend answered with an error${error.status === null ? '' : ` (HTTP ${error.status})`}.`;
      case 'parse':
        return 'The backend answered with an unexpected payload. Frontend and backend versions may differ.';
      default:
        return error.message;
    }
  }
  return 'Unknown error while contacting the backend.';
}

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
  const { data, error, isPending, isError, isFetching, refetch } = useHealthQuery();

  return (
    <Panel>
      <PanelHeader
        title="Backend"
        description="Go REST API"
        icon={<ServerCog className="size-4" />}
        headingLevel={3}
        actions={
          <button
            type="button"
            onClick={() => void refetch()}
            aria-label="Check backend health again"
            title="Check again"
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
            Contacting the backend...
          </p>
        )}

        {isError && (
          <div className="space-y-2">
            <StatusBadge status="error" label="Backend unavailable" />
            <p className="flex gap-2 text-xs leading-relaxed text-ink-muted">
              <ServerCrash aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
              <span>{describeError(error)}</span>
            </p>
            <p className="text-[11px] text-ink-faint">
              The dashboard keeps working - demo values below are unaffected.
            </p>
          </div>
        )}

        {data !== undefined && !isError && (
          <div className="space-y-2">
            <StatusBadge
              status={data.status === 'ok' ? 'live' : 'error'}
              label={data.status === 'ok' ? 'Connected' : data.status}
            />
            <div className="space-y-1.5 border-t border-line pt-2">
              <Row label="Service" value={data.service} />
              <Row label="Version" value={data.version} />
              {data.uptimeSeconds !== undefined && (
                <Row label="Uptime" value={formatUptime(data.uptimeSeconds)} />
              )}
            </div>
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}
