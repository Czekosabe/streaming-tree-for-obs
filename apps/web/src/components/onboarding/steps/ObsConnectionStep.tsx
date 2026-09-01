import { useTranslation } from 'react-i18next';

import { CopyableValue } from '@/components/runtime/CopyableValue';
import { StatusDot } from '@/components/ui/StatusBadge';
import { useRuntimeQuery } from '@/hooks/use-runtime';
import { cn } from '@/lib/cn';
import { ingestStateKey, ingestTone } from '@/models/runtime-presentation';

/**
 * Step 3: the OBS Connection Assistant (docs/onboarding.md §6.3).
 *
 * Reuses `GET /api/runtime`'s real `connection`/`ingest` fields, the
 * same `CopyableValue` component `SidebarFooter` already uses for
 * exactly these two values, and the same `ingestStateKey`/`ingestTone`
 * presentation helpers - no new readiness/connection logic, no fake
 * "Test connection" button. The local ingest path is explicitly not a
 * secret (`runtime:connection.notASecret`, already shown elsewhere),
 * so no reveal/secret UX is needed here either.
 */
export function ObsConnectionStep() {
  const { t } = useTranslation(['onboarding', 'runtime']);
  const runtimeQuery = useRuntimeQuery();
  const connection = runtimeQuery.data?.connection;
  const ingest = runtimeQuery.data?.ingest;

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight text-ink">
        {t('obsConnection.heading')}
      </h2>
      <p className="text-sm leading-relaxed text-ink-muted">{t('obsConnection.body')}</p>

      {connection !== undefined && (
        <div className="space-y-2 rounded-lg border border-line bg-surface-sunken p-3">
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
          <p className="text-[11px] leading-relaxed text-ink-faint">
            {t('runtime:connection.notASecret')}
          </p>
        </div>
      )}

      <ol className="list-decimal space-y-1 pl-5 text-sm text-ink-muted">
        <li>{t('obsConnection.steps.openSettings')}</li>
        <li>{t('obsConnection.steps.selectStream')}</li>
        <li>{t('obsConnection.steps.setCustom')}</li>
        <li>{t('obsConnection.steps.pasteValues')}</li>
        <li>{t('obsConnection.steps.apply')}</li>
      </ol>

      {ingest !== undefined && (
        <p
          className={cn(
            'flex items-center gap-2 text-sm font-medium',
            ingest.state === 'receiving' ? 'text-status-live' : 'text-ink',
          )}
        >
          <StatusDot status={ingestTone(ingest.state)} />
          {t(`runtime:${ingestStateKey(ingest.state)}`)}
        </p>
      )}
    </div>
  );
}
