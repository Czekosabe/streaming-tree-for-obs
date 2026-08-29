import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { UpcomingFeaturesCard } from './UpcomingFeaturesCard';

describe('UpcomingFeaturesCard', () => {
  it('lists the same real planned-feature copy the /platforms and /metadata placeholder routes already show', () => {
    renderWithProviders(<UpcomingFeaturesCard />);

    // Sourced from pages:platforms.planned.encoding / pages:metadata.planned.*
    // - never invented marketing copy, and never something already shipped.
    expect(
      screen.getByText(/per-branch encoding profile/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/reusable presets/i)).toBeInTheDocument();
    expect(screen.getByText(/history of previously used titles/i)).toBeInTheDocument();
  });

  it('never renders a rocket illustration or any other approximated asset', () => {
    const { container } = renderWithProviders(<UpcomingFeaturesCard />);
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('.lucide-rocket')).toBeNull();
  });
});
