import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ChatOverlayEditableFields } from '@/api/chat-overlay-schemas';
import { renderWithProviders } from '@/test/render';

import { OverlayPreviewPanel } from './OverlayPreviewPanel';

function stubMatchMedia(reducedMotion: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((query: string) => ({
      matches: reducedMotion,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
}

function draft(overrides: Partial<ChatOverlayEditableFields> = {}): ChatOverlayEditableFields {
  return {
    name: 'Preview overlay',
    enabled: true,
    layoutMode: 'horizontal',
    stackDirection: 'bottom_up',
    horizontalAlignment: 'left',
    showPlatformIcon: true,
    showPlatformName: false,
    showAccountLabel: false,
    showAvatar: true,
    showBadges: true,
    showTimestamp: false,
    showActivityEvents: true,
    showDeletedPlaceholder: true,
    hideCommands: false,
    hideBots: false,
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
    entryAnimation: 'none',
    exitAnimation: 'none',
    animationDurationMs: 250,
    highlightBroadcaster: true,
    highlightModerators: true,
    highlightSubscribers: false,
    highlightVips: false,
    language: 'en',
    ...overrides,
  };
}

describe('OverlayPreviewPanel', () => {
  beforeEach(() => {
    stubMatchMedia(false);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders the synthetic preview fixtures, never a real chat message', () => {
    renderWithProviders(<OverlayPreviewPanel draft={draft()} />);
    expect(screen.getByText('Hey, great stream today!')).toBeInTheDocument();
  });

  it('reflects the draft exit animation when simulating an expiry removal', async () => {
    const user = userEvent.setup();
    renderWithProviders(<OverlayPreviewPanel draft={draft({ exitAnimation: 'fade' })} />);

    await user.click(screen.getByRole('button', { name: /simulate expiry/i }));

    const leavingNode = await screen.findByTestId('chat-overlay-leaving-item');
    expect(leavingNode.className).toContain('animate-chat-overlay-fade-out');
  });

  it('never delays the simulated moderation removal - it applies on the same tick, with no leaving node', async () => {
    const user = userEvent.setup();
    renderWithProviders(<OverlayPreviewPanel draft={draft({ exitAnimation: 'fade' })} />);

    const removedText = screen.getByText('Keep it friendly, everyone.');
    expect(removedText).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /simulate moderation removal/i }));

    expect(screen.queryByText('Keep it friendly, everyone.')).not.toBeInTheDocument();
    expect(screen.queryByTestId('chat-overlay-leaving-item')).not.toBeInTheDocument();
  });

  it('reset preview restores the original fixture set', async () => {
    const user = userEvent.setup();
    renderWithProviders(<OverlayPreviewPanel draft={draft({ exitAnimation: 'none' })} />);

    await user.click(screen.getByRole('button', { name: /simulate moderation removal/i }));
    expect(screen.queryByText('Keep it friendly, everyone.')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /reset preview/i }));
    expect(screen.getByText('Keep it friendly, everyone.')).toBeInTheDocument();
  });
});
