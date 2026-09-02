import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import * as streamInsightsApi from '@/api/stream-insights';
import * as streamSessionsApi from '@/api/stream-sessions';
import { i18n } from '@/i18n';
import { renderWithProviders } from '@/test/render';

import { HistoryPage } from './HistoryPage';

vi.mock('@/api/stream-sessions');
vi.mock('@/api/stream-insights');

const CLOSED_SESSION = {
  id: 'sess_closed',
  startedAt: '2026-08-01T12:00:00Z',
  endedAt: '2026-08-01T13:00:00Z',
  open: false,
  endReason: 'ingest_stopped',
  destinations: [
    {
      id: 'sessdest_1', platformId: 'pf_1', providerId: 'twitch', displayName: 'My Twitch',
      startedAt: '2026-08-01T12:00:05Z', endedAt: '2026-08-01T13:00:00Z', open: false, outcome: 'session_ended',
    },
  ],
};

const OPEN_SESSION = {
  id: 'sess_open',
  // A real recent timestamp (not a fixed past date): SessionDuration
  // computes the in-progress duration against the real current clock.
  startedAt: new Date(Date.now() - 5 * 60_000).toISOString(),
  endedAt: null,
  open: true,
  endReason: '',
  destinations: [
    {
      id: 'sessdest_2', platformId: 'pf_2', providerId: 'youtube', displayName: 'My YouTube',
      startedAt: '2026-08-02T09:00:05Z', endedAt: null, open: true, outcome: '',
    },
  ],
};

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <HistoryPage />
    </MemoryRouter>,
  );
}

describe('HistoryPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(streamSessionsApi).fetchStreamSessionSettings.mockResolvedValue({ retentionDays: 90 });
    vi.mocked(streamInsightsApi).fetchStreamInsights.mockResolvedValue({
      totalSessions: 0, totalDurationSeconds: 0, averageDurationSeconds: 0,
      longestSession: null, sessionsByEndReason: {}, destinations: [],
    });
  });

  afterEach(() => {
    void i18n.changeLanguage('en');
  });

  it('shows an honest empty state on a fresh install', async () => {
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText(/no stream sessions yet/i)).toBeInTheDocument();
  });

  it('lists a closed session with its destination outcome', async () => {
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([CLOSED_SESSION]);
    renderPage();

    const row = await screen.findByTestId('history-session-row');
    expect(within(row).getByText('My Twitch')).toBeInTheDocument();
    expect(within(row).getByText(/completed/i)).toBeInTheDocument();
    expect(within(row).getByText('1:00:00')).toBeInTheDocument();
  });

  it('pins an in-progress session with a live badge and no fixed end time', async () => {
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([OPEN_SESSION]);
    renderPage();

    const row = await screen.findByTestId('history-session-row');
    expect(within(row).getByText(/in progress/i)).toBeInTheDocument();
    expect(within(row).getByText('My YouTube')).toBeInTheDocument();
    expect(within(row).getByText(/live/i)).toBeInTheDocument();
  });

  it('never renders any chat/donation-shaped content within a session row - only destination names and coarse outcomes', async () => {
    // The page's own policy disclaimer legitimately contains the words
    // "chat"/"donation" (explaining what this feature does NOT record) -
    // this test scopes the check to the actual session rows themselves,
    // where real leaked content would appear.
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([CLOSED_SESSION, OPEN_SESSION]);
    renderPage();
    const rows = await screen.findAllByTestId('history-session-row');

    for (const row of rows) {
      expect(row.textContent).not.toMatch(/chat|donation|donor|subscriber message/i);
    }
  });

  it('changes the retention setting', async () => {
    const user = userEvent.setup();
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([]);
    vi.mocked(streamSessionsApi).setStreamSessionRetentionDays.mockResolvedValue({ retentionDays: 30 });
    renderPage();

    const select = await screen.findByLabelText<HTMLSelectElement>(/keep history for/i);
    await waitFor(() => expect(select).not.toBeDisabled());
    await user.selectOptions(select, '30');

    await waitFor(() => expect(streamSessionsApi.setStreamSessionRetentionDays).toHaveBeenCalledWith(30));
  });

  it('clears history only after explicit confirmation', async () => {
    const user = userEvent.setup();
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([CLOSED_SESSION]);
    vi.mocked(streamSessionsApi).clearStreamSessionHistory.mockResolvedValue(undefined);
    renderPage();

    await screen.findByTestId('history-session-row');
    await user.click(screen.getByRole('button', { name: /clear history/i }));

    expect(streamSessionsApi.clearStreamSessionHistory).not.toHaveBeenCalled();

    const dialog = await screen.findByRole('dialog', { name: /clear all stream session history/i });
    await user.click(within(dialog).getByRole('button', { name: /clear history/i }));

    await waitFor(() => expect(streamSessionsApi.clearStreamSessionHistory).toHaveBeenCalledTimes(1));
  });

  it('shows an empty insights state when no sessions have ever been recorded', async () => {
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText(/insights appear once your first session is recorded/i)).toBeInTheDocument();
  });

  it('shows aggregate stats and a per-destination breakdown', async () => {
    vi.mocked(streamSessionsApi).fetchStreamSessions.mockResolvedValue([CLOSED_SESSION]);
    vi.mocked(streamInsightsApi).fetchStreamInsights.mockResolvedValue({
      totalSessions: 3, totalDurationSeconds: 3 * 3600, averageDurationSeconds: 3600,
      longestSession: { sessionId: 'sess_1', startedAt: '2026-08-01T12:00:00Z', durationSeconds: 2 * 3600 },
      sessionsByEndReason: { ingest_stopped: 3 },
      destinations: [
        {
          platformId: 'pf_1', providerId: 'twitch', displayName: 'My Twitch',
          sessionCount: 3, durationSeconds: 3 * 3600, outcomeCounts: { completed: 2, error: 1 },
        },
      ],
    });
    renderPage();

    expect(await screen.findByText('Insights')).toBeInTheDocument();
    expect(await screen.findByText('3')).toBeInTheDocument();
    expect(screen.getAllByText('3h 00m').length).toBeGreaterThan(0);
    expect(screen.getByText('1h 00m')).toBeInTheDocument();
    expect(screen.getAllByText('My Twitch').length).toBeGreaterThan(0);
    expect(screen.getByText(/2 completed, 1 error/i)).toBeInTheDocument();
  });
});
