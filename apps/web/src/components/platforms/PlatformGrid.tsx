import type { PlatformId, StreamPlatform } from '@/models/platform';

import { PlatformCard } from './PlatformCard';

type PlatformGridProps = {
  platforms: readonly StreamPlatform[];
  onStart: (id: PlatformId) => void;
  onStop: (id: PlatformId) => void;
  onConfigure: (id: PlatformId) => void;
};

/**
 * Responsive card grid: one column on phones, two from `sm`, back to two on
 * `xl` because the status rail takes the remaining width there.
 */
export function PlatformGrid({ platforms, onStart, onStop, onConfigure }: PlatformGridProps) {
  return (
    <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      {platforms.map((platform) => (
        <li key={platform.id} className="animate-fade-rise">
          <PlatformCard
            platform={platform}
            onStart={onStart}
            onStop={onStop}
            onConfigure={onConfigure}
          />
        </li>
      ))}
    </ul>
  );
}
