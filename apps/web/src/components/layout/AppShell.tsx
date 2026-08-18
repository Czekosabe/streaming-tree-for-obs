import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { UpdateBanner } from '@/components/system/UpdateBanner';

import { DesktopSidebar, MobileSidebar } from './Sidebar';
import { TopBar } from './TopBar';

type AppShellProps = {
  title: string;
  description: string;
  actions?: ReactNode;
  children: ReactNode;
};

/**
 * Application frame shared by every route.
 *
 * Layout: fixed sidebar on `lg+`, off-canvas drawer below it. The optional
 * right-hand status rail is a per-page concern (see `DashboardPage`) because it
 * only exists on the dashboard.
 */
export function AppShell({ title, description, actions, children }: AppShellProps) {
  const { t } = useTranslation('common');
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="flex min-h-dvh bg-canvas">
      <a href="#main-content" className="skip-link">
        {t('app.skipToContent')}
      </a>

      <DesktopSidebar />
      <MobileSidebar open={menuOpen} onClose={() => setMenuOpen(false)} />

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar
          title={title}
          description={description}
          actions={actions}
          onOpenMenu={() => setMenuOpen(true)}
        />
        <UpdateBanner />
        <main id="main-content" tabIndex={-1} className="flex-1 px-4 py-5 sm:px-6 sm:py-6">
          {children}
        </main>
      </div>
    </div>
  );
}
