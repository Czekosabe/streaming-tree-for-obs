import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { PublicChatOverlayConfig, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';
import { renderWithProviders } from '@/test/render';

import { ChatOverlayRenderer } from './ChatOverlayRenderer';

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
