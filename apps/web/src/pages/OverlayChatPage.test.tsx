import { screen, waitFor } from '@testing-library/react';
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
  renderingMode: 'legacy' as const,
};

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
  FakeEventSource.instances = [];
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function messagePayload(id: string, text: string) {
  return {
    version: 1,
    sequence: 1,
    id,
    kind: 'message',
    providerId: 'twitch',
    occurredAt: '2026-08-06T12:00:00Z',
    user: { anonymous: false, displayName: 'Viewer' },
    message: { plainText: text, fragments: [{ type: 'text', text }] },
    deleted: false,
    synthetic: false,
  };
}

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

  it('removes a moderation-deleted message immediately, with no leaving node and no leftover text', async () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockResolvedValue({
      ...baseConfig,
      exitAnimation: 'fade',
    });
    renderPage();
    await screen.findByTestId('chat-overlay-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('chat-overlay.upsert', messagePayload('m1', 'a message to moderate'));
    await screen.findByText('a message to moderate');

    source.emit('chat-overlay.remove', { id: 'm1', reason: 'message_deleted' });

    await waitFor(() => expect(screen.queryByText('a message to moderate')).not.toBeInTheDocument());
    expect(screen.queryByTestId('chat-overlay-leaving-item')).not.toBeInTheDocument();
  });

  it('removes messages immediately on a chat-cleared reason, with no leftover text anywhere in the DOM', async () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockResolvedValue({
      ...baseConfig,
      exitAnimation: 'fade',
    });
    renderPage();
    await screen.findByTestId('chat-overlay-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('chat-overlay.upsert', messagePayload('m1', 'first message'));
    source.emit('chat-overlay.upsert', messagePayload('m2', 'second message'));
    await screen.findByText('second message');

    source.emit('chat-overlay.remove', { id: 'm1', reason: 'chat_cleared' });
    source.emit('chat-overlay.remove', { id: 'm2', reason: 'chat_cleared' });

    await waitFor(() => expect(screen.queryByText('second message')).not.toBeInTheDocument());
    expect(screen.queryByText('first message')).not.toBeInTheDocument();
    expect(screen.queryByTestId('chat-overlay-leaving-item')).not.toBeInTheDocument();
  });

  it('removes a message immediately on a user-messages-cleared reason', async () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockResolvedValue({
      ...baseConfig,
      exitAnimation: 'fade',
    });
    renderPage();
    await screen.findByTestId('chat-overlay-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('chat-overlay.upsert', messagePayload('m1', 'from a hidden user'));
    await screen.findByText('from a hidden user');

    source.emit('chat-overlay.remove', { id: 'm1', reason: 'user_messages_cleared' });

    await waitFor(() => expect(screen.queryByText('from a hidden user')).not.toBeInTheDocument());
    expect(screen.queryByTestId('chat-overlay-leaving-item')).not.toBeInTheDocument();
  });

  it('replaces the visible set on reset without animating any old item out', async () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockResolvedValue({
      ...baseConfig,
      exitAnimation: 'fade',
    });
    renderPage();
    await screen.findByTestId('chat-overlay-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('chat-overlay.upsert', messagePayload('m1', 'old message one'));
    source.emit('chat-overlay.upsert', messagePayload('m2', 'old message two'));
    await screen.findByText('old message two');

    source.emit('chat-overlay.reset', { items: [messagePayload('m3', 'fresh after reset')] });

    expect(await screen.findByText('fresh after reset')).toBeInTheDocument();
    expect(screen.queryByText('old message one')).not.toBeInTheDocument();
    expect(screen.queryByText('old message two')).not.toBeInTheDocument();
    expect(screen.queryByTestId('chat-overlay-leaving-item')).not.toBeInTheDocument();
  });

  it('shows a leaving node for a cosmetic (expired) removal when the overlay is configured with an exit animation', async () => {
    vi.mocked(chatOverlayApi).fetchPublicChatOverlayConfig.mockResolvedValue({
      ...baseConfig,
      exitAnimation: 'fade',
    });
    renderPage();
    await screen.findByTestId('chat-overlay-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('chat-overlay.upsert', messagePayload('m1', 'expiring naturally'));
    await screen.findByText('expiring naturally');

    source.emit('chat-overlay.remove', { id: 'm1', reason: 'expired' });

    expect(await screen.findByTestId('chat-overlay-leaving-item')).toHaveTextContent('expiring naturally');
  });
});
