import type { ParseKeys } from 'i18next';
import { Bell, MonitorPlay, Target, Volume2 } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

import { OverlayUrlPanel } from '@/components/overlays/OverlayUrlPanel';
import { useChatOverlaysQuery } from '@/hooks/use-chat-overlay';

type ToolLink = { to: string; icon: LucideIcon; labelKey: ParseKeys<'onboarding'> };

const TOOL_LINKS: readonly ToolLink[] = [
  { to: '/chat', icon: MonitorPlay, labelKey: 'creatorTools.links.chatOverlay' },
  { to: '/alerts', icon: Bell, labelKey: 'creatorTools.links.alerts' },
  { to: '/goals', icon: Target, labelKey: 'creatorTools.links.goals' },
  { to: '/audio', icon: Volume2, labelKey: 'creatorTools.links.audio' },
];

/**
 * Step 6: creator tools discovery (docs/onboarding.md §7.1).
 *
 * Concise links to real, shipped features only - never a Planned/
 * Deferred connector presented as available. This is discovery, not
 * configuration: no subsystem is set up here. Where a real chat overlay
 * profile already exists, reuses `OverlayUrlPanel` (the same "Copy
 * Browser Source URL" component the Overlays page itself uses) for the
 * first one; where none exists, points at the Overlays page to create
 * one first rather than inventing a URL.
 */
export function CreatorToolsStep() {
  const { t } = useTranslation('onboarding');
  const overlaysQuery = useChatOverlaysQuery();
  const overlays = overlaysQuery.data ?? [];
  const firstOverlay = overlays[0];

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight text-ink">
        {t('creatorTools.heading')}
      </h2>
      <p className="text-sm leading-relaxed text-ink-muted">{t('creatorTools.body')}</p>

      <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {TOOL_LINKS.map(({ to, icon: Icon, labelKey }) => (
          <li key={to}>
            <Link
              to={to}
              className="flex items-center gap-2.5 rounded-lg border border-line bg-surface-sunken px-3 py-2.5 text-sm text-ink transition-colors hover:bg-surface-hover"
            >
              <Icon aria-hidden="true" className="size-4 shrink-0 text-accent-soft" />
              <span className="truncate">{t(labelKey)}</span>
            </Link>
          </li>
        ))}
      </ul>

      {firstOverlay !== undefined ? (
        <OverlayUrlPanel overlayId={firstOverlay.id} publicSlug={firstOverlay.publicSlug} />
      ) : (
        <p className="text-sm text-ink-faint">
          {t('creatorTools.noOverlayYet')}{' '}
          <Link to="/overlays" className="font-medium text-accent-soft underline-offset-2 hover:underline">
            {t('creatorTools.createOverlayAction')}
          </Link>
        </p>
      )}
    </div>
  );
}
