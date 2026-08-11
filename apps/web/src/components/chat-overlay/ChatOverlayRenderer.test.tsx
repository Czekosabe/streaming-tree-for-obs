import { screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ChatOverlayRemoveReason, PublicChatOverlayConfig, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';
import { renderWithProviders } from '@/test/render';
import type { ChatOverlayLeavingItem } from '@/models/chat-overlay-reducer';

import { ChatOverlayRenderer } from './ChatOverlayRenderer';

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

function baseConfig(overrides: Partial<PublicChatOverlayConfig> = {}): PublicChatOverlayConfig {
  return {
    schemaVersion: 1,
    layoutMode: 'horizontal',
    stackDirection: 'bottom_up',
    horizontalAlignment: 'left',
    showPlatformIcon: true,
    showPlatformName: false,
    showTimestamp: false,
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
    highlightSubscribers: true,
    highlightVips: true,
    language: 'en',
    renderingMode: 'legacy',
    ...overrides,
  };
}

function messageItem(id: string, sequence: number, overrides: Partial<PublicChatOverlayItem> = {}): PublicChatOverlayItem {
  return {
    version: 1,
    sequence,
    id,
    kind: 'message',
    providerId: 'twitch',
    occurredAt: '2026-08-06T12:00:00Z',
    user: { anonymous: false, displayName: `User ${id}` },
    message: { plainText: `hello from ${id}`, fragments: [{ type: 'text', text: `hello from ${id}` }] },
    deleted: false,
    synthetic: false,
    ...overrides,
  };
}

describe('ChatOverlayRenderer', () => {
  it('renders messages in first-seen order for stackDirection "bottom_up"', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ stackDirection: 'bottom_up' })}
        items={[messageItem('a', 1), messageItem('b', 2)]}
      />,
    );
    const items = screen.getAllByTestId('chat-overlay-item');
    expect(items[0]).toHaveTextContent('hello from a');
    expect(items[1]).toHaveTextContent('hello from b');
  });

  it('reverses render order for stackDirection "top_down" (newest enters at the top)', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ stackDirection: 'top_down' })}
        items={[messageItem('a', 1), messageItem('b', 2)]}
      />,
    );
    const items = screen.getAllByTestId('chat-overlay-item');
    expect(items[0]).toHaveTextContent('hello from b');
    expect(items[1]).toHaveTextContent('hello from a');
  });

  it('renders a deleted message as a placeholder, never the original text', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig()}
        items={[messageItem('a', 1, { deleted: true, message: undefined })]}
      />,
    );
    expect(screen.queryByText('hello from a')).not.toBeInTheDocument();
    expect(screen.getByText(/deleted/i)).toBeInTheDocument();
  });

  it('renders an anonymous user without a fabricated identity', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig()}
        items={[messageItem('a', 1, { user: { anonymous: true } })]}
      />,
    );
    expect(screen.getByText(/anonymous/i)).toBeInTheDocument();
    expect(screen.queryByText('User a')).not.toBeInTheDocument();
  });

  it('renders role tags for a highlighted broadcaster', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ highlightBroadcaster: true })}
        items={[messageItem('a', 1, { user: { anonymous: false, displayName: 'Streamer', isBroadcaster: true } })]}
      />,
    );
    expect(screen.getByText('Broadcaster')).toBeInTheDocument();
  });

  it('never falls back to rendering an emote image from an unrecognized host', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig()}
        items={[
          messageItem('a', 1, {
            message: {
              plainText: 'Kappa',
              fragments: [{ type: 'emote', text: 'Kappa', emoteImageUrl: 'https://evil.example.com/x.png' }],
            },
          }),
        ]}
      />,
    );
    expect(screen.getByText('Kappa')).toBeInTheDocument();
    expect(document.querySelector('img')).not.toBeInTheDocument();
  });

  it('never renders raw HTML from message text', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig()}
        items={[
          messageItem('a', 1, {
            message: {
              plainText: '<script>alert(1)</script>',
              fragments: [{ type: 'text', text: '<script>alert(1)</script>' }],
            },
          }),
        ]}
      />,
    );
    expect(screen.getByText('<script>alert(1)</script>')).toBeInTheDocument();
    expect(document.querySelector('script')).not.toBeInTheDocument();
  });

  it('renders an activity item using its activity type', () => {
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig()}
        items={[
          {
            version: 1,
            sequence: 1,
            id: 'act_1',
            kind: 'activity',
            providerId: 'twitch',
            occurredAt: '2026-08-06T12:00:00Z',
            user: { anonymous: false, displayName: 'Follower' },
            activity: { activityType: 'follow' },
            deleted: false,
            synthetic: false,
          },
        ]}
      />,
    );
    expect(screen.getByText(/followed/i)).toBeInTheDocument();
  });

  it('renders no items for an empty list without crashing', () => {
    renderWithProviders(<ChatOverlayRenderer config={baseConfig()} items={[]} />);
    expect(screen.queryAllByTestId('chat-overlay-item')).toHaveLength(0);
    expect(screen.getByTestId('chat-overlay-root')).toBeInTheDocument();
  });
});

function leavingEntry(id: string, reason: ChatOverlayRemoveReason): ChatOverlayLeavingItem {
  return { item: messageItem(id, 1, { message: { plainText: `leaving ${id}`, fragments: [{ type: 'text', text: `leaving ${id}` }] } }), reason };
}

describe('ChatOverlayRenderer - exit animation', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('renders a leaving item with the configured fade exit class for an expiry removal', () => {
    stubMatchMedia(false);
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ exitAnimation: 'fade' })}
        items={[]}
        leaving={[leavingEntry('a', 'expired')]}
        onLeavingComplete={vi.fn()}
      />,
    );
    const leavingNode = screen.getByTestId('chat-overlay-leaving-item');
    expect(leavingNode.className).toContain('animate-chat-overlay-fade-out');
    expect(leavingNode).toHaveTextContent('leaving a');
    expect(leavingNode.dataset.removeReason).toBe('expired');
  });

  it('renders a leaving item with the configured slide exit class for a capacity-eviction removal', () => {
    stubMatchMedia(false);
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ exitAnimation: 'slide_up' })}
        items={[]}
        leaving={[leavingEntry('a', 'capacity_evicted')]}
        onLeavingComplete={vi.fn()}
      />,
    );
    const leavingNode = screen.getByTestId('chat-overlay-leaving-item');
    expect(leavingNode.className).toContain('animate-chat-overlay-slide-up-out');
  });

  it('completes immediately (no rendered leaving node) when exitAnimation is "none"', () => {
    stubMatchMedia(false);
    const onLeavingComplete = vi.fn();
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ exitAnimation: 'none' })}
        items={[]}
        leaving={[leavingEntry('a', 'expired')]}
        onLeavingComplete={onLeavingComplete}
      />,
    );
    expect(screen.queryByTestId('chat-overlay-leaving-item')).not.toBeInTheDocument();
    expect(onLeavingComplete).toHaveBeenCalledWith('a');
  });

  it('completes immediately under prefers-reduced-motion, regardless of the configured exit animation', () => {
    stubMatchMedia(true);
    const onLeavingComplete = vi.fn();
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ exitAnimation: 'scale' })}
        items={[]}
        leaving={[leavingEntry('a', 'expired')]}
        onLeavingComplete={onLeavingComplete}
      />,
    );
    expect(screen.queryByTestId('chat-overlay-leaving-item')).not.toBeInTheDocument();
    expect(onLeavingComplete).toHaveBeenCalledWith('a');
  });

  it('falls back to a hard timeout when animationend never fires', () => {
    stubMatchMedia(false);
    const onLeavingComplete = vi.fn();
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ exitAnimation: 'fade', animationDurationMs: 250 })}
        items={[]}
        leaving={[leavingEntry('a', 'expired')]}
        onLeavingComplete={onLeavingComplete}
      />,
    );
    expect(onLeavingComplete).not.toHaveBeenCalled();
    vi.advanceTimersByTime(399);
    expect(onLeavingComplete).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(onLeavingComplete).toHaveBeenCalledWith('a');
  });

  it('completes via animationend before the fallback timeout, without a duplicate call racing it', () => {
    stubMatchMedia(false);
    const onLeavingComplete = vi.fn();
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ exitAnimation: 'fade', animationDurationMs: 250 })}
        items={[]}
        leaving={[leavingEntry('a', 'expired')]}
        onLeavingComplete={onLeavingComplete}
      />,
    );
    const leavingNode = screen.getByTestId('chat-overlay-leaving-item');
    leavingNode.dispatchEvent(new Event('animationend', { bubbles: true }));
    expect(onLeavingComplete).toHaveBeenCalledTimes(1);
    expect(onLeavingComplete).toHaveBeenCalledWith('a');
  });

  it('positions a leaving item as the oldest entry for stackDirection "bottom_up"', () => {
    stubMatchMedia(false);
    renderWithProviders(
      <ChatOverlayRenderer
        config={baseConfig({ stackDirection: 'bottom_up', exitAnimation: 'fade' })}
        items={[messageItem('active-1', 2)]}
        leaving={[leavingEntry('leaving-1', 'expired')]}
        onLeavingComplete={vi.fn()}
      />,
    );
    const children = screen.getByTestId('chat-overlay-root').children;
    expect(children[0]).toHaveAttribute('data-testid', 'chat-overlay-leaving-item');
    expect(children[1]).toHaveAttribute('data-testid', 'chat-overlay-item');
  });

  // --- Stage 13B: design-driven rendering --------------------------------

  function designDrivenConfig(): PublicChatOverlayConfig {
    return baseConfig({
      renderingMode: 'visual_design',
      visualDesign: {
        schemaVersion: 2,
        canvas: { width: 960, height: 280, transparent: true },
        layers: [
          {
            id: 'layer_username', kind: 'text',
            frame: { x: 10, y: 10, width: 400, height: 60 }, opacity: 1,
            text: {
              binding: 'username', missingValueBehavior: 'hide',
              fontFamily: 'system-ui', fontSize: 20, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
              textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'middle',
              outlineWidth: 0, outlineColor: '#000000',
              shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
            },
            entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
          },
        ],
      },
    });
  }

  it('renders a design-driven item through the shared VisualDesignRenderer, using the item\'s own username binding', () => {
    renderWithProviders(
      <ChatOverlayRenderer config={designDrivenConfig()} items={[messageItem('a', 1)]} />,
    );
    expect(screen.getByTestId('visual-design-renderer')).toBeInTheDocument();
    expect(screen.getByText('User a')).toBeInTheDocument();
    expect(screen.getByTestId('chat-overlay-item')).toHaveAttribute('data-rendering-mode', 'visual_design');
  });

  it('a design-driven overlay never renders the legacy bubble content for the same item', () => {
    renderWithProviders(
      <ChatOverlayRenderer config={designDrivenConfig()} items={[messageItem('a', 1)]} />,
    );
    expect(screen.queryByText('hello from a')).not.toBeInTheDocument();
  });

  it('falls back to legacy rendering when renderingMode is legacy, even with items present', () => {
    renderWithProviders(
      <ChatOverlayRenderer config={baseConfig()} items={[messageItem('a', 1)]} />,
    );
    expect(screen.queryByTestId('visual-design-renderer')).not.toBeInTheDocument();
    expect(screen.getByText('hello from a')).toBeInTheDocument();
    expect(screen.getByTestId('chat-overlay-item')).toHaveAttribute('data-rendering-mode', 'legacy');
  });
});
