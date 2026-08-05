import { useTranslation } from 'react-i18next';

import type { EngagementEvent } from '@/api/engagement-schemas';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { useEngagementStream } from '@/hooks/use-engagement-stream';
import { eventSummary, eventTypeKey } from '@/models/engagement-presentation';

/**
 * Bounded diagnostic feed of recently received normalized events.
 *
 * This is explicitly NOT the unified operator chat or an OBS overlay - no
 * message bubbles, no theming, no per-platform styling, no animation. It is
 * a plain, safe list for verifying the connector actually works, per the
 * stage task's own instruction not to present it as the finished product.
 */
export function RecentEventsFeed() {
  const { t } = useTranslation('engagement');
  const { events, status, gapDetected } = useEngagementStream();

  return (
    <Panel>
      <PanelHeader
        title={t('feed.title')}
        description={t('feed.description')}
        actions={<StreamStatusChip status={status} />}
      />
      <PanelBody className="space-y-2">
        {gapDetected && <p className="text-[11px] text-status-starting">{t('feed.gapNotice')}</p>}
        {events.length === 0 ? (
          <p className="text-xs text-ink-faint">{t('feed.empty')}</p>
        ) : (
          <ul className="max-h-96 space-y-1.5 overflow-y-auto text-xs">
            {[...events].reverse().map((event) => (
              <EventRow key={event.id} event={event} />
            ))}
          </ul>
        )}
      </PanelBody>
    </Panel>
  );
}

function StreamStatusChip({ status }: { status: 'connecting' | 'open' | 'error' | 'closed' }) {
  const { t } = useTranslation('engagement');
  const labelKey =
    status === 'open'
      ? 'feed.streamStatus.open'
      : status === 'connecting'
        ? 'feed.streamStatus.connecting'
        : status === 'error'
          ? 'feed.streamStatus.error'
          : 'feed.streamStatus.closed';
  return <span className="text-[11px] text-ink-faint">{t(labelKey)}</span>;
}

function EventRow({ event }: { event: EngagementEvent }) {
  const { t } = useTranslation('engagement');
  const summary = eventSummary(event);
  const time = new Date(event.receivedAt).toLocaleTimeString();

  return (
    <li className="flex items-baseline gap-2 rounded-md border border-line/60 px-2 py-1.5">
      <span className="shrink-0 tabular-nums text-ink-faint">{time}</span>
      <span className="shrink-0 rounded bg-surface-raised px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-ink-muted">
        {t(eventTypeKey(event.type))}
      </span>
      {summary.actor !== '' && <span className="shrink-0 font-medium text-ink">{summary.actor}</span>}
      {event.user?.anonymous === true && (
        <span className="shrink-0 italic text-ink-faint">{t('feed.anonymous')}</span>
      )}
      {summary.detail !== '' && <span className="truncate text-ink-muted">{summary.detail}</span>}
      {event.moderationRef !== undefined && (
        <span className="shrink-0 text-status-error">{t('feed.moderated')}</span>
      )}
      {event.synthetic && <span className="shrink-0 text-ink-faint">{t('feed.synthetic')}</span>}
    </li>
  );
}
