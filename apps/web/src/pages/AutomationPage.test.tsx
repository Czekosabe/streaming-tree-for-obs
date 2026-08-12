import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as chatAutomationApi from '@/api/chat-automation';
import type { Command, Schedule } from '@/api/chat-automation-schemas';
import * as platformsApi from '@/api/platforms';
import { renderWithProviders } from '@/test/render';

import { AutomationPage } from './AutomationPage';

vi.mock('@/api/accounts');
vi.mock('@/api/platforms');
vi.mock('@/api/chat-automation');

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <AutomationPage />
    </MemoryRouter>,
  );
}

const twitchAccount = {
  id: 'acct_1',
  providerId: 'twitch',
  login: 'streamer',
  displayName: 'Streamer',
  status: 'connected' as const,
  scopes: ['user:write:chat'],
  createdAt: '2026-08-06T00:00:00Z',
  updatedAt: '2026-08-06T00:00:00Z',
};

const youtubeAccount = {
  id: 'acct_2',
  providerId: 'youtube',
  login: 'My Channel',
  displayName: 'My Channel',
  status: 'connected' as const,
  scopes: ['https://www.googleapis.com/auth/youtube.force-ssl'],
  createdAt: '2026-08-06T00:00:00Z',
  updatedAt: '2026-08-06T00:00:00Z',
};

function baseSchedule(overrides: Partial<Schedule> = {}): Schedule {
  return {
    id: 'sched_1', name: 'Hourly reminder', enabled: true,
    intervalSeconds: 3600, firstDelaySeconds: 0, jitterSeconds: 0,
    onlyWhileIngestReceiving: false, minimumChatMessages: 0, maximumSendsPerHour: 10,
    targets: [{ accountId: 'acct_1' }],
    messages: [{ id: 'schedmsg_1', template: 'hello there' }],
    createdAt: '2026-08-06T00:00:00Z', updatedAt: '2026-08-06T00:00:00Z',
    state: 'scheduled',
    ...overrides,
  };
}

function baseCommand(overrides: Partial<Command> = {}): Command {
  return {
    id: 'cmd_1', name: 'discord', enabled: true, responseTemplate: 'join us',
    requiredRole: 'everyone', globalCooldownSeconds: 0, userCooldownSeconds: 0,
    aliases: [], targets: [{ accountId: 'acct_1' }],
    createdAt: '2026-08-06T00:00:00Z', updatedAt: '2026-08-06T00:00:00Z',
    matchCount: 0, responseCount: 0,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([twitchAccount]);
  vi.mocked(platformsApi).fetchPlatforms.mockResolvedValue([]);
  vi.mocked(chatAutomationApi).fetchSchedules.mockResolvedValue([]);
  vi.mocked(chatAutomationApi).fetchCommands.mockResolvedValue([]);
});

describe('AutomationPage', () => {
  it('shows the schedules tab by default with its own empty state', async () => {
    renderPage();
    expect(await screen.findByText(/no scheduled messages yet/i)).toBeInTheDocument();
  });

  it('switches to the commands tab and shows its own empty state', async () => {
    renderPage();
    await screen.findByText(/no scheduled messages yet/i);
    (await screen.findByRole('tab', { name: /chat commands/i })).click();
    expect(await screen.findByText(/no chat commands yet/i)).toBeInTheDocument();
  });

  it('lists an existing schedule with its state', async () => {
    vi.mocked(chatAutomationApi).fetchSchedules.mockResolvedValue([baseSchedule()]);
    renderPage();
    expect(await screen.findByText('Hourly reminder')).toBeInTheDocument();
    expect(screen.getByText('Scheduled')).toBeInTheDocument();
  });

  it('lists an existing command with its "!" prefix', async () => {
    vi.mocked(chatAutomationApi).fetchCommands.mockResolvedValue([baseCommand()]);
    renderPage();
    (await screen.findByRole('tab', { name: /chat commands/i })).click();
    expect(await screen.findByText('!discord')).toBeInTheDocument();
  });

  it('creates a new schedule with a name, a target account and a message', async () => {
    const created = baseSchedule();
    vi.mocked(chatAutomationApi).createSchedule.mockResolvedValue(created);
    renderPage();

    (await screen.findByRole('button', { name: /^create$/i })).click();
    const nameInput = await screen.findByLabelText(/^name$/i);
    const user = userEvent.setup();
    await user.type(nameInput, 'Hourly reminder');

    screen.getByRole('button', { name: /add target/i }).click();
    const accountSelect = await screen.findByLabelText(/^account$/i);
    await user.selectOptions(accountSelect, 'acct_1');

    const messageBoxes = screen.getAllByRole('textbox').filter((el) => el.tagName === 'TEXTAREA');
    await user.type(messageBoxes[0]!, 'hello there');

    const saveButton = screen.getByRole('button', { name: /^save$/i });
    await waitFor(() => expect(saveButton).not.toBeDisabled());
    saveButton.click();

    await waitFor(() =>
      expect(chatAutomationApi.createSchedule).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'Hourly reminder', targets: [{ accountId: 'acct_1' }], messages: ['hello there'] }),
      ),
    );
  });

  it('requires confirmation before deleting a schedule', async () => {
    vi.mocked(chatAutomationApi).fetchSchedules.mockResolvedValue([baseSchedule()]);
    vi.mocked(chatAutomationApi).deleteSchedule.mockResolvedValue(undefined);
    renderPage();

    (await screen.findByRole('button', { name: /^delete$/i })).click();
    expect(await screen.findByText(/delete this scheduled message\?/i)).toBeInTheDocument();
    expect(chatAutomationApi.deleteSchedule).not.toHaveBeenCalled();

    screen.getAllByRole('button', { name: /^delete$/i })[1]!.click();
    await waitFor(() => expect(chatAutomationApi.deleteSchedule).toHaveBeenCalledWith('sched_1'));
  });

  it('Send now shows a confirmation before calling the API, and reports a per-target result', async () => {
    vi.mocked(chatAutomationApi).fetchSchedules.mockResolvedValue([baseSchedule()]);
    vi.mocked(chatAutomationApi).sendScheduleNow.mockResolvedValue({
      results: [{ accountId: 'acct_1', sent: true, providerMessageId: 'm1' }],
    });
    renderPage();

    (await screen.findByRole('button', { name: /send now/i })).click();
    expect(await screen.findByText(/send "hourly reminder" now\?/i)).toBeInTheDocument();
    expect(chatAutomationApi.sendScheduleNow).not.toHaveBeenCalled();

    const dialog = screen.getByRole('dialog');
    within(dialog).getByRole('button', { name: /^send now$/i }).click();
    await waitFor(() => expect(chatAutomationApi.sendScheduleNow).toHaveBeenCalledWith('sched_1', []));
    expect(await screen.findByText(/sent as streamer/i)).toBeInTheDocument();
  });

  it('never shows a "no accounts" warning when a Twitch account is ready', async () => {
    renderPage();
    (await screen.findByRole('button', { name: /^create$/i })).click();
    await screen.findByLabelText(/^name$/i);
    expect(screen.queryByText(/no.*account is ready/i)).not.toBeInTheDocument();
  });

  it('offers a connected YouTube account as a target alongside Twitch (Stage 15A)', async () => {
    vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([twitchAccount, youtubeAccount]);
    renderPage();

    (await screen.findByRole('button', { name: /^create$/i })).click();
    await screen.findByLabelText(/^name$/i);
    screen.getByRole('button', { name: /add target/i }).click();

    const accountSelect = await screen.findByLabelText(/^account$/i);
    const optionLabels = within(accountSelect)
      .getAllByRole('option')
      .map((o) => o.textContent);
    expect(optionLabels).toEqual(expect.arrayContaining(['Streamer', 'My Channel']));
  });
});
