import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { PlatformCard } from './PlatformCard';

type PlatformGridProps = {
  platforms: readonly ConfiguredPlatform[];
  onOpenSettings: (id: string) => void;
  onEditMetadata: (id: string) => void;
};

/** Responsive card grid: one column on phones, two from `sm` upwards. */
export function PlatformGrid({ platforms, onOpenSettings, onEditMetadata }: PlatformGridProps) {
  return (
    <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
