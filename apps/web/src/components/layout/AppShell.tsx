import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet, useOutletContext } from 'react-router-dom';

import { UpdateBanner } from '@/components/system/UpdateBanner';

import { DesktopSidebar, MobileSidebar } from './Sidebar';
import { TopBar } from './TopBar';

type ShellOutletContext = {
  onOpenMenu: () => void;
};

/**
 * Persistent application frame, mounted once as a router layout route.
 *
 * The sidebar and mobile drawer live here rather than inside `AppShell`
 * itself, specifically so `<Outlet>` swapping the routed page never remounts
 * them: every page used to render its own `<AppShell>` as its route
 * `element`, so React tore down and rebuilt the whole sidebar - including its
 * scroll position - on every navigation. Lifting the chrome one level above
 * the routed content, and letting only the content below it be swapped, is
 * what actually keeps the sidebar's scroll position and DOM identity stable
 * across routes.
 */
export function ShellLayout() {
  const { t } = useTranslation('common');
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="flex min-h-dvh bg-canvas">
      <a href="#main-content" className="skip-link">
        {t('app.skipToContent')}
      </a>

      <DesktopSidebar />
      <MobileSidebar open={menuOpen} onClose={() => setMenuOpen(false)} />

      <Outlet context={{ onOpenMenu: () => setMenuOpen(true) } satisfies ShellOutletContext} />
    </div>
  );
}

type AppShellProps = {
  title: string;
  description: string;
  actions?: ReactNode;
  children: ReactNode;
};

/**
 * Per-page content frame: top bar, update banner and the page body.
 *
 * Rendered by each page inside `ShellLayout`'s `<Outlet>`. `onOpenMenu` comes
 * from that outlet context; a page rendered without an enclosing
 * `ShellLayout` (unit tests, or a route deliberately kept outside it) falls
 * back to a no-op, which is harmless because there is then no mobile drawer
 * for it to open either.
 */
export function AppShell({ title, description, actions, children }: AppShellProps) {
  const outletContext = useOutletContext<ShellOutletContext | null>();
  const onOpenMenu = outletContext?.onOpenMenu ?? (() => {});

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <TopBar title={title} description={description} actions={actions} onOpenMenu={onOpenMenu} />
      <UpdateBanner />
      <main id="main-content" tabIndex={-1} className="flex-1 px-4 py-5 sm:px-6 sm:py-6">
        {children}
      </main>
    </div>
  );
}
