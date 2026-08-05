import { screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { RecentEventsFeed } from './RecentEventsFeed';

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

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal('EventSource', FakeEventSource);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('RecentEventsFeed', () => {
  it('shows an empty state before any event arrives', async () => {
    renderWithProviders(<RecentEventsFeed />);
    expect(await screen.findByText(/no events received yet/i)).toBeInTheDocument();
  });

  it('renders a received follow event with its type label', async () => {
    renderWithProviders(<RecentEventsFeed />);
    const source = FakeEventSource.instances[0]!;

    source.emit('engagement.event', {
      schemaVersion: 1,
      sequence: 1,
      id: 'evt_1',
      providerId: 'twitch',
      connectedAccountId: 'acct_1',
      type: 'follow',
      providerEventType: 'channel.follow',
      platformTimestamp: '2026-08-05T12:00:00Z',
      receivedAt: '2026-08-05T12:00:00Z',
      synthetic: false,
      user: { displayName: 'Viewer', anonymous: false },
    });

    expect(await screen.findByText('Follow')).toBeInTheDocument();
    expect(screen.getByText('Viewer')).toBeInTheDocument();
  });

  it('renders an anonymous event without exposing a fabricated identity', async () => {
    renderWithProviders(<RecentEventsFeed />);
    const source = FakeEventSource.instances[0]!;

    source.emit('engagement.event', {
      schemaVersion: 1,
      sequence: 1,
      id: 'evt_1',
      providerId: 'twitch',
      connectedAccountId: 'acct_1',
      type: 'bits',
      providerEventType: 'channel.cheer',
      platformTimestamp: '2026-08-05T12:00:00Z',
      receivedAt: '2026-08-05T12:00:00Z',
      synthetic: false,
      quantity: 100,
      user: { anonymous: true },
    });

    expect(await screen.findByText(/anonymous/i)).toBeInTheDocument();
  });

  it('shows a gap notice after an engagement.gap event', async () => {
    renderWithProviders(<RecentEventsFeed />);
    const source = FakeEventSource.instances[0]!;
    source.emit('engagement.gap', { reason: 'sequence_evicted' });
    expect(await screen.findByText(/some events may have been missed/i)).toBeInTheDocument();
  });
});
