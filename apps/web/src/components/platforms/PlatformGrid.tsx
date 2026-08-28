import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { PlatformCard } from './PlatformCard';

type PlatformGridProps = {
  platforms: readonly ConfiguredPlatform[];
  onOpenSettings: (id: string) => void;
  onEditMetadata: (id: string) => void;
};

/**
 * Responsive card grid using CSS auto-fill rather than fixed breakpoint
 * column counts: it always packs as many ~260px-minimum cards per row as
 * the available width allows (naturally reaching four across at a common
 * wide-desktop width once the sidebar and right rail are accounted for),
 * and gracefully reduces down to one column on narrow viewports - never a
 * hard-coded destination count, and never a lone card stranded alone on
 * its own row purely because of an arbitrary column-count breakpoint.
 */
export function PlatformGrid({ platforms, onOpenSettings, onEditMetadata }: PlatformGridProps) {
  return (
    <ul className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-4">
      {platforms.map((platform) => (
        <li key={platform.id} className="animate-fade-rise">
          <PlatformCard
            platform={platform}
            onOpenSettings={onOpenSettings}
            onEditMetadata={onEditMetadata}
          />
        </li>
      ))}
    </ul>
  );
}
