import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as donationSourcesApi from '@/api/donationsources';
import * as goalsApi from '@/api/goals';
import type { Goal, WidgetProfile } from '@/api/goals-schemas';
import { renderWithProviders } from '@/test/render';

import { GoalsPage } from './GoalsPage';

vi.mock('@/api/accounts');
vi.mock('@/api/donationsources');
vi.mock('@/api/goals');

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <GoalsPage />
    </MemoryRouter>,
  );
}

function baseGoal(overrides: Partial<Goal> = {}): Goal {
  return {
    id: 'goal_1', name: 'Main Goal', kind: 'followers', enabled: true,
    target: 1000, current: 825, baseline: 825, providers: [], accounts: [],
    progressBasisPoints: 8250, completed: false,
    createdAt: '2026-08-16T00:00:00Z', updatedAt: '2026-08-16T00:00:00Z', startedAt: '2026-08-16T00:00:00Z',
    configRevision: 1,
    ...overrides,
  };
}

function baseWidgetProfile(overrides: Partial<WidgetProfile> = {}): WidgetProfile {
  return {
    id: 'widget_1', goalId: 'goal_1', name: 'Widget', enabled: true, publicSlug: 'a'.repeat(40),
    showCurrent: true, showTarget: true, showPercent: true,
    orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
    backgroundColor: '#00000080', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
    borderRadiusPx: 12, opacity: 1.0,
    createdAt: '2026-08-16T00:00:00Z', updatedAt: '2026-08-16T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([]);
  vi.mocked(donationSourcesApi).fetchDonationSources.mockResolvedValue([]);
  vi.mocked(goalsApi).fetchGoals.mockResolvedValue([]);
  vi.mocked(goalsApi).fetchWidgetProfiles.mockResolvedValue([]);
});

describe('GoalsPage', () => {
  it('shows the empty state with no goals', async () => {
    renderPage();
    expect(await screen.findByText(/no goals yet/i)).toBeInTheDocument();
  });

  it('creates a new goal and selects it', async () => {
    const created = baseGoal();
    vi.mocked(goalsApi).createGoal.mockResolvedValue(created);
    vi.mocked(goalsApi).fetchGoals.mockResolvedValueOnce([]).mockResolvedValue([created]);
    renderPage();

    (await screen.findByRole('button', { name: /^create$/i })).click();
    const dialog = await screen.findByRole('dialog');
    const nameInput = within(dialog).getByLabelText(/^name$/i);
    const user = userEvent.setup();
    await user.type(nameInput, 'Main Goal');

    const createButton = within(dialog).getByRole('button', { name: /^create$/i });
    await waitFor(() => expect(createButton).not.toBeDisabled());
    createButton.click();

    await waitFor(() =>
      expect(goalsApi.createGoal).toHaveBeenCalledWith(expect.objectContaining({ name: 'Main Goal', kind: 'followers' })),
    );
  });

  it('lists an existing goal and shows its observed progress', async () => {
    vi.mocked(goalsApi).fetchGoals.mockResolvedValue([baseGoal()]);
    renderPage();

    (await screen.findByText('Main Goal')).click();
    expect(await screen.findByText(/observed progress/i)).toBeInTheDocument();
    expect(screen.getByText('825 / 1,000')).toBeInTheDocument();
  });

  it('applies a manual set-current action', async () => {
    vi.mocked(goalsApi).fetchGoals.mockResolvedValue([baseGoal()]);
    vi.mocked(goalsApi).setGoalCurrent.mockResolvedValue(baseGoal({ current: 900 }));
    renderPage();

    (await screen.findByText('Main Goal')).click();
    const applyButton = await screen.findByRole('button', { name: /^apply$/i });
    const input = screen.getByPlaceholderText('0');
    const user = userEvent.setup();
    await user.type(input, '900');
    applyButton.click();

    await waitFor(() => expect(goalsApi.setGoalCurrent).toHaveBeenCalledWith('goal_1', 900));
  });

  it('resets a goal to its baseline', async () => {
    vi.mocked(goalsApi).fetchGoals.mockResolvedValue([baseGoal({ current: 900 })]);
    vi.mocked(goalsApi).resetGoal.mockResolvedValue(baseGoal());
    renderPage();

    (await screen.findByText('Main Goal')).click();
    (await screen.findByRole('button', { name: /reset to baseline/i })).click();

    await waitFor(() => expect(goalsApi.resetGoal).toHaveBeenCalledWith('goal_1'));
  });

  it('requires confirmation before deleting a goal', async () => {
    vi.mocked(goalsApi).fetchGoals.mockResolvedValue([baseGoal()]);
    vi.mocked(goalsApi).deleteGoal.mockResolvedValue(undefined);
    renderPage();

    (await screen.findByText('Main Goal')).click();
    const deleteButtons = await screen.findAllByRole('button', { name: /^delete$/i });
    deleteButtons[0]?.click();

    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/delete "Main Goal"/i)).toBeInTheDocument();
    within(dialog).getByRole('button', { name: /^delete$/i }).click();

    await waitFor(() => expect(goalsApi.deleteGoal).toHaveBeenCalledWith('goal_1'));
  });

  it('creates a widget profile and shows its public URL', async () => {
    vi.mocked(goalsApi).fetchGoals.mockResolvedValue([baseGoal()]);
    const createdWidget = baseWidgetProfile();
    vi.mocked(goalsApi).createWidgetProfile.mockResolvedValue(createdWidget);
    vi.mocked(goalsApi).fetchWidgetProfiles.mockResolvedValueOnce([]).mockResolvedValue([createdWidget]);
    renderPage();

    (await screen.findByText('Main Goal')).click();
    (await screen.findByRole('button', { name: /add widget/i })).click();

    const dialog = await screen.findByRole('dialog');
    const nameInput = within(dialog).getByLabelText(/^name$/i);
    const user = userEvent.setup();
    await user.type(nameInput, 'My Widget');
    within(dialog).getByRole('button', { name: /^create$/i }).click();

    await waitFor(() => expect(goalsApi.createWidgetProfile).toHaveBeenCalled());
    expect(await screen.findByText(new RegExp(createdWidget.publicSlug))).toBeInTheDocument();
  });
});
