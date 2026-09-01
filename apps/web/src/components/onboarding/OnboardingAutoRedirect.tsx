import { useEffect, useRef } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { useOnboardingStateQuery } from '@/hooks/use-onboarding';

/** Never auto-redirected into onboarding - matches AuthGate's own
 * "public overlay routes are never gated" convention. */
const PUBLIC_OVERLAY_PATH_PREFIX = '/overlay/';

/**
 * Auto-shows the onboarding assistant once, on a genuinely fresh
 * application state (docs/onboarding.md §5.2) - never inferred from
 * localStorage or the absence of configured destinations, only the
 * real persisted `status === 'pending'`. Renders nothing; it is a pure
 * side-effect component, mounted once alongside the route table.
 *
 * The `hasChecked` ref makes this a one-time check per page load: once
 * it has acted (redirected, or found status is not 'pending'), it never
 * fires again for the rest of this session, so navigating back to `/`
 * after visiting `/onboarding` never bounces the operator right back.
 */
export function OnboardingAutoRedirect() {
  const location = useLocation();
  const navigate = useNavigate();
  const stateQuery = useOnboardingStateQuery();
  const hasChecked = useRef(false);

  useEffect(() => {
    if (hasChecked.current) return;
    if (stateQuery.data === undefined) return;
    if (location.pathname.startsWith(PUBLIC_OVERLAY_PATH_PREFIX)) return;

    hasChecked.current = true;
    if (stateQuery.data.status === 'pending' && location.pathname !== '/onboarding') {
      void navigate('/onboarding');
    }
  }, [stateQuery.data, location.pathname, navigate]);

  return null;
}
