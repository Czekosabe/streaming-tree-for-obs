import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/cn';
import type { PlatformId, StreamPlatform } from '@/models/platform';

import { StatusDot } from '../ui/StatusBadge';

type PlatformTabsProps = {
  platforms: readonly StreamPlatform[];
  activeId: PlatformId;
  onSelect: (id: PlatformId) => void;
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
            <StatusDot status={platform.status} />
            {platform.name}
          </button>
        );
      })}
    </div>
  );
}
