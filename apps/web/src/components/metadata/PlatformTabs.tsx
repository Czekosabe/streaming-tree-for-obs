import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { ProviderBrand } from '@/components/providers/ProviderBrand';
import { cn } from '@/lib/cn';

type PlatformTabsProps = {
  platforms: readonly ConfiguredPlatform[];
  activeId: string;
  onSelect: (id: string) => void;
};

/**
 * Tab strip following the WAI-ARIA tabs pattern: arrow keys move between tabs,
 * only the selected tab is in the tab order, and each tab points at its panel.
 */
export function PlatformTabs({ platforms, activeId, onSelect }: PlatformTabsProps) {
  const { t } = useTranslation('metadata');
  const activeIndex = platforms.findIndex((platform) => platform.id === activeId);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    const lastIndex = platforms.length - 1;
    let nextIndex: number | null = null;

    if (event.key === 'ArrowRight') nextIndex = activeIndex >= lastIndex ? 0 : activeIndex + 1;
    if (event.key === 'ArrowLeft') nextIndex = activeIndex <= 0 ? lastIndex : activeIndex - 1;
    if (event.key === 'Home') nextIndex = 0;
    if (event.key === 'End') nextIndex = lastIndex;
    if (nextIndex === null) return;

    event.preventDefault();
    const next = platforms[nextIndex];
    if (next === undefined) return;
    onSelect(next.id);
    document.getElementById(`metadata-tab-${next.id}`)?.focus();
  };

  return (
    <div
      role="tablist"
      aria-label={t('editor.tabsLabel')}
      className="flex gap-1 overflow-x-auto border-b border-line px-2"
    >
      {platforms.map((platform) => {
        const isActive = platform.id === activeId;
        return (
          <button
            key={platform.id}
            id={`metadata-tab-${platform.id}`}
            role="tab"
            type="button"
            aria-selected={isActive}
            aria-controls={`metadata-panel-${platform.id}`}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onSelect(platform.id)}
            onKeyDown={handleKeyDown}
            className={cn(
              'flex shrink-0 items-center gap-2 border-b-2 px-3 py-2.5 text-xs font-medium',
              'transition-colors duration-150',
              isActive
                ? 'border-accent text-ink'
                : 'border-transparent text-ink-muted hover:border-line-strong hover:text-ink',
            )}
          >
            <ProviderBrand
              providerId={platform.providerId}
              fallbackLabel={platform.providerId.slice(0, 2).toUpperCase()}
              size="sm"
              className="size-5 rounded"
            />
            <span
              aria-hidden="true"
              className={cn(
                'size-1.5 rounded-full',
                platform.enabled ? 'bg-accent-soft' : 'bg-status-offline',
              )}
            />
            {/* User-chosen destination name, rendered verbatim. */}
            <span className="max-w-40 truncate">{platform.displayName}</span>
          </button>
        );
      })}
    </div>
  );
}
