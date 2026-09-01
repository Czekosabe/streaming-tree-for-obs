import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as onboardingApi from '@/api/onboarding';
import { renderWithProviders } from '@/test/render';

import { OnboardingDashboardBanner } from './OnboardingDashboardBanner';

vi.mock('@/api/onboarding');

function renderBanner() {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="/" element={<OnboardingDashboardBanner />} />
        <Route path="/onboarding" element={<div>onboarding-marker</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('OnboardingDashboardBanner', () => {
  it('renders nothing while onboarding status is still loading', () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockReturnValue(new Promise(() => {}));
    renderBanner();

    expect(screen.queryByText(/setup incomplete/i)).not.toBeInTheDocument();
  });

  it('renders nothing once onboarding is completed', async () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue({
      version: 1,
      status: 'completed',
      schemaVersion: 1,
    });
    renderBanner();

    await waitFor(() => expect(onboardingApi.fetchOnboardingState).toHaveBeenCalled());
    expect(screen.queryByText(/setup incomplete/i)).not.toBeInTheDocument();
  });

  it.each(['pending', 'dismissed'] as const)('shows the banner while status is %s', async (status) => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue({
      version: 1,
      status,
      schemaVersion: 1,
    });
    renderBanner();

    expect(await screen.findByText(/setup incomplete/i)).toBeInTheDocument();
  });

  it('navigates to the onboarding assistant on "Continue setup"', async () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue({
      version: 1,
      status: 'pending',
      schemaVersion: 1,
    });
    renderBanner();

    await userEvent.click(await screen.findByRole('button', { name: /continue setup/i }));

    expect(await screen.findByText('onboarding-marker')).toBeInTheDocument();
  });

  it('dismissing hides the banner for this session without changing the persisted status', async () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue({
      version: 1,
      status: 'pending',
      schemaVersion: 1,
    });
    renderBanner();

    await userEvent.click(await screen.findByRole('button', { name: /dismiss/i }));

    expect(screen.queryByText(/setup incomplete/i)).not.toBeInTheDocument();
    expect(onboardingApi.setOnboardingStatus).not.toHaveBeenCalled();
  });
});
