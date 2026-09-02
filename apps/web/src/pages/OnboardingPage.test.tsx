import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as onboardingApi from '@/api/onboarding';
import { renderWithProviders } from '@/test/render';

import { OnboardingPage } from './OnboardingPage';

vi.mock('@/api/onboarding');

// The step framework (navigation/skip/finish) is tested here in
// isolation from each real step's own data fetching - each step gets
// its own focused test file (ReadinessStep.test.tsx and friends)
// mocking only its own specific API dependencies. Faking the step list
// keeps this file about navigation mechanics only, matching the same
// separation of concerns onboarding-steps.ts's own array shape invites.
vi.mock('@/components/onboarding/onboarding-steps', () => ({
  ONBOARDING_STEPS: [
    { id: 'welcome', Component: () => <div><h2>Fake welcome heading</h2></div> },
    { id: 'middle', Component: () => <div><h2>Fake middle heading</h2></div> },
    { id: 'summary', Component: () => <div><h2>Fake summary heading</h2></div> },
  ],
}));

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
  it('starts on the first step, with Back disabled and Continue available', async () => {
    renderApp();

    expect(await screen.findByRole('heading', { name: /fake welcome heading/i })).toBeInTheDocument();
    expect(screen.getByText(/step 1 of 3/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /back/i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /^continue$/i })).toBeInTheDocument();
  });

  it('Continue moves to the next step and moves focus to its heading', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /fake welcome heading/i });

    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));

    const middleHeading = await screen.findByRole('heading', { name: /fake middle heading/i });
    expect(screen.getByText(/step 2 of 3/i)).toBeInTheDocument();
    await waitFor(() => expect(middleHeading.closest('[tabindex="-1"]')).toHaveFocus());
  });

  it('Back returns to the previous step', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /fake welcome heading/i });
    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));
    await screen.findByRole('heading', { name: /fake middle heading/i });

    await userEvent.click(screen.getByRole('button', { name: /back/i }));

    expect(await screen.findByRole('heading', { name: /fake welcome heading/i })).toBeInTheDocument();
    expect(screen.getByText(/step 1 of 3/i)).toBeInTheDocument();
  });

  it('reaches the last step and finishing marks onboarding completed, returning to the dashboard', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /fake welcome heading/i });
    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));
    await screen.findByRole('heading', { name: /fake middle heading/i });
    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));
    await screen.findByRole('heading', { name: /fake summary heading/i });

    expect(screen.getByRole('button', { name: /go to dashboard/i })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /go to dashboard/i }));

    await waitFor(() => expect(onboardingApi.setOnboardingStatus).toHaveBeenCalled());
    expect(vi.mocked(onboardingApi).setOnboardingStatus.mock.calls[0]?.[0]).toBe('completed');
    expect(await screen.findByText('dashboard-marker')).toBeInTheDocument();
  });

  it('does not navigate away and shows a retryable error when persisting completion fails', async () => {
    vi.mocked(onboardingApi).setOnboardingStatus.mockRejectedValueOnce(new Error('network error'));
    renderApp();
    await screen.findByRole('heading', { name: /fake welcome heading/i });
    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));
    await screen.findByRole('heading', { name: /fake middle heading/i });
    await userEvent.click(screen.getByRole('button', { name: /^continue$/i }));
    await screen.findByRole('heading', { name: /fake summary heading/i });

    await userEvent.click(screen.getByRole('button', { name: /go to dashboard/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/something went wrong/i);
    // Still on the assistant - the Dashboard must never contradict a claim
    // of completion that was never actually persisted.
    expect(screen.queryByText('dashboard-marker')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /fake summary heading/i })).toBeInTheDocument();
  });

  it('Skip setup marks onboarding dismissed and returns to the dashboard immediately, from any step', async () => {
    renderApp();
    await screen.findByRole('heading', { name: /fake welcome heading/i });

    await userEvent.click(screen.getByRole('button', { name: /skip setup/i }));

    await waitFor(() => expect(onboardingApi.setOnboardingStatus).toHaveBeenCalled());
    expect(vi.mocked(onboardingApi).setOnboardingStatus.mock.calls[0]?.[0]).toBe('dismissed');
    expect(await screen.findByText('dashboard-marker')).toBeInTheDocument();
  });
});
