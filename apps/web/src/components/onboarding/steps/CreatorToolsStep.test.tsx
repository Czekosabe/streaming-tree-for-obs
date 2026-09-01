import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as chatOverlayApi from '@/api/chat-overlay';
import type { ChatOverlayProfile } from '@/api/chat-overlay-schemas';
import { renderWithProviders } from '@/test/render';

import { CreatorToolsStep } from './CreatorToolsStep';

vi.mock('@/api/chat-overlay');

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

function renderStep() {
  return renderWithProviders(
    <MemoryRouter>
      <CreatorToolsStep />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('CreatorToolsStep', () => {
  it('links only to real, shipped creator tools', async () => {
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([]);
    renderStep();

    expect(await screen.findByRole('link', { name: /chat overlay/i })).toHaveAttribute('href', '/chat');
    expect(screen.getByRole('link', { name: /alerts/i })).toHaveAttribute('href', '/alerts');
    expect(screen.getByRole('link', { name: /goals & widgets/i })).toHaveAttribute('href', '/goals');
    expect(screen.getByRole('link', { name: /audio & tts/i })).toHaveAttribute('href', '/audio');
  });

  it('directs to creating an overlay when none exists yet, without inventing a URL', async () => {
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([]);
    renderStep();

    expect(await screen.findByText(/don't have a chat overlay yet/i)).toBeInTheDocument();
    expect(screen.queryByText(/\/overlay\/chat\//)).not.toBeInTheDocument();
  });

  it('shows the real Browser Source URL once a chat overlay exists', async () => {
    vi.mocked(chatOverlayApi).fetchChatOverlays.mockResolvedValue([baseProfile()]);
    renderStep();

    expect(await screen.findByText(/\/overlay\/chat\/slug_1/)).toBeInTheDocument();
  });
});
