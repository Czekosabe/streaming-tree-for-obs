import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as onboardingApi from '@/api/onboarding';
import { renderWithProviders } from '@/test/render';

import { OnboardingPage } from './OnboardingPage';

vi.mock('@/api/onboarding');

function DashboardMarker() {
  return <div>dashboard-marker</div>;
}

function renderApp() {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/onboarding']}>
      <Routes>
        <Route path="/onboarding" element={<OnboardingPage />} />
        <Route path="/" element={<DashboardMarker />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(onboardingApi).setOnboardingStatus.mockResolvedValue({
    version: 1,
    status: 'completed',
    schemaVersion: 1,
  });
});

describe('OnboardingPage', () => {
  it('starts on the Welcome step, with Back disabled and Continue available', async () => {
    renderApp();

    expect(await screen.findByRole('heading', { name: /welcome to streaming tree for obs/i })).toBeInTheDocument();
    expect(screen.getByText(/step 1 of 2/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /back/i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /^continue$/i })).toBeInTheDocument();
  });

  it('never claims Streaming Tree is an OBS plugin, and explains the OBS -> destinations flow', async () => {
    renderApp();

    await screen.findByRole('heading', { name: /welcome to streaming tree for obs/i });
    expect(screen.getByText(/not an obs plugin/i)).toBeInTheDocument();
    expect(screen.getByText(/twitch, youtube, kick, tiktok/i)).toBeInTheDocument();
  });

  it('Continue moves to the Summary step and moves focus to its heading', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /welcome to streaming tree for obs/i });

    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));

    const summaryHeading = await screen.findByRole('heading', { name: /you're ready to go/i });
    expect(screen.getByText(/step 2 of 2/i)).toBeInTheDocument();
    await waitFor(() => expect(summaryHeading.closest('[tabindex="-1"]')).toHaveFocus());
  });

  it('Back returns from Summary to Welcome', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /welcome to streaming tree for obs/i });
    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));
    await screen.findByRole('heading', { name: /you're ready to go/i });

    await userEvent.click(screen.getByRole('button', { name: /back/i }));

    expect(await screen.findByRole('heading', { name: /welcome to streaming tree for obs/i })).toBeInTheDocument();
    expect(screen.getByText(/step 1 of 2/i)).toBeInTheDocument();
  });

  it('finishing on the last step marks onboarding completed and returns to the dashboard', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /welcome to streaming tree for obs/i });
    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));
    await screen.findByRole('heading', { name: /you're ready to go/i });

    await userEvent.click(screen.getByRole('button', { name: /go to dashboard/i }));

    await waitFor(() => expect(onboardingApi.setOnboardingStatus).toHaveBeenCalled());
    expect(vi.mocked(onboardingApi).setOnboardingStatus.mock.calls[0]?.[0]).toBe('completed');
    expect(await screen.findByText('dashboard-marker')).toBeInTheDocument();
  });

  it('Skip setup marks onboarding dismissed and returns to the dashboard immediately, from any step', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /welcome to streaming tree for obs/i });

    await userEvent.click(screen.getByRole('button', { name: /skip setup/i }));

    await waitFor(() => expect(onboardingApi.setOnboardingStatus).toHaveBeenCalled());
    expect(vi.mocked(onboardingApi).setOnboardingStatus.mock.calls[0]?.[0]).toBe('dismissed');
    expect(await screen.findByText('dashboard-marker')).toBeInTheDocument();
  });
});
