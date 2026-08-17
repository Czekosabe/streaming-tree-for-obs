import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { PublicWidgetSnapshot, SupporterItem, TickerItem } from '@/api/goals-schemas';
import { cn } from '@/lib/cn';
import { formatAmountMicros } from '@/models/alerts';

function supportsMatchMedia(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function';
}

/** Tracks the `prefers-reduced-motion` media query - mirrors
 * components/alerts/AlertRenderer.tsx's own identical hook. */
function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(
    () => supportsMatchMedia() && window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  );
  useEffect(() => {
    if (!supportsMatchMedia()) return;
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = () => setReduced(query.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);
  return reduced;
}

function formatGoalValue(kind: PublicWidgetSnapshot['goalKind'], value: number, currency: string | undefined): string {
  if (kind === 'donations') {
    const amount = formatAmountMicros(value);
    return currency ? `${amount} ${currency}` : amount;
  }
  return value.toLocaleString();
}

function fontClassFor(fontFamily: string): string {
  if (fontFamily === 'serif') return 'font-serif';
  if (fontFamily === 'monospace') return 'font-mono';
  return 'font-sans';
}

function alignClassFor(textAlign: string): string {
  if (textAlign === 'left') return 'items-start text-left';
  if (textAlign === 'right') return 'items-end text-right';
  return 'items-center text-center';
}

/**
 * The shared bounded-style card frame every Stage 18A/18B widget kind
 * renders inside - styled entirely from a snapshot's own `presentation`
 * fields (docs/supporter-widgets.md §31), never arbitrary CSS, never a
 * `visualdesign.Document`.
 */
function WidgetFrame({ snapshot, children }: { snapshot: PublicWidgetSnapshot; children: React.ReactNode }) {
  const { presentation } = snapshot;
  return (
    <div
      className={cn(
        'flex w-full flex-col gap-2 p-3',
        fontClassFor(presentation.fontFamily),
        alignClassFor(presentation.textAlign),
        presentation.orientation === 'vertical' && 'max-w-xs',
      )}
      style={{
        backgroundColor: presentation.backgroundColor,
        color: presentation.foregroundColor,
        borderColor: presentation.borderColor,
        borderWidth: 1,
        borderStyle: 'solid',
        borderRadius: presentation.borderRadiusPx,
        opacity: presentation.opacity,
      }}
    >
      {children}
    </div>
  );
}

function formatObservedAt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

function ItemAmount({ item }: { item: SupporterItem }) {
  if (item.amountMicros !== undefined && item.currency !== undefined) {
    return <span className="tabular-nums">{formatAmountMicros(item.amountMicros)} {item.currency}</span>;
  }
  if (item.quantity !== undefined) {
    return <span className="tabular-nums">{item.quantity.toLocaleString()}</span>;
  }
  return null;
}

function GoalWidget({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const reducedMotion = usePrefersReducedMotion();
  const { presentation } = snapshot;
  const progress = snapshot.progressBasisPoints ?? 0;
  const fillPercent = Math.min(100, Math.max(0, progress / 100));
  const percentLabel = `${Math.round(progress) / 100}%`;
  const current = snapshot.current ?? 0;
  const target = snapshot.target ?? 0;

  return (
    <WidgetFrame snapshot={snapshot}>
      <div className="flex w-full items-baseline justify-between gap-2">
        <span className="truncate text-sm font-semibold">{snapshot.title}</span>
        {snapshot.completed === true && <span className="shrink-0 text-xs font-medium">&#10003;</span>}
      </div>

      {(presentation.showCurrent || presentation.showTarget) && (
        <div className="text-xs tabular-nums opacity-90">
          {presentation.showCurrent && formatGoalValue(snapshot.goalKind, current, snapshot.currency)}
          {presentation.showCurrent && presentation.showTarget && ' / '}
          {presentation.showTarget && formatGoalValue(snapshot.goalKind, target, snapshot.currency)}
        </div>
      )}

      <div className="h-2 w-full overflow-hidden rounded-full bg-black/20">
        <div
          className={cn('h-full rounded-full', !reducedMotion && 'transition-[width] duration-500 ease-out')}
          style={{ width: `${fillPercent}%`, backgroundColor: presentation.fillColor }}
        />
      </div>

      {presentation.showPercent && <span className="text-[11px] tabular-nums opacity-80">{percentLabel}</span>}
    </WidgetFrame>
  );
}

function LatestWidget({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const { t } = useTranslation('goals');
  const { presentation } = snapshot;
  const item = snapshot.latest;

  return (
    <WidgetFrame snapshot={snapshot}>
      <span className="truncate text-sm font-semibold">{snapshot.title}</span>
      {item === undefined ? (
        <span className="text-xs opacity-70">{t(`widgets.empty.${snapshot.kind}`)}</span>
      ) : (
        <div className="flex flex-col gap-0.5">
          <span className="text-base font-medium">{item.displayName ?? t('widgets.anonymous')}</span>
          {snapshot.kind === 'latest_donation' && (
            <span className="text-sm opacity-90">
              <ItemAmount item={item} />
            </span>
          )}
          {snapshot.kind === 'latest_donation' && item.message !== undefined && (
            <span className="text-xs italic opacity-80">&ldquo;{item.message}&rdquo;</span>
          )}
          <span className="text-[11px] opacity-70">
            {presentation.showProvider === true && item.provider !== undefined && item.provider}
            {presentation.showProvider === true && presentation.showTime === true && item.provider !== undefined && ' · '}
            {presentation.showTime === true && formatObservedAt(item.observedAt)}
          </span>
        </div>
      )}
    </WidgetFrame>
  );
}

function LargestDonationWidget({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const { t } = useTranslation('goals');
  const item = snapshot.largest;
  return (
    <WidgetFrame snapshot={snapshot}>
      <span className="truncate text-sm font-semibold">{snapshot.title}</span>
      <span className="text-[10px] uppercase tracking-wide opacity-60">{t('widgets.sessionScope')}</span>
      {item === undefined ? (
        <span className="text-xs opacity-70">{t('widgets.empty.largest_donation')}</span>
      ) : (
        <div className="flex flex-col gap-0.5">
          <span className="text-base font-medium">{item.displayName ?? t('widgets.anonymous')}</span>
          <span className="text-sm opacity-90">
            <ItemAmount item={item} />
          </span>
        </div>
      )}
    </WidgetFrame>
  );
}

function RecentSupportersWidget({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const { t } = useTranslation('goals');
  const items = snapshot.recent ?? [];
  return (
    <WidgetFrame snapshot={snapshot}>
      <span className="truncate text-sm font-semibold">{snapshot.title}</span>
      {items.length === 0 ? (
        <span className="text-xs opacity-70">{t('widgets.empty.recent_supporters')}</span>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.map((item) => (
            <li key={item.itemId} className="flex items-center justify-between gap-2 text-xs">
              <span className="truncate">{item.displayName ?? t('widgets.anonymous')}</span>
              <ItemAmount item={item} />
            </li>
          ))}
        </ul>
      )}
    </WidgetFrame>
  );
}

function TickerRow({ item }: { item: TickerItem }) {
  const { t } = useTranslation('goals');
  return (
    <li className="flex items-center justify-between gap-2 text-xs">
      <span className="truncate">
        {item.displayName ?? t('widgets.anonymous')} <span className="opacity-60">{t(`widgets.eventType.${item.eventType}`)}</span>
      </span>
      <ItemAmount item={item} />
    </li>
  );
}

function EventTickerWidget({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const { t } = useTranslation('goals');
  const items = snapshot.ticker ?? [];
  return (
    <WidgetFrame snapshot={snapshot}>
      <span className="truncate text-sm font-semibold">{snapshot.title}</span>
      {items.length === 0 ? (
        <span className="text-xs opacity-70">{t('widgets.empty.event_ticker')}</span>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.map((item) => (
            <TickerRow key={item.itemId} item={item} />
          ))}
        </ul>
      )}
    </WidgetFrame>
  );
}

function SessionCounterWidget({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const { t } = useTranslation('goals');
  const value = snapshot.counter ?? 0;
  return (
    <WidgetFrame snapshot={snapshot}>
      <span className="truncate text-sm font-semibold">{snapshot.title}</span>
      <span className="text-2xl font-bold tabular-nums">{value.toLocaleString()}</span>
      <span className="text-[10px] uppercase tracking-wide opacity-60">{t('widgets.sessionScope')}</span>
    </WidgetFrame>
  );
}

function DashboardWidget({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const children = snapshot.dashboard ?? [];
  const columns = snapshot.presentation.columns ?? 1;
  return (
    <div className="grid w-full gap-2" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
      {children.map((child) => (
        <div key={child.key} style={{ gridColumn: `${child.column} / span ${child.columnSpan}`, gridRow: `${child.row} / span ${child.rowSpan}` }}>
          <GoalWidgetRenderer snapshot={child.snapshot} />
        </div>
      ))}
    </div>
  );
}

/**
 * The one shared Stage 18A/18B widget renderer, dispatching by
 * `snapshot.kind` (docs/supporter-widgets.md §45) - used both by the
 * real public route (pages/PublicWidgetPage.tsx, which sizes its own
 * full-viewport wrapper) and every management page's own in-editor
 * preview - this component always fills 100% of whatever its parent
 * gives it, never a fixed viewport size of its own (mirrors
 * AlertRenderer/ChatOverlayRenderer's identical "one renderer, two call
 * sites" convention). A dashboard recurses into this same component for
 * each of its own children - never more than one level deep, since a
 * dashboard can never contain another dashboard.
 */
export function GoalWidgetRenderer({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  switch (snapshot.kind) {
    case 'goal':
      return <GoalWidget snapshot={snapshot} />;
    case 'latest_follower':
    case 'latest_subscriber':
    case 'latest_donation':
      return <LatestWidget snapshot={snapshot} />;
    case 'largest_donation':
      return <LargestDonationWidget snapshot={snapshot} />;
    case 'recent_supporters':
      return <RecentSupportersWidget snapshot={snapshot} />;
    case 'event_ticker':
      return <EventTickerWidget snapshot={snapshot} />;
    case 'session_counter':
      return <SessionCounterWidget snapshot={snapshot} />;
    case 'dashboard':
      return <DashboardWidget snapshot={snapshot} />;
    default:
      return null;
  }
}
