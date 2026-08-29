import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { PlatformCard } from './PlatformCard';

type PlatformGridProps = {
  platforms: readonly ConfiguredPlatform[];
  onOpenSettings: (id: string) => void;
  onEditMetadata: (id: string) => void;
};

/**
 * Responsive card grid using CSS auto-fit rather than fixed breakpoint
 * column counts: it always packs as many ~280px-minimum cards per row as
 * the available width allows (naturally reaching four across at a common
 * wide-desktop width once the sidebar and right rail are accounted for),
 * and gracefully reduces down to one column on narrow viewports - never a
 * hard-coded destination count.
 *
 * `auto-fit` (not `auto-fill`) deliberately: with `auto-fill`, unfilled
 * grid tracks stay reserved-but-empty, so real cards never grow to use
 * that leftover width - visible as a large dead area whenever the
 * destination count doesn't exactly fill a row, a real physical finding
 * against an earlier build of this grid. `auto-fit` collapses those empty
 * tracks instead, so the real cards' own `1fr` stretches to fill the row
 * with no unexplained blank region.
 */
export function PlatformGrid({ platforms, onOpenSettings, onEditMetadata }: PlatformGridProps) {
  return (
    <ul className="grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-4">
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
