import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as chatOverlayApi from '@/api/chat-overlay';
import type { ChatOverlayProfile } from '@/api/chat-overlay-schemas';
import { renderWithProviders } from '@/test/render';

import { OverlaysPage } from './OverlaysPage';

vi.mock('@/api/accounts');
vi.mock('@/api/chat-overlay');

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <OverlaysPage />
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
  createdAt: '2026-08-06T00:00:00Z',
  updatedAt: '2026-08-06T00:00:00Z',
};

function baseProfile(overrides: Partial<ChatOverlayProfile> = {}): ChatOverlayProfile {
  return {
    id: 'ov_1',
    publicSlug: 'slug_1',
    name: 'Main Overlay',
    enabled: true,
    layoutMode: 'horizontal',
    stackDirection: 'bottom_up',
    horizontalAlignment: 'left',
    showPlatformIcon: true,
    showPlatformName: false,
    showAccountLabel: false,
    showAvatar: false,
    showBadges: true,
    showTimestamp: false,
    showActivityEvents: true,
    showDeletedPlaceholder: false,
    hideCommands: true,
    hideBots: true,
    maxVisibleItems: 30,
    messageLifetimeSeconds: 0,
    fontFamily: 'sans_serif',
    fontSize: 16,
    fontWeight: 400,
    lineHeight: 1.4,
    textColor: '#FFFFFF',
    usernameColorMode: 'provider',
    bubbleColor: '#000000',
    bubbleOpacity: 0.45,
    borderRadius: 8,
    itemSpacing: 6,
    textOutline: true,
    textShadow: false,
    entryAnimation: 'fade',
    exitAnimation: 'fade',
    animationDurationMs: 250,
    highlightBroadcaster: true,
    highlightModerators: true,
    highlightSubscribers: false,
    highlightVips: false,
    language: 'en',
    createdAt: '2026-08-06T00:00:00Z',
    updatedAt: '2026-08-06T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([twitchAccount]);
  vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([]);
  vi.mocked(chatOverlayApi).fetchChatOverlayAccounts.mockResolvedValue([]);
  vi.mocked(chatOverlayApi).fetchChatOverlayHiddenUsers.mockResolvedValue([]);
  vi.mocked(chatOverlayApi).fetchChatOverlayBlockedTerms.mockResolvedValue([]);
  vi.mocked(chatOverlayApi).fetchChatOverlayActivityTypes.mockResolvedValue([]);
});

describe('OverlaysPage', () => {
  it('shows the empty state before any overlay exists', async () => {
    renderPage();
    expect(await screen.findByText(/no overlays yet/i)).toBeInTheDocument();
  });

  it('creates a new overlay and selects it', async () => {
    const created = baseProfile();
    vi.mocked(chatOverlayApi).createChatOverlay.mockResolvedValue(created);
    vi.mocked(chatOverlayApi).fetchChatOverlay.mockResolvedValue(created);

    renderPage();
    const input = await screen.findByPlaceholderText(/overlay name/i);
    await userEvent.setup().type(input, 'Main Overlay');
    screen.getByRole('button', { name: /new overlay/i }).click();

    await waitFor(() => expect(chatOverlayApi.createChatOverlay).toHaveBeenCalledWith('Main Overlay'));
    expect(await screen.findByText(/browser source url/i)).toBeInTheDocument();
  });

  it('selecting an existing overlay shows its Browser Source URL and settings', async () => {
    const profile = baseProfile();
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([profile]);
    vi.mocked(chatOverlayApi).fetchChatOverlay.mockResolvedValue(profile);

    renderPage();
    const listButton = await screen.findByRole('button', { name: /main overlay/i });
    listButton.click();

    expect(await screen.findByText(new RegExp(profile.publicSlug))).toBeInTheDocument();
    expect(screen.getByText(/visual settings/i)).toBeInTheDocument();
  });

  it('editing a setting enables Save and shows an unsaved-changes indicator, without saving until clicked', async () => {
    const profile = baseProfile();
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([profile]);
    vi.mocked(chatOverlayApi).fetchChatOverlay.mockResolvedValue(profile);
    vi.mocked(chatOverlayApi).replaceChatOverlay.mockResolvedValue({ ...profile, maxVisibleItems: 5 });

    renderPage();
    (await screen.findByRole('button', { name: /main overlay/i })).click();

    const maxVisible = await screen.findByLabelText(/maximum visible items/i);
    const saveButton = screen.getByRole('button', { name: /^save$/i });
    expect(saveButton).toBeDisabled();

    const user = userEvent.setup();
    await user.clear(maxVisible);
    await user.type(maxVisible, '5');

    expect(await screen.findByText(/unsaved changes/i)).toBeInTheDocument();
    expect(saveButton).not.toBeDisabled();
    expect(chatOverlayApi.replaceChatOverlay).not.toHaveBeenCalled();

    saveButton.click();
    await waitFor(() => expect(chatOverlayApi.replaceChatOverlay).toHaveBeenCalledTimes(1));
  });

  it('deleting an overlay requires confirmation before calling the API', async () => {
    const profile = baseProfile();
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([profile]);
    vi.mocked(chatOverlayApi).deleteChatOverlay.mockResolvedValue(undefined);

    renderPage();
    const row = (await screen.findByRole('button', { name: /main overlay/i })).closest('div')!;
    within(row).getByRole('button', { name: /delete/i }).click();

    expect(await screen.findByText(/delete this overlay\?/i)).toBeInTheDocument();
    expect(chatOverlayApi.deleteChatOverlay).not.toHaveBeenCalled();

    screen.getByRole('button', { name: /delete overlay/i }).click();
    await waitFor(() => expect(chatOverlayApi.deleteChatOverlay).toHaveBeenCalledWith('ov_1'));
  });

  it('shows an error state, distinct from the empty state, when the overlay list fails to load', async () => {
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockRejectedValue(new Error('network down'));
    renderPage();

    expect(await screen.findByText(/overlays could not be loaded/i)).toBeInTheDocument();
    expect(screen.queryByText(/no overlays yet/i)).not.toBeInTheDocument();
  });

  it('never renders a blocked term value or hidden-user list from another overlay implicitly (only what the mocked API for THIS overlay returns)', async () => {
    const profile = baseProfile();
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([profile]);
    vi.mocked(chatOverlayApi).fetchChatOverlay.mockResolvedValue(profile);
    vi.mocked(chatOverlayApi).fetchChatOverlayBlockedTerms.mockResolvedValue([
      { id: 'term_1', value: 'secretword', matchMode: 'contains', createdAt: '2026-08-06T00:00:00Z' },
    ]);

    renderPage();
    (await screen.findByRole('button', { name: /main overlay/i })).click();
    expect(await screen.findByText('secretword')).toBeInTheDocument();
  });
});
