import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';

import { fetchSessionStatus, login as loginRequest, logout as logoutRequest } from '@/api/auth';
import { ApiError, setCSRFToken } from '@/lib/api-client';

/**
 * Stage 20D2B frontend auth state (docs/remote-management.md §27).
 *
 * `not-applicable` means this backend does not have remote management
 * enabled at all (GET /api/auth/session itself 404s) - every existing
 * desktop/D2A-headless-only deployment lands here and the management
 * UI renders exactly as it did before this stage, with zero gating.
 *
 * Nothing here is ever persisted to localStorage/sessionStorage/
 * IndexedDB/URL - the only real credential is the HttpOnly cookie the
 * browser itself holds; this context only mirrors what the backend
 * told it on the last bootstrap/login/logout round trip.
 */
type AuthStatus = 'checking' | 'not-applicable' | 'unauthenticated' | 'authenticated';

type LoginResult = { ok: true } | { ok: false; reason: 'invalid-credentials' | 'rate-limited' | 'network' };

type AuthContextValue = {
  status: AuthStatus;
  login: (password: string) => Promise<LoginResult>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('checking');

  useEffect(() => {
    let cancelled = false;
    void fetchSessionStatus()
      .then((result) => {
        if (cancelled) return;
        if (result === null) {
          setStatus('not-applicable');
          return;
        }
        if (result.authenticated) {
          setCSRFToken(result.csrfToken ?? null);
          setStatus('authenticated');
        } else {
          setCSRFToken(null);
          setStatus('unauthenticated');
        }
      })
      .catch(() => {
        // A bootstrap failure (network error, unexpected shape) is
        // treated the same as "not applicable" rather than trapping
        // the operator behind a login screen a broken backend can
        // never satisfy - the rest of the app's own existing error
        // states (e.g. a failed health check) already communicate a
        // genuinely unreachable backend.
        if (!cancelled) setStatus('not-applicable');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (password: string): Promise<LoginResult> => {
    try {
      const result = await loginRequest(password);
      setCSRFToken(result.csrfToken ?? null);
      setStatus('authenticated');
      return { ok: true };
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.status === 429) return { ok: false, reason: 'rate-limited' };
        if (error.status === 401) return { ok: false, reason: 'invalid-credentials' };
      }
      return { ok: false, reason: 'network' };
    }
  }, []);

  const logout = useCallback(async () => {
    try {
      await logoutRequest();
    } finally {
      setCSRFToken(null);
      setStatus('unauthenticated');
    }
  }, []);

  return <AuthContext.Provider value={{ status, login, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (ctx === null) {
    throw new Error('useAuth() must be used within an AuthProvider.');
  }
  return ctx;
}
