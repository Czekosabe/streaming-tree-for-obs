import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { cn } from '@/lib/cn';

import { PlatformCard } from './PlatformCard';
import { columnClassesFor } from './platform-grid-columns';

type PlatformGridProps = {
  platforms: readonly ConfiguredPlatform[];
  onOpenSettings: (id: string) => void;
  onEditMetadata: (id: string) => void;
};

/**
 * Responsive destination-card grid.
 *
 * Wrapped in its own `@container` so `columnClassesFor`'s breakpoints (see
 * that module's own doc comment) measure this grid's real available
 * width - which shrinks when the right rail is present and grows again
 * once it drops below the main content on narrower layouts - rather than
 * the full browser viewport.
 */
export function PlatformGrid({ platforms, onOpenSettings, onEditMetadata }: PlatformGridProps) {
  return (
    <div className="@container">
      <ul className={cn('grid gap-4', columnClassesFor(platforms.length))}>
        {platforms.map((platform) => (
          <li key={platform.id} className="animate-fade-rise min-w-0">
            <PlatformCard
              platform={platform}
              onOpenSettings={onOpenSettings}
              onEditMetadata={onEditMetadata}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}
