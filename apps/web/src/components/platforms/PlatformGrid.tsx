import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { PlatformCard } from './PlatformCard';

type PlatformGridProps = {
  platforms: readonly ConfiguredPlatform[];
  onOpenSettings: (id: string) => void;
  onEditMetadata: (id: string) => void;
};

/**
 * Responsive card grid: one column on phones, growing up to three at wide
 * desktop width. Not hard-coded to any fixed destination count - the grid
 * simply wraps to as many columns as the viewport and card min-width allow.
 */
export function PlatformGrid({ platforms, onOpenSettings, onEditMetadata }: PlatformGridProps) {
  return (
    <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 2xl:grid-cols-3 min-[1800px]:grid-cols-4">
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
