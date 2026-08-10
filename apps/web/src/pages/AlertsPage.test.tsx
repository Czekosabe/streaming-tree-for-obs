import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as alertsApi from '@/api/alerts';
import type { AlertProfile, AlertQueueStatus, AlertRule } from '@/api/alerts-schemas';
import { renderWithProviders } from '@/test/render';

import { AlertsPage } from './AlertsPage';

vi.mock('@/api/accounts');
vi.mock('@/api/alerts');

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <AlertsPage />
    </MemoryRouter>,
  );
}

const twitchAccount = {
  id: 'acct_1',
  providerId: 'twitch',
  login: 'streamer',
  displayName: 'Streamer',
  status: 'connected' as const,
  scopes: [],
  createdAt: '2026-08-10T00:00:00Z',
  updatedAt: '2026-08-10T00:00:00Z',
};

function baseProfile(overrides: Partial<AlertProfile> = {}): AlertProfile {
  return {
    id: 'alprof_1', publicSlug: 'slug1', name: 'Main', enabled: true,
    language: 'en', theme: 'minimal', position: 'bottom', textAlign: 'center',
    maxQueueItems: 100, maximumQueueAgeSeconds: 120,
    createdAt: '2026-08-10T00:00:00Z', updatedAt: '2026-08-10T00:00:00Z',
    ...overrides,
  };
}

function baseRule(overrides: Partial<AlertRule> = {}): AlertRule {
  return {
    id: 'alrule_1', profileId: 'alprof_1', name: 'Follow alert', enabled: true,
    eventType: 'follow', priority: 50, durationMs: 5000, minimumQuantity: null, maximumQuantity: null,
    requiredRole: 'everyone', showPlatform: true, showUsername: true, showMessage: false, showQuantity: false,
    textTemplate: '{username} just followed!', entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 400,
    providers: [], accounts: [],
    createdAt: '2026-08-10T00:00:00Z', updatedAt: '2026-08-10T00:00:00Z',
    ...overrides,
  };
}

function baseQueueStatus(overrides: Partial<AlertQueueStatus> = {}): AlertQueueStatus {
  return {
    profileId: 'alprof_1', enabled: true, paused: false, queuedCount: 0, queueCapacity: 100,
    nextQueued: [], totalEnqueued: 0, totalPlayed: 0, totalExpired: 0, totalCapacityDropped: 0,
    totalManuallySkipped: 0, totalSynthetic: 0, replayAvailable: false, activeSubscribers: 0, inputGap: false,
    ...overrides,
  };
}

const eventTypes = [
  {
    eventType: 'follow' as const, hasUser: true, hasMessage: false, hasQuantity: false,
    hasAnonymity: false, hasRewardTitle: false, hasRoles: false,
    availablePlaceholders: ['platform', 'eventType', 'username'],
  },
  {
    eventType: 'bits' as const, hasUser: true, hasMessage: true, hasQuantity: true,
    hasAnonymity: true, hasRewardTitle: false, hasRoles: false,
    availablePlaceholders: ['platform', 'eventType', 'username', 'quantity', 'message'],
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([twitchAccount]);
  vi.mocked(alertsApi).fetchAlertEventTypes.mockResolvedValue(eventTypes);
  vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([]);
});

describe('AlertsPage', () => {
  it('shows the empty state with no profiles', async () => {
    renderPage();
    expect(await screen.findByText(/no alert profiles yet/i)).toBeInTheDocument();
  });

  it('creates a new profile and selects it', async () => {
    const created = baseProfile();
    vi.mocked(alertsApi).createAlertProfile.mockResolvedValue(created);
    // The list refetches (invalidated) once the profile is created - the
    // first call still reflects the pre-create empty state.
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValueOnce([]).mockResolvedValue([created]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByRole('button', { name: /^create$/i })).click();
    const nameInput = await screen.findByLabelText(/^name$/i);
    const user = userEvent.setup();
    await user.type(nameInput, 'Main');

    const dialog = screen.getByRole('dialog');
    const createButton = within(dialog).getByRole('button', { name: /^create$/i });
    await waitFor(() => expect(createButton).not.toBeDisabled());
    createButton.click();

    await waitFor(() => expect(alertsApi.createAlertProfile).toHaveBeenCalledWith('Main'));
    expect(await screen.findByDisplayValue('Main')).toBeInTheDocument();
  });

  it('lists an existing profile and selecting it shows its Browser Source URL', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    expect(await screen.findByText(/slug1/)).toBeInTheDocument();
  });

  it('requires confirmation before deleting a profile', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    vi.mocked(alertsApi).deleteAlertProfile.mockResolvedValue(undefined);
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/slug1/);
    (await screen.findByRole('button', { name: /^delete$/i })).click();

    expect(await screen.findByText(/delete this alert profile\?/i)).toBeInTheDocument();
    expect(alertsApi.deleteAlertProfile).not.toHaveBeenCalled();

    const dialog = screen.getByRole('dialog');
    within(dialog).getByRole('button', { name: /^delete$/i }).click();
    await waitFor(() => expect(alertsApi.deleteAlertProfile).toHaveBeenCalledWith('alprof_1'));
  });

  it('shows queue status and pauses/resumes', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    vi.mocked(alertsApi).pauseAlertQueue.mockResolvedValue(baseQueueStatus({ paused: true }));
    renderPage();

    (await screen.findByText('Main')).click();
    (await screen.findByRole('button', { name: /^pause$/i })).click();
    await waitFor(() => expect(alertsApi.pauseAlertQueue).toHaveBeenCalledWith('alprof_1'));
  });

  it('lists an existing rule with its event type', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [baseRule()], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    expect(await screen.findByText('Follow alert')).toBeInTheDocument();
    expect(screen.getByText('Follow')).toBeInTheDocument();
  });

  it('creating a rule only shows capability-driven fields for the selected event type', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();

    // Default event type is "follow" (first in the mocked capability
    // list) - it has no quantity, so no quantity fields should render.
    await screen.findByLabelText(/^name$/i);
    expect(screen.queryByText(/minimum quantity/i)).not.toBeInTheDocument();

    const eventTypeSelect = screen.getByLabelText(/event type/i);
    const user = userEvent.setup();
    await user.selectOptions(eventTypeSelect, 'bits');

    expect(await screen.findByText(/minimum quantity/i)).toBeInTheDocument();
  });

  it('test rule sends a request and shows a synthetic notice', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [baseRule()], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    vi.mocked(alertsApi).testAlertRule.mockResolvedValue({
      alertId: 'alinst_1', ruleId: 'alrule_1', eventType: 'follow', queuedAt: '2026-08-10T00:00:00Z',
      priority: 50, renderedText: 'Ann just followed!', synthetic: true, replayed: false,
    });
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText('Follow alert');
    (await screen.findByRole('button', { name: /test rule/i })).click();

    await waitFor(() => expect(alertsApi.testAlertRule).toHaveBeenCalledWith('alrule_1', undefined));
    expect(await screen.findByText(/real, synthetic test alert/i)).toBeInTheDocument();
  });

  it('overlapping quantity ranges show a warning', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({
      rules: [
        baseRule({ id: 'alrule_1', name: 'Bits low', eventType: 'bits' }),
        baseRule({ id: 'alrule_2', name: 'Bits high', eventType: 'bits' }),
      ],
      overlapWarnings: [{ ruleId: 'alrule_1', otherRuleId: 'alrule_2', eventType: 'bits' }],
    });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText('Bits low');
    const warnings = await screen.findAllByText(/quantity range overlaps/i);
    // Each side of the overlapping pair shows its own warning.
    expect(warnings).toHaveLength(2);
  });
});
