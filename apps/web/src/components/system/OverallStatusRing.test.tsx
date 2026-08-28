import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { OverallStatusRing } from './OverallStatusRing';

describe('OverallStatusRing', () => {
  it('shows the real live count at the centre and never fabricates a value', () => {
    renderWithProviders(<OverallStatusRing live={2} starting={1} error={0} offline={1} />);
    expect(screen.getByText('2')).toBeInTheDocument();
  });

  it('lists only the states that actually have a non-zero count', () => {
    renderWithProviders(<OverallStatusRing live={2} starting={1} error={0} offline={1} />);
    expect(screen.getByText(/2 live/i)).toBeInTheDocument();
    expect(screen.getByText(/1 starting/i)).toBeInTheDocument();
    expect(screen.queryByText(/error/i)).not.toBeInTheDocument();
    expect(screen.getByText(/1 offline/i)).toBeInTheDocument();
  });

  it('shows an honest idle message instead of an empty ring when nothing is configured', () => {
    renderWithProviders(<OverallStatusRing live={0} starting={0} error={0} offline={0} />);
    expect(screen.getByText(/no destination is currently sending/i)).toBeInTheDocument();
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('the ring graphic is decorative - the legend is the accessible source of truth', () => {
    const { container } = renderWithProviders(
      <OverallStatusRing live={3} starting={0} error={0} offline={0} />,
    );
    const ring = container.querySelector('[aria-hidden="true"]');
    expect(ring).not.toBeNull();
    // A screen reader must still get real counts from the legend list, not
    // only from the hidden ring's colour proportions.
    expect(screen.getByRole('list')).toBeInTheDocument();
  });
});
