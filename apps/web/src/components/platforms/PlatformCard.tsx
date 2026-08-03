import { Ban, Play, Settings2, SlidersHorizontal } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { cn } from '@/lib/cn';
import { categoryFieldLabelKey, providerGlyphClass } from '@/models/provider-labels';

import { Button, IconButton } from '../ui/Button';
import { PlatformGlyph } from './PlatformGlyph';

type PlatformCardProps = {
  platform: ConfiguredPlatform;
  onOpenSettings: (id: string) => void;
  onEditMetadata: (id: string) => void;
};

/**
 * One configured destination branch.
 *
 * The card shows CONFIGURATION only. There is no streaming engine yet, so it
 * reports "configured / offline" and never a live state, no viewer count and no
 * connection quality - previously those were demo values, and showing them next
 * to real saved configuration would be misleading.
 *
 * Start is disabled with an explanation; the settings and metadata actions are
 * real and open editors backed by the API.
 *
 * The title and category shown here are user-authored content, rendered
 * verbatim and never translated.
 */
export function PlatformCard({ platform, onOpenSettings, onEditMetadata }: PlatformCardProps) {
  const { t } = useTranslation(['platforms', 'common']);

  const provider = platform.provider;
  const brandName = provider?.brandName ?? platform.providerId;
  const shortLabel = provider?.shortLabel ?? platform.providerId.slice(0, 2).toUpperCase();

  // An unknown category-field identifier falls back to the generic label rather
  // than blanking the row.
  const categoryKey =
    provider === undefined ? null : categoryFieldLabelKey(provider.categoryFieldType);
  const categoryLabel = categoryKey === null ? t('platforms:fields.category') : t(categoryKey);

  const hasTitle = platform.metadata.title.trim() !== '';
  const hasCategory = platform.metadata.category.trim() !== '';

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
        className={cn(
          'absolute inset-y-0 left-0 w-0.5',
          platform.enabled ? 'bg-accent/70' : 'bg-status-offline/50',
        )}
      />

      <div className="flex items-start justify-between gap-3 p-4 pb-3">
        <div className="flex min-w-0 items-center gap-3">
          <PlatformGlyph className={providerGlyphClass(platform.providerId)} label={shortLabel} />
          <div className="min-w-0">
            {/* User-chosen destination name; brand name sits underneath it. */}
            <h3
              id={`platform-${platform.id}-name`}
              className="truncate text-sm font-semibold text-ink"
              title={platform.displayName}
            >
              {platform.displayName}
            </h3>
            <p className="truncate text-[11px] text-ink-faint">{brandName}</p>
          </div>
        </div>

        <span
          className={cn(
            'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-0.5',
            'text-[11px] font-semibold uppercase tracking-wide',
            platform.enabled
              ? 'border-accent/40 bg-accent/12 text-accent-soft'
              : 'border-status-offline/40 bg-status-offline/12 text-status-offline',
          )}
        >
          <span
            aria-hidden="true"
            className={cn(
              'size-2 rounded-full',
              platform.enabled ? 'bg-accent-soft' : 'bg-status-offline',
            )}
          />
          {platform.enabled
            ? t('platforms:card.enabled')
            : t('platforms:card.disabled')}
        </span>
      </div>

      <div className="space-y-3 px-4 pb-3">
        <p className="line-clamp-2 min-h-9 text-sm text-ink" title={platform.metadata.title}>
          {hasTitle ? (
            platform.metadata.title
          ) : (
            <span className="text-ink-faint italic">{t('platforms:card.noTitle')}</span>
          )}
        </p>

        <dl className="grid grid-cols-2 gap-3">
          <div className="min-w-0">
            <dt className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {categoryLabel}
            </dt>
            <dd
              className="mt-0.5 truncate text-xs text-ink-muted"
              title={platform.metadata.category}
            >
              {hasCategory ? platform.metadata.category : t('common:values.empty')}
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('platforms:card.stateLabel')}
            </dt>
            <dd className="mt-0.5 truncate text-xs text-ink-muted">
              {t('platforms:card.offlineConfigured')}
            </dd>
          </div>
        </dl>

        {provider === undefined && (
          <p className="rounded-md border border-status-warning/30 bg-status-warning/10 px-2 py-1.5 text-[11px] text-status-warning">
            {t('platforms:card.unknownProvider', { providerId: platform.providerId })}
          </p>
        )}
      </div>

      <footer className="flex items-center justify-between gap-2 border-t border-line px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            disabled
            title={t('platforms:card.streamingNotImplemented')}
            icon={<Play className="size-3.5" />}
          >
            {t('platforms:card.start')}
          </Button>
          <span className="flex min-w-0 items-center gap-1 text-[10px] text-ink-faint">
            <Ban aria-hidden="true" className="size-3 shrink-0" />
            <span className="truncate">{t('platforms:card.streamingNotImplemented')}</span>
          </span>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <IconButton
            label={t('platforms:card.editMetadata', { platform: platform.displayName })}
            variant="ghost"
            onClick={() => onEditMetadata(platform.id)}
            icon={<SlidersHorizontal className="size-4" />}
          />
          <IconButton
            label={t('platforms:card.openSettings', { platform: platform.displayName })}
            variant="ghost"
            onClick={() => onOpenSettings(platform.id)}
            icon={<Settings2 className="size-4" />}
          />
        </div>
      </footer>
    </article>
  );
}
