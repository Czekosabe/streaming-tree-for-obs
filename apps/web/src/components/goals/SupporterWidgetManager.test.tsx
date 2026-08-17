import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as donationSourcesApi from '@/api/donationsources';
import * as goalsApi from '@/api/goals';
import type { WidgetProfile } from '@/api/goals-schemas';
import { renderWithProviders } from '@/test/render';

import { SupporterWidgetManager } from './SupporterWidgetManager';

vi.mock('@/api/accounts');
vi.mock('@/api/donationsources');
vi.mock('@/api/goals');

function baseSupporterWidget(overrides: Partial<WidgetProfile> = {}): WidgetProfile {
  return {
    id: 'widget_1', kind: 'latest_follower', name: 'Latest Follower', enabled: true, publicSlug: 'a'.repeat(40),
    providers: [], accounts: [],
    showCurrent: false, showTarget: false, showPercent: false, showProvider: true, showTime: true, showMessage: false,
    maxItems: 0, eventTypes: [], columns: 0, children: [],
    orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
    backgroundColor: '#00000080', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
    borderRadiusPx: 12, opacity: 1.0,
    createdAt: '2026-08-17T00:00:00Z', updatedAt: '2026-08-17T00:00:00Z',
    ...overrides,
  };
}

function renderManager(dashboardsOnly: boolean) {
  return renderWithProviders(
    <MemoryRouter>
      <SupporterWidgetManager dashboardsOnly={dashboardsOnly} />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
  vi.mocked(donationSourcesApi).fetchDonationSources.mockResolvedValue([]);
  vi.mocked(goalsApi).fetchWidgetProfiles.mockResolvedValue([]);
});

describe('SupporterWidgetManager - widgets', () => {
  it('shows the empty state with no widgets', async () => {
    renderManager(false);
    expect(await screen.findByText(/no public widgets yet/i)).toBeInTheDocument();
  });

  it('creates a latest_follower widget with the chosen kind', async () => {
    const created = baseSupporterWidget();
    vi.mocked(goalsApi).createWidgetProfile.mockResolvedValue(created);
    renderManager(false);

    (await screen.findByRole('button', { name: /add widget/i })).click();
    const dialog = await screen.findByRole('dialog');
    const user = userEvent.setup();
    await user.type(within(dialog).getByLabelText(/^name$/i), 'Latest Follower');
    within(dialog).getByRole('button', { name: /^create$/i }).click();

    await waitFor(() =>
      expect(goalsApi.createWidgetProfile).toHaveBeenCalledWith(expect.objectContaining({ kind: 'latest_follower', name: 'Latest Follower' })),
    );
  });

  it('lists only non-dashboard, non-goal kinds', async () => {
    vi.mocked(goalsApi).fetchWidgetProfiles.mockResolvedValue([
      baseSupporterWidget({ id: 'w1', name: 'Latest Follower' }),
      baseSupporterWidget({ id: 'w2', name: 'My Dashboard', kind: 'dashboard', columns: 1, children: [{ widgetProfileId: 'w1', column: 1, columnSpan: 1, row: 1, rowSpan: 1 }] }),
    ]);
    renderManager(false);

    expect(await screen.findByText('Latest Follower')).toBeInTheDocument();
    expect(screen.queryByText('My Dashboard')).not.toBeInTheDocument();
  });

  it('shows a runtime-only note and a reset action once a widget is selected', async () => {
    vi.mocked(goalsApi).fetchWidgetProfiles.mockResolvedValue([baseSupporterWidget()]);
    vi.mocked(goalsApi).fetchWidgetRuntimeStatus.mockResolvedValue({ kind: 'latest_follower' });
    renderManager(false);

    (await screen.findByText('Latest Follower')).click();
    expect(await screen.findByText(/observed during the current application session/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^reset$/i })).toBeInTheDocument();
  });
});

describe('SupporterWidgetManager - dashboards', () => {
  it('shows the empty state with no dashboards', async () => {
    renderManager(true);
    expect(await screen.findByText(/no dashboards yet/i)).toBeInTheDocument();
  });

  it('lists only dashboard kinds and creates a new dashboard directly with kind=dashboard', async () => {
    vi.mocked(goalsApi).fetchWidgetProfiles.mockResolvedValue([
      baseSupporterWidget({ id: 'w1', name: 'Latest Follower' }),
    ]);
    const createdDashboard = baseSupporterWidget({ id: 'w2', name: 'My Dashboard', kind: 'dashboard', columns: 1 });
    vi.mocked(goalsApi).createWidgetProfile.mockResolvedValue(createdDashboard);
    renderManager(true);

    expect(screen.queryByText('Latest Follower')).not.toBeInTheDocument();

    (await screen.findByRole('button', { name: /add dashboard/i })).click();
    const dialog = await screen.findByRole('dialog');
    const user = userEvent.setup();
    await user.type(within(dialog).getByLabelText(/^name$/i), 'My Dashboard');
    within(dialog).getByRole('button', { name: /^create$/i }).click();

    await waitFor(() => expect(goalsApi.createWidgetProfile).toHaveBeenCalledWith(expect.objectContaining({ kind: 'dashboard', name: 'My Dashboard' })));
  });

  it('lets an operator add an existing widget as a dashboard child', async () => {
    const leaf = baseSupporterWidget({ id: 'w1', name: 'Latest Follower' });
    const dashboard = baseSupporterWidget({ id: 'w2', name: 'My Dashboard', kind: 'dashboard', columns: 1 });
    vi.mocked(goalsApi).fetchWidgetProfiles.mockResolvedValue([leaf, dashboard]);
    vi.mocked(goalsApi).updateWidgetProfile.mockResolvedValue({ ...dashboard, children: [{ widgetProfileId: 'w1', column: 1, columnSpan: 1, row: 1, rowSpan: 1 }] });
    renderManager(true);

    (await screen.findByText('My Dashboard')).click();
    const addChildSelect = await screen.findByLabelText(/add widget/i);
    const user = userEvent.setup();
    await user.selectOptions(addChildSelect, 'w1');

    const saveButton = await screen.findByRole('button', { name: /^save$/i });
    await waitFor(() => expect(saveButton).not.toBeDisabled());
    saveButton.click();

    await waitFor(() =>
      expect(goalsApi.updateWidgetProfile).toHaveBeenCalledWith(
        'w2',
        expect.objectContaining({ children: [expect.objectContaining({ widgetProfileId: 'w1' })] }),
      ),
    );
  });
});
