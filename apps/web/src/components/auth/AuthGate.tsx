import type { ReactNode } from 'react';
import { useLocation } from 'react-router-dom';

import { useAuth } from '@/app/auth-context';
import { LoginPage } from '@/pages/LoginPage';

/** Every public overlay/Browser-Source route's own path prefix - see
 * router.go's own /api/public/* convention this mirrors on the
 * frontend side. Never gated, regardless of auth status
 * (docs/remote-management.md §35/§49). */
const PUBLIC_OVERLAY_PATH_PREFIX = '/overlay/';

/**
 * Gates the operator management UI behind Stage 20D2B authentication
 * (docs/remote-management.md §26/§46). Never gates the public overlay
 * routes (`/overlay/*`) - those keep their existing loopback/local
 * capability behavior unchanged.
 *
 * `status === 'not-applicable'` (remote management disabled, i.e.
 * every desktop and D2A-headless-only deployment today) renders
 * children immediately - zero UI change from before this stage.
 */
export function AuthGate({ children }: { children: ReactNode }) {
  const { status } = useAuth();
  const location = useLocation();

  if (location.pathname.startsWith(PUBLIC_OVERLAY_PATH_PREFIX)) {
    return <>{children}</>;
  }

  if (status === 'checking') {
    // A brief, unstyled blank frame while the one bootstrap request
    // resolves - deliberately minimal rather than a themed spinner,
    // since this state is expected to last well under a second on any
    // reachable backend.
    return null;
  }

  if (status === 'unauthenticated') {
    return <LoginPage />;
  }

  return <>{children}</>;
}
