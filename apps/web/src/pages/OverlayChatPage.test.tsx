import { screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import * as chatOverlayApi from '@/api/chat-overlay';
import { renderWithProviders } from '@/test/render';

import { OverlayChatPage } from './OverlayChatPage';

vi.mock('@/api/chat-overlay');

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  listeners = new Map<string, Set<(event: MessageEvent<string>) => void>>();
  constructor(public url: string) {
    FakeEventSource.instances.push(this);
  }
  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)?.add(listener);
  }
  removeEventListener() {}
  close() {}
  emit(type: string, data: unknown) {
    const event = { data: JSON.stringify(data) } as MessageEvent<string>;
    for (const listener of this.listeners.get(type) ?? []) listener(event);
  }
}

function renderPage(slug = 'slug_1') {
  return renderWithProviders(
    <MemoryRouter initialEntries={[`/overlay/chat/${slug}`]}>
      <Routes>
        <Route path="/overlay/chat/:publicSlug" element={<OverlayChatPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

const baseConfig = {
  schemaVersion: 1,
  layoutMode: 'horizontal' as const,
  stackDirection: 'bottom_up' as const,
  horizontalAlignment: 'left' as const,
  showPlatformIcon: true,
  showPlatformName: false,
  showTimestamp: false,
  maxVisibleItems: 30,
  messageLifetimeSeconds: 0,
  fontFamily: 'sans_serif' as const,
  fontSize: 16,
  fontWeight: 400,
  lineHeight: 1.4,
  textColor: '#FFFFFF',
  usernameColorMode: 'provider' as const,
  bubbleColor: '#000000',
  bubbleOpacity: 0.45,
  borderRadius: 8,
  itemSpacing: 6,
  textOutline: true,
  textShadow: false,
  entryAnimation: 'none' as const,
  exitAnimation: 'none' as const,
  animationDurationMs: 250,
  highlightBroadcaster: true,
  highlightModerators: true,
  highlightSubscribers: false,
  highlightVips: false,
  language: 'en' as const,
};

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
  FakeEventSource.instances = [];
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('OverlayChatPage', () => {
  it('renders nothing but a bare div while the config has not loaded', () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(screen.getByTestId('chat-overlay-empty')).toBeInTheDocument();
  });

  it('never renders the application shell (no sidebar navigation landmark)', async () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('chat-overlay-root');
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('renders a live message once the config loads and the stream delivers one', async () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('chat-overlay-root');

    FakeEventSource.instances[0]!.emit('chat-overlay.upsert', {
      version: 1,
      sequence: 1,
      id: 'm1',
      kind: 'message',
      providerId: 'twitch',
      occurredAt: '2026-08-06T12:00:00Z',
      user: { anonymous: false, displayName: 'Viewer' },
      message: { plainText: 'hi from the stream', fragments: [{ type: 'text', text: 'hi from the stream' }] },
      deleted: false,
      synthetic: false,
    });

    expect(await screen.findByText('hi from the stream')).toBeInTheDocument();
  });
});
