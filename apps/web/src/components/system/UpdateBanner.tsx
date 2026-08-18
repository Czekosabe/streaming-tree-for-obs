import { X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

import { Button } from '@/components/ui/Button';
import { useUpdateStatusQuery } from '@/hooks/use-updates';
import { cn } from '@/lib/cn';

/**
 * Versions dismissed via "Later" for the remainder of this browser tab's
 * current load (docs/updater.md §32) - deliberately module-level, in-memory
 * state, not `localStorage`/`sessionStorage`: this application persists
 * exactly one thing client-side, the language preference (see
 * `i18n/config.ts`'s own doc comment) - "Later" is a transient UI
 * preference, not data, and resets on an ordinary page reload exactly like
 * "the current application process" naturally would.
 */
const dismissedVersions = new Set<string>();

/**
 * Non-blocking global "update available" banner (docs/updater.md §32),
 * mounted once in `AppShell` so it appears on every page. Shown only once a
 * Stable update is confirmed available or further along; never for a
 * development build (which never reaches that state at all - see
 * `updater.Manager`'s own `disabled` state).
 */
export function UpdateBanner({ className }: { className?: string }) {
  const { t } = useTranslation('updates');
  const { data: status } = useUpdateStatusQuery();
  const [, forceRerender] = useState(0);

  if (status === undefined || status.latestVersion === undefined) {
    return null;
  }
  if (status.state !== 'available' && status.state !== 'downloading' && status.state !== 'ready_to_install') {
    return null;
  }
  if (dismissedVersions.has(status.latestVersion)) {
    return null;
  }

  return (
    <div
      role="status"
      className={cn(
        'flex flex-wrap items-center justify-between gap-3 border-b border-accent/30 bg-accent/10 px-4 py-2 sm:px-6',
        className,
      )}
    >
      <p className="text-xs font-medium text-ink">
        {t('banner.message', { version: status.latestVersion })}
      </p>
      <div className="flex items-center gap-2">
        <Link
          to="/settings/about"
          className="text-xs font-medium text-accent-soft hover:underline"
        >
          {t('banner.updateNow')}
        </Link>
        <Button
          type="button"
          variant="ghost"
          className="h-7 px-2"
          onClick={() => {
            dismissedVersions.add(status.latestVersion as string);
            forceRerender((n) => n + 1);
          }}
          aria-label={t('banner.later')}
        >
          <X aria-hidden="true" className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}
