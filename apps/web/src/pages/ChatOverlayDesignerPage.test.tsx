import { screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import type { ChatOverlayProfile } from '@/api/chat-overlay-schemas';
import * as chatOverlayApi from '@/api/chat-overlay';
import * as visualDesignApi from '@/api/visualdesign';
import { renderWithProviders } from '@/test/render';

import { ChatOverlayDesignerPage } from './ChatOverlayDesignerPage';

vi.mock('@/api/chat-overlay');
vi.mock('@/api/visualdesign');

const overlay: ChatOverlayProfile = {
  id: 'co_1', publicSlug: 'slug', name: 'Main Overlay', enabled: true,
  layoutMode: 'horizontal', stackDirection: 'bottom_up', horizontalAlignment: 'left',
  showPlatformIcon: true, showPlatformName: false, showAccountLabel: false, showAvatar: false,
  showBadges: true, showTimestamp: false, showActivityEvents: true, showDeletedPlaceholder: false,
  hideCommands: true, hideBots: true, maxVisibleItems: 30, messageLifetimeSeconds: 0,
  fontFamily: 'sans_serif', fontSize: 16, fontWeight: 400, lineHeight: 1.4, textColor: '#FFFFFF',
  usernameColorMode: 'provider', bubbleColor: '#000000', bubbleOpacity: 0.45, borderRadius: 8,
  itemSpacing: 6, textOutline: true, textShadow: false, entryAnimation: 'fade', exitAnimation: 'fade',
  animationDurationMs: 250, highlightBroadcaster: true, highlightModerators: true,
  highlightSubscribers: false, highlightVips: false, language: 'en',
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
};

function renderPage() {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/overlays/co_1/designer']}>
      <Routes>
        <Route path="/overlays/:overlayId/designer" element={<ChatOverlayDesignerPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('ChatOverlayDesignerPage', () => {
  it('shows a loading state before every dependency has resolved', () => {
    vi.mocked(chatOverlayApi).fetchChatOverlay.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(screen.getByTestId('chat-overlay-designer-loading')).toBeInTheDocument();
  });

  it('shows an error state when the overlay fails to load', async () => {
    vi.mocked(chatOverlayApi).fetchChatOverlay.mockRejectedValue(new Error('not found'));
    renderPage();
    expect(await screen.findByTestId('chat-overlay-designer-error')).toBeInTheDocument();
  });

  it('renders the workspace once the overlay and design have both loaded', async () => {
    vi.mocked(chatOverlayApi).fetchChatOverlay.mockResolvedValue(overlay);
    vi.mocked(visualDesignApi).fetchVisualDesign.mockResolvedValue({
      persisted: false, revision: 0,
      document: { version: 2, canvas: { width: 960, height: 280, transparent: true }, layers: [] },
    });
    renderPage();
    expect(await screen.findByTestId('chat-overlay-designer-workspace')).toBeInTheDocument();
  });
});
