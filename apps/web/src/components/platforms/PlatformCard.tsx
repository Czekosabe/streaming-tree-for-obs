import { Loader2, Play, Settings2, Square, Users, Wifi } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { useLanguage } from '@/i18n/use-language';
import { cn } from '@/lib/cn';
import { formatViewers } from '@/lib/format';
import {
  CONNECTION_QUALITY_LABEL_KEYS,
  type ConnectionQuality,
  type PlatformId,
  type StreamPlatform,
} from '@/models/platform';

import { Button, IconButton } from '../ui/Button';
import { StatusBadge } from '../ui/StatusBadge';
import { PlatformGlyph } from './PlatformGlyph';

const QUALITY_TEXT_CLASSES: Record<ConnectionQuality, string> = {
  excellent: 'text-status-live',
  good: 'text-status-live',
  fair: 'text-status-warning',
  poor: 'text-status-error',
  unknown: 'text-ink-faint',
};

/** Left accent stripe reflecting the branch status. */
const STATUS_STRIPE_CLASSES: Record<StreamPlatform['status'], string> = {
  live: 'bg-status-live',
  starting: 'bg-status-starting',
  error: 'bg-status-error',
  offline: 'bg-status-offline/50',
};

type PlatformCardProps = {
  platform: StreamPlatform;
  onStart: (id: PlatformId) => void;
  onStop: (id: PlatformId) => void;
  onConfigure: (id: PlatformId) => void;
};

function MetaRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {label}
      </dt>
      <dd className="mt-0.5 truncate text-xs text-ink-muted" title={value}>
        {value}
      </dd>
    </div>
  );
}

/**
 * One branch of the streaming tree.
 *
 * The Start/Stop button only mutates the local DEMO store - it does not spawn
 * FFmpeg, does not contact any platform and does not transmit anything. The
 * "Demo control" caption under the button states this in the UI itself.
 *
 * The stream title, category and tags shown here are user-authored content and
 * are rendered verbatim - they are never translated.
 */
export function PlatformCard({ platform, onStart, onStop, onConfigure }: PlatformCardProps) {
  const { t } = useTranslation(['platforms', 'common']);
  const { locale } = useLanguage();

  const isBusy = platform.status === 'starting';
  const isRunning = platform.status === 'live' || platform.status === 'starting';
  const showViewers = platform.status === 'live' && platform.viewers !== null;

  return (
    <article
      aria-labelledby={`platform-${platform.id}-name`}
      className={cn(
        'group relative overflow-hidden rounded-xl border border-line bg-surface shadow-panel',
        'transition-colors duration-200 hover:border-line-strong hover:bg-surface-raised',
      )}
    >
      <span
        aria-hidden="true"
        className={cn('absolute inset-y-0 left-0 w-0.5', STATUS_STRIPE_CLASSES[platform.status])}
      />

      <div className="flex items-start justify-between gap-3 p-4 pb-3">
        <div className="flex min-w-0 items-center gap-3">
          <PlatformGlyph id={platform.id} label={platform.shortLabel} />
          <div className="min-w-0">
            {/* Brand name - never translated. */}
            <h3
              id={`platform-${platform.id}-name`}
              className="truncate text-sm font-semibold text-ink"
            >
              {platform.name}
            </h3>
            <p className="truncate text-[11px] text-ink-faint">{t('platforms:card.ingestHint')}</p>
          </div>
        </div>
        <StatusBadge status={platform.status} />
      </div>

      <div className="space-y-3 px-4 pb-3">
        <p
          className="line-clamp-2 min-h-9 text-sm text-ink"
          title={platform.metadata.title}
        >
          {platform.metadata.title === '' ? (
            <span className="text-ink-faint italic">{t('platforms:card.noTitle')}</span>
          ) : (
            platform.metadata.title
          )}
        </p>

        <dl className="grid grid-cols-3 gap-3">
          <MetaRow
            label={t(platform.options.categoryLabelKey)}
            value={
              platform.metadata.category === ''
                ? t('common:values.empty')
                : platform.metadata.category
            }
          />
          <div className="min-w-0">
            <dt className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('platforms:card.viewers')}
            </dt>
            <dd className="mt-0.5 flex items-center gap-1 text-xs text-ink-muted">
              <Users aria-hidden="true" className="size-3 shrink-0" />
              <span
                className="font-mono tabular-nums"
                title={
                  showViewers && platform.viewers !== null
                    ? t('platforms:card.viewersAccessible', { count: platform.viewers })
                    : undefined
                }
              >
                {showViewers ? formatViewers(platform.viewers, locale) : t('common:values.empty')}
              </span>
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('platforms:card.quality')}
            </dt>
            <dd
              className={cn(
                'mt-0.5 flex items-center gap-1 text-xs',
                QUALITY_TEXT_CLASSES[platform.quality],
              )}
            >
              <Wifi aria-hidden="true" className="size-3 shrink-0" />
              <span className="truncate">{t(CONNECTION_QUALITY_LABEL_KEYS[platform.quality])}</span>
            </dd>
          </div>
        </dl>

        {platform.statusDetailKey !== null && (
          <p className="rounded-md border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-[11px] text-status-error">
            {t(platform.statusDetailKey)}
          </p>
        )}
      </div>

      <footer className="flex items-center justify-between gap-2 border-t border-line px-4 py-3">
        <div className="flex items-center gap-2">
          {isRunning ? (
            <Button
              variant="danger"
              size="sm"
              onClick={() => onStop(platform.id)}
              icon={<Square className="size-3.5" />}
            >
              {t('platforms:card.stop')}
            </Button>
          ) : (
            <Button
              variant="success"
              size="sm"
              onClick={() => onStart(platform.id)}
              icon={
                isBusy ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Play className="size-3.5" />
                )
              }
            >
              {t('platforms:card.start')}
            </Button>
          )}
          <span className="text-[10px] text-ink-faint">{t('platforms:card.demoControl')}</span>
        </div>

        <IconButton
          label={t('platforms:card.openMetadata', { platform: platform.name })}
          variant="ghost"
          onClick={() => onConfigure(platform.id)}
          icon={<Settings2 className="size-4" />}
        />
      </footer>
    </article>
  );
}
