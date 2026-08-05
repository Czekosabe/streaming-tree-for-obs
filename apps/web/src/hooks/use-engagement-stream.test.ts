import { renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useEngagementStream } from './use-engagement-stream';

/**
 * Minimal fake EventSource: jsdom (this project's test environment) does
 * not implement the real one, so every rendered/hook test that touches
 * this hook needs a controllable stand-in. Deliberately reproduces only
 * the subset of the real API this hook actually uses (addEventListener,
 * close) - not a general-purpose polyfill.
 */
class FakeEventSource {
  static instances: FakeEventSource[] = [];
  listeners = new Map<string, Set<(event: MessageEvent<string>) => void>>();
  closed = false;

  constructor(public url: string) {
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)?.add(listener);
  }

  removeEventListener(type: string, listener: (event: MessageEvent<string>) => void) {
    this.listeners.get(type)?.delete(listener);
  }

  close() {
    this.closed = true;
  }

  emit(type: string, data: unknown) {
    const event = { data: JSON.stringify(data) } as MessageEvent<string>;
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }

  emitRaw(type: string) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({} as MessageEvent<string>);
    }
  }
}

function baseEvent(sequence: number) {
  return {
    schemaVersion: 1,
    sequence,
    id: `evt_${sequence}`,
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    type: 'follow',
    providerEventType: 'channel.follow',
    platformTimestamp: '2026-08-05T12:00:00Z',
    receivedAt: '2026-08-05T12:00:00Z',
    synthetic: false,
    user: { providerUserId: 'u1', anonymous: false },
  };
}

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal('EventSource', FakeEventSource);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('useEngagementStream', () => {
  it('starts in the connecting state and opens exactly one EventSource', () => {
    const { result } = renderHook(() => useEngagementStream());
    expect(result.current.status).toBe('connecting');
    expect(FakeEventSource.instances).toHaveLength(1);
  });

  it('transitions to open when the source fires its open event', async () => {
    const { result } = renderHook(() => useEngagementStream());
    const source = FakeEventSource.instances[0]!;
    source.emitRaw('open');
    await waitFor(() => expect(result.current.status).toBe('open'));
  });

  it('accepts a well-formed engagement.event and appends it', async () => {
    const { result } = renderHook(() => useEngagementStream());
    const source = FakeEventSource.instances[0]!;
    source.emit('engagement.event', baseEvent(1));
    await waitFor(() => expect(result.current.events).toHaveLength(1));
    expect(result.current.events[0]?.sequence).toBe(1);
  });

  it('ignores a duplicate or out-of-order sequence', async () => {
    const { result } = renderHook(() => useEngagementStream());
    const source = FakeEventSource.instances[0]!;
    source.emit('engagement.event', baseEvent(2));
    await waitFor(() => expect(result.current.events).toHaveLength(1));

    source.emit('engagement.event', baseEvent(2)); // duplicate
    source.emit('engagement.event', baseEvent(1)); // out of order
    await new Promise((resolve) => setTimeout(resolve, 10));

    expect(result.current.events).toHaveLength(1);
  });

  it('accepts strictly increasing sequences in order', async () => {
    const { result } = renderHook(() => useEngagementStream());
    const source = FakeEventSource.instances[0]!;
    source.emit('engagement.event', baseEvent(1));
    source.emit('engagement.event', baseEvent(2));
    source.emit('engagement.event', baseEvent(3));
    await waitFor(() => expect(result.current.events).toHaveLength(3));
    expect(result.current.events.map((e) => e.sequence)).toEqual([1, 2, 3]);
  });

  it('sets gapDetected on an engagement.gap event', async () => {
    const { result } = renderHook(() => useEngagementStream());
    const source = FakeEventSource.instances[0]!;
    expect(result.current.gapDetected).toBe(false);
    source.emitRaw('engagement.gap');
    await waitFor(() => expect(result.current.gapDetected).toBe(true));
  });

  it('ignores a malformed payload rather than crashing', async () => {
    const { result } = renderHook(() => useEngagementStream());
    const source = FakeEventSource.instances[0]!;
    source.emit('engagement.event', { not: 'a valid event' });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(result.current.events).toHaveLength(0);
  });

  it('closes the EventSource on unmount', () => {
    const { unmount } = renderHook(() => useEngagementStream());
    const source = FakeEventSource.instances[0]!;
    expect(source.closed).toBe(false);
    unmount();
    expect(source.closed).toBe(true);
  });

  it('does not open a connection when disabled', () => {
    renderHook(() => useEngagementStream(false));
    expect(FakeEventSource.instances).toHaveLength(0);
  });
});
