import { Loader2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/**
 * Suspense fallback for a lazily-loaded route module (performance-
 * hardening pass: every major page under `ShellLayout`, the two
 * full-viewport designer pages, and the onboarding assistant are code-
 * split via `React.lazy` so a fresh load only downloads the Dashboard
 * up front - see `src/lib/lazy-page.ts` and `App.tsx`).
 *
 * Deliberately small and centered rather than a full-page skeleton: the
 * lazy chunk is typically already cached after the first visit, so this
 * is seen at most briefly on a route's first load. `min-h-40` keeps the
 * content area from visibly collapsing to zero height while it renders.
 * Rendered inside the routed `<Outlet>` only - `ShellLayout` itself (the
 * sidebar, the OBS connection panel) is never part of the lazy boundary
 * and stays mounted throughout.
 */
export function RouteLoadingFallback() {
  const { t } = useTranslation('common');

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex min-h-40 flex-1 items-center justify-center gap-2 p-6 text-sm text-ink-muted"
    >
      <Loader2 aria-hidden="true" className="size-4 animate-spin" />
      {t('loading.page')}
    </div>
  );
}
