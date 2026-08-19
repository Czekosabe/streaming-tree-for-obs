import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { AuthProvider } from '@/app/auth-context';
import * as authApi from '@/api/auth';
import { renderWithProviders } from '@/test/render';

import { LoginPage } from './LoginPage';

vi.mock('@/api/auth');

function renderLoginPage() {
  return renderWithProviders(
    <AuthProvider>
      <LoginPage />
    </AuthProvider>,
  );
}

beforeEach(() => {
  vi.mocked(authApi.fetchSessionStatus).mockResolvedValue({ authenticated: false });
});

describe('LoginPage', () => {
  it('renders the password field and submit button', async () => {
    renderLoginPage();
    expect(await screen.findByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('does not render a username or registration field', async () => {
    renderLoginPage();
    await screen.findByLabelText(/password/i);
    expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/register|sign up/i)).not.toBeInTheDocument();
  });

  it('submits the password and shows an error on invalid credentials', async () => {
    const { ApiError } = await import('@/lib/api-client');
    vi.mocked(authApi.login).mockRejectedValue(new ApiError('http', 'unauthorized', { status: 401 }));

    renderLoginPage();
    const passwordField = await screen.findByLabelText(/password/i);
    await userEvent.type(passwordField, 'wrong-password');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/incorrect password/i);
    });
    // The password field is cleared after a failed attempt.
    expect(passwordField).toHaveValue('');
  });

  it('shows a rate-limited message on 429', async () => {
    const { ApiError } = await import('@/lib/api-client');
    vi.mocked(authApi.login).mockRejectedValue(new ApiError('http', 'rate limited', { status: 429 }));

    renderLoginPage();
    const passwordField = await screen.findByLabelText(/password/i);
    await userEvent.type(passwordField, 'anything');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/too many attempts/i);
    });
  });

  it('calls login with the entered password on submit', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ authenticated: true, csrfToken: 'a-csrf-token' });

    renderLoginPage();
    const passwordField = await screen.findByLabelText(/password/i);
    await userEvent.type(passwordField, 'correct-password');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(authApi.login).toHaveBeenCalledWith('correct-password');
    });
  });

  it('never stores the password in browser storage', async () => {
    vi.mocked(authApi.login).mockResolvedValue({ authenticated: true, csrfToken: 'a-csrf-token' });

    renderLoginPage();
    const passwordField = await screen.findByLabelText(/password/i);
    await userEvent.type(passwordField, 'super-secret-value');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => expect(authApi.login).toHaveBeenCalled());

    const storageDump = JSON.stringify(window.localStorage) + JSON.stringify(window.sessionStorage);
    expect(storageDump).not.toContain('super-secret-value');
  });
});
