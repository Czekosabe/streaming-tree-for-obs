import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as alertsApi from '@/api/alerts';
import type { AlertProfile, AlertQueueStatus, AlertRule } from '@/api/alerts-schemas';
import * as audioAssetApi from '@/api/audioasset';
import { renderWithProviders } from '@/test/render';

import { AlertsPage } from './AlertsPage';

vi.mock('@/api/accounts');
vi.mock('@/api/alerts');
vi.mock('@/api/audioasset');

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

const youtubeAccount = {
  id: 'acct_2',
  providerId: 'youtube',
  login: 'ytstreamer',
  displayName: 'YT Streamer',
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
    showAmount: false,
    allowGrouping: false, groupWindowMs: 5000, interruptMode: 'never', interruptible: true,
    audio: { soundEnabled: false, soundVolume: 1, ttsEnabled: false, ttsVolume: 1 },
    createdAt: '2026-08-10T00:00:00Z', updatedAt: '2026-08-10T00:00:00Z',
    ...overrides,
  };
}

function baseQueueStatus(overrides: Partial<AlertQueueStatus> = {}): AlertQueueStatus {
  return {
    profileId: 'alprof_1', enabled: true, paused: false, queuedCount: 0, queueCapacity: 100,
    nextQueued: [], totalEnqueued: 0, totalPlayed: 0, totalExpired: 0, totalCapacityDropped: 0,
    totalManuallySkipped: 0, totalSynthetic: 0, totalGroupedMembers: 0, totalGroupsCreated: 0, totalPreempted: 0,
    replayAvailable: false, activeSubscribers: 0, inputGap: false,
    ...overrides,
  };
}

const eventTypes = [
  {
    eventType: 'follow' as const, hasUser: true, hasMessage: false, hasQuantity: false,
    hasAnonymity: false, hasRewardTitle: false, hasRoles: false, hasAmount: false, hasMembershipLevel: false,
    availablePlaceholders: ['platform', 'eventType', 'username'],
    groupable: false, groupingRequiresHiddenMessage: false,
  },
  {
    eventType: 'bits' as const, hasUser: true, hasMessage: true, hasQuantity: true,
    hasAnonymity: true, hasRewardTitle: false, hasRoles: false, hasAmount: false, hasMembershipLevel: false,
    availablePlaceholders: ['platform', 'eventType', 'username', 'quantity', 'message', 'groupCount'],
    groupable: true, groupingRequiresHiddenMessage: true,
  },
  {
    eventType: 'youtube_super_chat' as const, hasUser: true, hasMessage: true, hasQuantity: false,
    hasAnonymity: false, hasRewardTitle: false, hasRoles: false, hasAmount: true, hasMembershipLevel: false,
    availablePlaceholders: ['platform', 'eventType', 'username', 'message', 'amount', 'currency', 'groupCount'],
    groupable: false, groupingRequiresHiddenMessage: false,
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([twitchAccount]);
  vi.mocked(alertsApi).fetchAlertEventTypes.mockResolvedValue(eventTypes);
  vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([]);
  vi.mocked(audioAssetApi).fetchAudioAssets.mockResolvedValue([]);
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

  it('the audio section reveals sound/TTS controls only once each is enabled, and TTS never offers the groupCount placeholder (Stage 17B)', async () => {
    // This test drives an unusually long sequence of real userEvent
    // keystrokes/clicks across two toggles and two text fields, so it
    // is the one most exposed to worker CPU contention under the full
    // suite's ~1370 tests running in parallel; it consistently passes
    // in isolation. An explicit longer timeout (default 5000ms) avoids
    // spurious failure under that legitimate contention without
    // masking a genuine hang, which would still exceed even this.
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();

    const dialog = await screen.findByRole('dialog', { name: /create alert rule/i });
    await within(dialog).findByLabelText(/^name$/i);
    expect(within(dialog).queryByRole('button', { name: /choose sound/i })).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText(/spoken text/i)).not.toBeInTheDocument();

    const user = userEvent.setup();
    // formValid also requires a name and the visual text template - fill
    // both so the TTS-specific assertions below are the only thing
    // gating Save.
    await user.type(within(dialog).getByLabelText(/^name$/i), 'Bits alert');
    await user.selectOptions(within(dialog).getByLabelText(/event type/i), 'bits');
    await user.type(within(dialog).getByLabelText(/alert text/i), '{{username}} cheered!');
    await user.click(within(dialog).getByRole('switch', { name: /play a sound/i }));
    expect(await within(dialog).findByRole('button', { name: /choose sound/i })).toBeInTheDocument();
    expect(within(dialog).getByText(/no sound selected/i)).toBeInTheDocument();
    // Turn sound back off - a soundEnabled=true rule with no asset chosen
    // is itself invalid and would otherwise block Save below for a
    // reason unrelated to what this test is checking.
    await user.click(within(dialog).getByRole('switch', { name: /play a sound/i }));

    await user.click(within(dialog).getByRole('switch', { name: /speak this alert aloud/i }));
    const ttsField = await within(dialog).findByLabelText(/spoken text/i);
    // bits' own availablePlaceholders includes groupCount (used by the
    // visual text-template's own placeholder row), but the TTS row must
    // never offer it - grouping never restarts already-playing audio.
    const ttsSection = ttsField.closest('div')!.parentElement!;
    expect(within(ttsSection).queryByRole('button', { name: '{groupCount}' })).not.toBeInTheDocument();
    expect(within(ttsSection).getByRole('button', { name: '{username}' })).toBeInTheDocument();

    // Enabling TTS with an empty template must block Save.
    const saveButton = within(dialog).getByRole('button', { name: /^save$/i });
    expect(saveButton).toBeDisabled();
    // userEvent.type reserves single braces for special key sequences -
    // "{{" / "}}" is how it escapes a literal brace.
    await user.type(ttsField, '{{username}} cheered!');
    await waitFor(() => expect(saveButton).not.toBeDisabled());
  }, 15000);

  it('choosing a sound from the picker selects it, and the create request carries the full audio object (Stage 17B)', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    vi.mocked(audioAssetApi).fetchAudioAssets.mockResolvedValue([
      {
        id: 'audioasset_1', kind: 'sound', mediaType: 'audio/wav', sizeBytes: 1000, durationMs: 2500,
        displayName: 'Coin chime', source: 'upload', referenceCount: 0,
        createdAt: '2026-08-10T00:00:00Z', updatedAt: '2026-08-10T00:00:00Z',
      },
    ]);
    vi.mocked(alertsApi).createAlertRule.mockResolvedValue(baseRule({ id: 'alrule_new' }));
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();
    const dialog = await screen.findByRole('dialog', { name: /create alert rule/i });
    await within(dialog).findByLabelText(/^name$/i);

    const user = userEvent.setup();
    await user.type(within(dialog).getByLabelText(/^name$/i), 'Coin alert');
    await user.type(within(dialog).getByLabelText(/alert text/i), '{{username}} triggered a coin!');
    await user.click(within(dialog).getByRole('switch', { name: /play a sound/i }));
    await user.click(await within(dialog).findByRole('button', { name: /choose sound/i }));

    const item = await screen.findByTestId('audio-asset-picker-item-audioasset_1');
    await user.click(item);

    expect(await within(dialog).findByText('Coin chime')).toBeInTheDocument();
    expect(screen.queryByRole('dialog', { name: /choose a sound/i })).not.toBeInTheDocument();

    await user.click(within(dialog).getByRole('button', { name: /^save$/i }));

    await waitFor(() => expect(alertsApi.createAlertRule).toHaveBeenCalled());
    const [, input] = vi.mocked(alertsApi.createAlertRule).mock.calls[0]!;
    expect(input.audio).toEqual({
      soundEnabled: true, soundAssetId: 'audioasset_1', soundVolume: 1,
      ttsEnabled: false, ttsTemplate: '', ttsVolume: 1,
    });
  });

  it('selecting a YouTube money event type shows the currency/amount fields and hides them again for follow (Stage 15A)', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();

    await screen.findByLabelText(/^name$/i);
    expect(screen.queryByLabelText(/^currency$/i)).not.toBeInTheDocument();

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText(/event type/i), 'youtube_super_chat');

    expect(await screen.findByLabelText(/^currency$/i)).toBeInTheDocument();
    expect(screen.getByText(/minimum amount/i)).toBeInTheDocument();
    expect(screen.getByText(/maximum amount/i)).toBeInTheDocument();
    expect(screen.getByText(/^show amount$/i)).toBeInTheDocument();
    // The amount placeholder buttons come straight from the mocked
    // capability's own availablePlaceholders - never hand-maintained.
    expect(screen.getByRole('button', { name: '{amount}' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '{currency}' })).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText(/event type/i), 'follow');
    expect(screen.queryByLabelText(/^currency$/i)).not.toBeInTheDocument();
  });

  it('the provider filter offers both Twitch and YouTube, and the account picker lists both providers (Stage 15A)', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([twitchAccount, youtubeAccount]);
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();

    await screen.findByLabelText(/^name$/i);
    expect(screen.getByText('Twitch')).toBeInTheDocument();
    expect(screen.getByText('YouTube')).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: /add account/i }));
    const accountSelect = screen.getByLabelText(/account filter/i);
    const optionLabels = within(accountSelect)
      .getAllByRole('option')
      .map((o) => o.textContent);
    expect(optionLabels).toEqual(expect.arrayContaining(['Streamer', 'YT Streamer']));
  });

  it('grouping control is unavailable for follow, available for bits, and reveals the group window once enabled (Stage 12B)', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();

    // Default event type is "follow" - not groupable - so the grouping
    // toggle is absent and the explanatory "unavailable" hint shows
    // instead.
    await screen.findByLabelText(/^name$/i);
    expect(screen.queryByText(/group similar queued alerts/i)).not.toBeInTheDocument();
    expect(await screen.findByText(/no safe way to group this event type/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/group window/i)).not.toBeInTheDocument();

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText(/event type/i), 'bits');

    const groupingToggle = await screen.findByText(/group similar queued alerts/i);
    expect(groupingToggle).toBeInTheDocument();
    expect(screen.queryByLabelText(/group window/i)).not.toBeInTheDocument();

    await user.click(groupingToggle);
    expect(await screen.findByLabelText(/group window/i)).toBeInTheDocument();
  });

  it('interruption controls (interrupt lower-priority, allow interruption) are always shown regardless of event type', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();

    await screen.findByLabelText(/^name$/i);
    expect(await screen.findByText(/interrupt lower-priority alert/i)).toBeInTheDocument();
    expect(await screen.findByText(/allow this alert to be interrupted/i)).toBeInTheDocument();
  });

  it('grouping is force-disabled by default and enabling it on a message-bearing type turns off "show message"', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    renderPage();

    (await screen.findByText('Main')).click();
    await screen.findByText(/no alert rules yet/i);
    const rulesPanel = (await screen.findByRole('heading', { name: 'Rules' })).closest('section')!;
    within(rulesPanel).getByRole('button', { name: /^create$/i }).click();

    await screen.findByLabelText(/^name$/i);
    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText(/event type/i), 'bits');

    const showMessageToggle = await screen.findByLabelText(/show message/i);
    await user.click(showMessageToggle);
    expect(showMessageToggle).toBeChecked();

    await user.click(await screen.findByText(/group similar queued alerts/i));
    expect(showMessageToggle).not.toBeChecked();
    expect(await screen.findByText(/has been turned off for you/i)).toBeInTheDocument();
  });

  it('test rule sends a request and shows a synthetic notice', async () => {
    vi.mocked(alertsApi).fetchAlertProfiles.mockResolvedValue([baseProfile()]);
    vi.mocked(alertsApi).fetchAlertRules.mockResolvedValue({ rules: [baseRule()], overlapWarnings: [] });
    vi.mocked(alertsApi).fetchAlertQueueStatus.mockResolvedValue(baseQueueStatus());
    vi.mocked(alertsApi).testAlertRule.mockResolvedValue({
      alertId: 'alinst_1', ruleId: 'alrule_1', eventType: 'follow', queuedAt: '2026-08-10T00:00:00Z',
      priority: 50, renderedText: 'Ann just followed!', synthetic: true, replayed: false,
      groupCount: 1, interruptible: true,
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
