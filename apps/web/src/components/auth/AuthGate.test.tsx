import { screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import { AuthProvider } from '@/app/auth-context';
import * as authApi from '@/api/auth';
import { renderWithProviders } from '@/test/render';

import { AuthGate } from './AuthGate';

vi.mock('@/api/auth');

function renderAt(path: string) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <AuthGate>
          <div>protected content</div>
        </AuthGate>
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe('AuthGate', () => {
  it('never gates a public overlay route, even while unauthenticated', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockResolvedValue({ authenticated: false });

    renderAt('/overlay/chat/some-public-slug');

    // The overlay route renders immediately, without waiting for the
    // auth bootstrap to resolve at all.
    expect(screen.getByText('protected content')).toBeInTheDocument();
  });

  it('renders children immediately when remote management is not applicable', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockResolvedValue(null);

    renderAt('/');

    await waitFor(() => expect(screen.getByText('protected content')).toBeInTheDocument());
  });

  it('renders the login page for a management route when unauthenticated', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockResolvedValue({ authenticated: false });

    renderAt('/');

    await waitFor(() => expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument());
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  it('renders children for a management route when authenticated', async () => {
    vi.mocked(authApi.fetchSessionStatus).mockResolvedValue({ authenticated: true, csrfToken: 'x' });

    renderAt('/');

    await waitFor(() => expect(screen.getByText('protected content')).toBeInTheDocument());
  });
});
