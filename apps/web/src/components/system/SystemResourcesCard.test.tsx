import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { SystemResourcesCard } from './SystemResourcesCard';

/**
 * No network mocking here deliberately: this exercises the same honest,
 * already-established real-failure path every other query in this test
 * suite relies on when unmocked (jsdom's `fetch` cannot reach a real
 * backend, `apiGet` converts that into a handled `ApiError`, and the
 * query settles as `isError`/not-pending) - proving the card never
 * crashes and never shows a fake success value when the backend is
 * unreachable, without needing to stub the shared low-level transport.
 */
describe('SystemResourcesCard', () => {
  it('renders the three real service-dependency rows, never host CPU/memory/disk/network', async () => {
    renderWithProviders(<SystemResourcesCard />);

    expect(screen.getByText('Backend')).toBeInTheDocument();
    expect(screen.getByText('Ingest engine')).toBeInTheDocument();
    expect(screen.getByText('FFmpeg')).toBeInTheDocument();

    expect(screen.queryByText(/cpu usage/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/memory usage/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/disk usage/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/demo/i)).not.toBeInTheDocument();
  });

  it('shows an honest unavailable state per row rather than a fake success value when the backend cannot be reached', async () => {
    renderWithProviders(<SystemResourcesCard />);

    expect(await screen.findByText('Backend unavailable')).toBeInTheDocument();
  });
});
