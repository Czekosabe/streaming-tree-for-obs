import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authApi from '@/api/auth';
import { setCSRFToken } from '@/lib/api-client';

import { AuthProvider, useAuth } from './auth-context';

vi.mock('@/api/auth');

function Probe() {
  const { status } = useAuth();
  return <div data-testid="status">{status}</div>;
}

beforeEach(() => {
  setCSRFToken(null);
});

describe('AuthProvider', () => {
  it('starts in checking state, then resolves to not-applicable when the backend has no remote-management route', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockResolvedValue(null);

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    expect(screen.getByTestId('status')).toHaveTextContent('checking');
    await waitFor(() => expect(screen.getByTestId('status')).toHaveTextContent('not-applicable'));
  });

  it('resolves to unauthenticated when the backend reports no active session', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockResolvedValue({ authenticated: false });

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated'));
  });

  it('resolves to authenticated and sets the CSRF token when the backend reports an active session', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockResolvedValue({ authenticated: true, csrfToken: 'bootstrap-csrf' });

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => expect(screen.getByTestId('status')).toHaveTextContent('authenticated'));
  });

  it('treats a bootstrap failure as not-applicable rather than trapping the user behind a login screen', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockRejectedValue(new Error('network down'));

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => expect(screen.getByTestId('status')).toHaveTextContent('not-applicable'));
  });
});
