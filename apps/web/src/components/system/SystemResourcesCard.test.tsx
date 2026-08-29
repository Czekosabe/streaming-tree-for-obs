import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as systemResourcesApi from '@/api/system-resources';
import type { SystemResourcesSnapshot } from '@/models/system-resources';
import { renderWithProviders } from '@/test/render';

import { SystemResourcesCard } from './SystemResourcesCard';

vi.mock('@/api/system-resources');

const GIB = 1024 ** 3;

// Exact binary byte counts (`formatBytes` divides by 1024^n), so the
// rendered "used of total" string is a clean, predictable "4 GB of 16 GB"
// rather than an approximate value this test would need a fuzzy regex for.
function snapshot(overrides: Partial<SystemResourcesSnapshot> = {}): SystemResourcesSnapshot {
  return {
    cpuPercent: 12,
    memoryPercent: 48,
    memoryUsedBytes: 4 * GIB,
    memoryTotalBytes: 16 * GIB,
    diskPercent: 71,
    diskUsedBytes: 700 * GIB,
    diskTotalBytes: 1000 * GIB,
    unavailable: [],
    sampledAt: '2026-08-29T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SystemResourcesCard', () => {
  it('renders the real sampled CPU/memory/disk percentages - never a DEMO/placeholder value', async () => {
    vi.mocked(systemResourcesApi).fetchSystemResources.mockResolvedValue(snapshot());
    renderWithProviders(<SystemResourcesCard />);

    expect(await screen.findByText('12%')).toBeInTheDocument();
    expect(screen.getByText('48%')).toBeInTheDocument();
    expect(screen.getByText('71%')).toBeInTheDocument();
    expect(screen.queryByText(/demo/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/placeholder/i)).not.toBeInTheDocument();
  });

  it('shows an honest per-metric "unavailable" state instead of a fabricated fallback percentage', async () => {
    vi.mocked(systemResourcesApi).fetchSystemResources.mockResolvedValue(
      snapshot({ cpuPercent: null, unavailable: ['cpu'] }),
    );
    renderWithProviders(<SystemResourcesCard />);

    // Memory/disk still render real values...
    expect(await screen.findByText('48%')).toBeInTheDocument();
    expect(screen.getByText('71%')).toBeInTheDocument();
    // ...while CPU honestly reports unavailable, never "0%".
    expect(screen.getByText('Unavailable')).toBeInTheDocument();
    expect(screen.queryByText('0%')).not.toBeInTheDocument();
  });

  it('shows a real used-of-total detail line for memory and disk', async () => {
    vi.mocked(systemResourcesApi).fetchSystemResources.mockResolvedValue(snapshot());
    renderWithProviders(<SystemResourcesCard />);

    expect(await screen.findByText('4 GB of 16 GB')).toBeInTheDocument();
    expect(screen.getByText('700 GB of 1,000 GB')).toBeInTheDocument();
  });

  it('renders a real host-resource meter as an accessible ARIA meter, not a decorative-only bar', async () => {
    vi.mocked(systemResourcesApi).fetchSystemResources.mockResolvedValue(snapshot());
    renderWithProviders(<SystemResourcesCard />);

    await screen.findByText('12%');
    const meters = screen.getAllByRole('meter');
    expect(meters.length).toBe(3);
  });
});
