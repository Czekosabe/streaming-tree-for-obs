import { Plus } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AddPlatformDialog } from '@/components/platforms/AddPlatformDialog';
import { Button } from '@/components/ui/Button';
import { StatusDot } from '@/components/ui/StatusBadge';
import { usePlatformDefinitionsQuery, usePlatformsQuery } from '@/hooks/use-platforms';

/**
 * Step 4: destinations summary (docs/onboarding.md §6.4).
 *
 * Reuses `usePlatformsQuery`/`usePlatformDefinitionsQuery` and the exact
 * `AddPlatformDialog` the Dashboard itself uses for adding a
 * destination - never a second destination form. Zero, one, or many
 * destinations are all valid; nothing here requires a specific count to
 * proceed.
 */
export function DestinationsStep() {
  const { t } = useTranslation('onboarding');
  const platformsQuery = usePlatformsQuery();
  const definitionsQuery = usePlatformDefinitionsQuery();
  const [addOpen, setAddOpen] = useState(false);

  const platforms = platformsQuery.data ?? [];
  const definitions = definitionsQuery.data ?? [];

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight text-ink">
        {t('destinations.heading')}
      </h2>
      <p className="text-sm leading-relaxed text-ink-muted">{t('destinations.body')}</p>

      {platforms.length === 0 ? (
        <p className="text-sm text-ink-faint">{t('destinations.empty')}</p>
      ) : (
        <ul className="space-y-1.5">
          {platforms.map((platform) => (
            <li
              key={platform.id}
              className="flex items-center justify-between gap-2 rounded-lg border border-line bg-surface-sunken px-3 py-2 text-sm"
            >
              <span className="min-w-0 truncate text-ink">{platform.displayName}</span>
              <span className="flex shrink-0 items-center gap-1.5 text-xs text-ink-muted">
                <StatusDot status={platform.enabled ? 'live' : 'offline'} />
                {platform.enabled ? t('destinations.enabled') : t('destinations.disabled')}
              </span>
            </li>
          ))}
        </ul>
      )}

      <Button
        variant="secondary"
        icon={<Plus className="size-4" />}
        disabled={definitions.length === 0}
        onClick={() => setAddOpen(true)}
      >
        {t('destinations.addAction')}
      </Button>

      <AddPlatformDialog open={addOpen} onClose={() => setAddOpen(false)} definitions={definitions} />
    </div>
  );
}
