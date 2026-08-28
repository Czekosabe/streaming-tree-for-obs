import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { ProviderBrand } from '@/components/providers/ProviderBrand';
import { cn } from '@/lib/cn';

type PlatformTabsProps = {
  platforms: readonly ConfiguredPlatform[];
  activeId: string;
  onSelect: (id: string) => void;
  /** `horizontal` (default): a scrollable strip, underline-style active
   * indicator. `vertical`: a left-rail list of full-width rows with a
   * left-border active indicator - used by `MetadataEditor` at desktop
   * width, where a dedicated provider-switching column reads as a much
   * more deliberate workspace than a thin strip above a wide form. */
  orientation?: 'horizontal' | 'vertical';
};

/**
 * Tab strip following the WAI-ARIA tabs pattern: arrow keys move between tabs,
 * only the selected tab is in the tab order, and each tab points at its panel.
 */
export function PlatformTabs({
  platforms,
  activeId,
  onSelect,
  orientation = 'horizontal',
}: PlatformTabsProps) {
  const { t } = useTranslation('metadata');
  const activeIndex = platforms.findIndex((platform) => platform.id === activeId);
  const vertical = orientation === 'vertical';

  const handleKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    const lastIndex = platforms.length - 1;
    let nextIndex: number | null = null;

    const forwardKey = vertical ? 'ArrowDown' : 'ArrowRight';
    const backwardKey = vertical ? 'ArrowUp' : 'ArrowLeft';

    if (event.key === forwardKey) nextIndex = activeIndex >= lastIndex ? 0 : activeIndex + 1;
    if (event.key === backwardKey) nextIndex = activeIndex <= 0 ? lastIndex : activeIndex - 1;
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
      aria-orientation={vertical ? 'vertical' : undefined}
      className={cn(
        vertical
          ? 'flex flex-col gap-0.5 p-2'
          : 'flex gap-1 overflow-x-auto border-b border-line px-2',
      )}
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
              'flex shrink-0 items-center gap-2 text-xs font-medium transition-colors duration-150',
              vertical
                ? cn(
                    'w-full rounded-lg border-l-2 px-2.5 py-2 text-left',
                    isActive
                      ? 'border-accent bg-accent/10 text-ink'
                      : 'border-transparent text-ink-muted hover:bg-surface-hover hover:text-ink',
                  )
                : cn(
                    'border-b-2 px-3 py-2.5',
                    isActive
                      ? 'border-accent text-ink'
                      : 'border-transparent text-ink-muted hover:border-line-strong hover:text-ink',
                  ),
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
                'size-1.5 shrink-0 rounded-full',
                platform.enabled ? 'bg-accent-soft' : 'bg-status-offline',
              )}
            />
            {/* User-chosen destination name, rendered verbatim. */}
            <span className={cn('truncate', vertical ? 'min-w-0 flex-1' : 'max-w-40')}>
              {platform.displayName}
            </span>
          </button>
        );
      })}
    </div>
  );
}
