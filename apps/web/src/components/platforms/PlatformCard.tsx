import { AlertTriangle, Loader2, Radio, Settings2, SlidersHorizontal, VideoOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { ProviderBrand } from '@/components/providers/ProviderBrand';
import { useCredentialStatusQuery } from '@/hooks/use-credentials';
import { useBranchRuntimeQuery } from '@/hooks/use-branches';
import { useLanguage } from '@/i18n/use-language';
import { cn } from '@/lib/cn';
import { formatViewers } from '@/lib/format';
import { branchFor, branchStateKey, branchTone } from '@/models/branch-presentation';
import { presentCredentialStatus } from '@/models/credential-presentation';
import { categoryFieldLabelKey, providerHeroClass } from '@/models/provider-labels';
import type { PlatformStatus } from '@/models/platform';

import { BranchControls } from './BranchControls';
import { IconButton } from '../ui/Button';
import { StatusBadge } from '../ui/StatusBadge';

/** Icon for the hero band's centred state indicator - reuses the same
 * live/starting/error/offline tone every status badge in this codebase
 * already uses, never a fake preview thumbnail. */
const HERO_STATE_ICON: Record<PlatformStatus, typeof Radio> = {
  live: Radio,
  starting: Loader2,
  error: AlertTriangle,
  offline: VideoOff,
};

/** Text colour for the dl's own "Status" value - the same tone every other
 * status indicator on this card already uses. */
const STATUS_TEXT_TONE: Record<PlatformStatus, string> = {
  live: 'text-status-live',
  starting: 'text-status-starting',
  error: 'text-status-error',
  offline: 'text-ink-muted',
};

type PlatformCardProps = {
  platform: ConfiguredPlatform;
  onOpenSettings: (id: string) => void;
  onEditMetadata: (id: string) => void;
};

/**
 * One configured destination branch.
 *
 * Configuration (provider, display name, enabled state, credential status)
 * and real branch runtime state (see `BranchControls`) are shown together,
 * but they stay visibly distinct facts: a card can show a stream key
 * "Stored" while the branch itself is "Blocked" for an unrelated reason, or
 * "Sending" only once the backend reports real, advancing FFmpeg output -
 * never a fake viewer count or connection quality.
 *
 * The title and category shown here are user-authored content, rendered
 * verbatim and never translated.
 */
export function PlatformCard({ platform, onOpenSettings, onEditMetadata }: PlatformCardProps) {
  const { t } = useTranslation(['platforms', 'common', 'runtime']);
  const { locale } = useLanguage();

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

  // Non-sensitive: configured/missing status only, never the key itself.
  const credentialStatus = useCredentialStatusQuery(platform.id);
  const credentialPresentation = presentCredentialStatus(
    credentialStatus.data,
    credentialStatus.isLoading,
  );

  // Shares one underlying request/cache entry across every rendered card,
  // since every card queries the same ['branches'] key.
  const branchesQuery = useBranchRuntimeQuery();
  const branch = branchFor(branchesQuery.data, platform.id);
  const state = branch?.state ?? 'idle';
  const tone = branchTone(state);
  const HeroIcon = HERO_STATE_ICON[tone];

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
          'absolute inset-y-0 left-0 w-0.5 z-10',
          platform.enabled ? 'bg-accent/70' : 'bg-status-offline/50',
        )}
      />

      {/*
       * Decorative header band: a per-provider gradient wash plus a large,
       * low-opacity brand watermark, with the destination's real branch
       * state (never a fake preview/thumbnail) centred on top. This is the
       * card's visual anchor - the reference direction's card art occupies
       * real weight here, but everything shown is either pure decoration
       * (the gradient/watermark) or real backend state (the icon/label).
       */}
      <div
        className={cn(
          'relative flex h-24 items-center justify-center overflow-hidden border-b border-line',
          providerHeroClass(platform.providerId),
        )}
      >
        <ProviderBrand
          providerId={platform.providerId}
          fallbackLabel={shortLabel}
          size="xl"
          bare
          className="pointer-events-none absolute -right-3 -bottom-3 opacity-[0.14] blur-[0.5px]"
        />
        <div className="relative flex flex-col items-center gap-1.5">
          <HeroIcon
            aria-hidden="true"
            className={cn(
              'size-6',
              tone === 'live' && 'text-status-live',
              tone === 'starting' && 'animate-spin text-status-starting',
              tone === 'error' && 'text-status-error',
              tone === 'offline' && 'text-ink-faint',
            )}
          />
          <span
            className={cn(
              'text-[11px] font-semibold uppercase tracking-wide',
              tone === 'live' && 'text-status-live',
              tone === 'starting' && 'text-status-starting',
              tone === 'error' && 'text-status-error',
              tone === 'offline' && 'text-ink-faint',
            )}
          >
            {t(`runtime:${branchStateKey(state)}`)}
          </span>
        </div>
      </div>

      <div className="flex items-start justify-between gap-3 p-4 pb-3">
        <div className="flex min-w-0 items-center gap-3">
          <ProviderBrand providerId={platform.providerId} fallbackLabel={shortLabel} size="lg" />
          <div className="min-w-0">
            {/* User-chosen destination name; brand name sits underneath it. */}
            <h3
              id={`platform-${platform.id}-name`}
              className="truncate text-base font-semibold text-ink"
              title={platform.displayName}
            >
              {platform.displayName}
            </h3>
            <p className="flex items-center gap-1.5 truncate text-xs text-ink-faint">
              {brandName}
              <span aria-hidden="true">·</span>
              {platform.enabled ? t('platforms:card.enabled') : t('platforms:card.disabled')}
            </p>
          </div>
        </div>

        <StatusBadge status={tone} label={t(`runtime:${branchStateKey(state)}`)} className="shrink-0" />
      </div>

      <div className="space-y-3 px-4 pb-3">
        <p className="line-clamp-2 min-h-10 text-sm text-ink" title={platform.metadata.title}>
          {hasTitle ? (
            platform.metadata.title
          ) : (
            <span className="text-ink-faint italic">{t('platforms:card.noTitle')}</span>
          )}
        </p>

        <dl className="grid grid-cols-2 gap-x-3 gap-y-2.5">
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
              {t('platforms:card.streamKeyLabel')}
            </dt>
            <dd className="mt-0.5 truncate text-xs text-ink-muted">
              {t(credentialPresentation.labelKey)}
            </dd>
          </div>
          {/*
           * "Viewers" has no real backend source anywhere in this
           * application (audited exhaustively: no Twitch/YouTube/Kick
           * viewer-count API call exists, and the DB layer explicitly never
           * persists one - docs/project-overview.md). `formatViewers` was
           * built for exactly this honest case: it returns the locale's own
           * "--" placeholder for `null` rather than a fabricated number.
           * Real per-destination viewer counts would need genuine new
           * backend polling work this task does not add.
           */}
          <div className="min-w-0">
            <dt className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('platforms:card.viewersLabel')}
            </dt>
            <dd className="mt-0.5 truncate text-xs text-ink-muted">
              {formatViewers(null, locale)}
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('platforms:card.statusLabel')}
            </dt>
            <dd className={cn('mt-0.5 truncate text-xs font-medium', STATUS_TEXT_TONE[tone])}>
              {t(`runtime:${branchStateKey(state)}`)}
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
        <div className="min-w-0">
          <BranchControls platformId={platform.id} branch={branch} hideStatus />
        </div>

        <div className="flex shrink-0 items-start gap-1">
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
