import { screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as onboardingApi from '@/api/onboarding';
import type { OnboardingState } from '@/api/onboarding-schemas';
import { renderWithProviders } from '@/test/render';

import { OnboardingAutoRedirect } from './OnboardingAutoRedirect';

vi.mock('@/api/onboarding');

function stateOf(status: OnboardingState['status']): OnboardingState {
  return { version: 1, status, schemaVersion: 1 };
}

function renderAt(initialPath: string) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[initialPath]}>
      <OnboardingAutoRedirect />
      <Routes>
        <Route path="/" element={<div>dashboard-marker</div>} />
        <Route path="/onboarding" element={<div>onboarding-marker</div>} />
        <Route path="/overlay/chat/:publicSlug" element={<div>overlay-marker</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('OnboardingAutoRedirect', () => {
  it('redirects from the dashboard to the assistant when onboarding is pending', async () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue(stateOf('pending'));
    renderAt('/');

    expect(await screen.findByText('onboarding-marker')).toBeInTheDocument();
  });

  it('does not redirect once onboarding is completed', async () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue(stateOf('completed'));
    renderAt('/');

    await screen.findByText('dashboard-marker');
    expect(screen.queryByText('onboarding-marker')).not.toBeInTheDocument();
  });

  it('does not redirect once onboarding was explicitly dismissed', async () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue(stateOf('dismissed'));
    renderAt('/');

    await screen.findByText('dashboard-marker');
    expect(screen.queryByText('onboarding-marker')).not.toBeInTheDocument();
  });

  it('never redirects a public overlay route, regardless of status', async () => {
    vi.mocked(onboardingApi).fetchOnboardingState.mockResolvedValue(stateOf('pending'));
    renderAt('/overlay/chat/abc123');

    await screen.findByText('overlay-marker');
    expect(screen.queryByText('onboarding-marker')).not.toBeInTheDocument();
  });
});
