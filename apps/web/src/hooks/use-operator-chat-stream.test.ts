import { renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useOperatorChatStream } from './use-operator-chat-stream';

/** Minimal fake EventSource - see use-engagement-stream.test.ts's own
 * identical fake for why jsdom needs one. */
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

function baseItem(id: string, sequence: number) {
  return {
    version: 1,
    sequence,
    id,
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    kind: 'message',
    occurredAt: '2026-08-06T12:00:00Z',
    receivedAt: '2026-08-06T12:00:00Z',
    lifecycle: { deleted: false },
    synthetic: false,
    user: { providerUserId: 'u1', anonymous: false },
    message: { plainText: 'hi', fragments: [{ type: 'text', text: 'hi' }] },
  };
}

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal('EventSource', FakeEventSource);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('useOperatorChatStream', () => {
  it('starts connecting and opens exactly one EventSource', () => {
    const { result } = renderHook(() => useOperatorChatStream());
    expect(result.current.status).toBe('connecting');
    expect(FakeEventSource.instances).toHaveLength(1);
  });

  it('transitions to open when the source fires its open event', async () => {
    const { result } = renderHook(() => useOperatorChatStream());
    const source = FakeEventSource.instances[0]!;
    source.emitRaw('open');
    await waitFor(() => expect(result.current.status).toBe('open'));
  });

  it('accepts a well-formed operator-chat.item and adds it', async () => {
    const { result } = renderHook(() => useOperatorChatStream());
    const source = FakeEventSource.instances[0]!;
    source.emit('operator-chat.item', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    expect(result.current.items[0]?.id).toBe('a');
  });

  it('a lifecycle update to the same id replaces it in place, not appended', async () => {
    const { result } = renderHook(() => useOperatorChatStream());
    const source = FakeEventSource.instances[0]!;
    source.emit('operator-chat.item', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    source.emit('operator-chat.item', { ...baseItem('a', 2), lifecycle: { deleted: true } });
    await waitFor(() => expect(result.current.items[0]?.lifecycle.deleted).toBe(true));
    expect(result.current.items).toHaveLength(1);
  });

  it('ignores a malformed payload rather than crashing', async () => {
    const { result } = renderHook(() => useOperatorChatStream());
    const source = FakeEventSource.instances[0]!;
    source.emit('operator-chat.item', { not: 'a valid item' });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(result.current.items).toHaveLength(0);
  });

  it('sets gapDetected on an operator-chat.gap event, and it stays set', async () => {
    const { result } = renderHook(() => useOperatorChatStream());
    const source = FakeEventSource.instances[0]!;
    expect(result.current.gapDetected).toBe(false);
    source.emitRaw('operator-chat.gap');
    await waitFor(() => expect(result.current.gapDetected).toBe(true));
  });

  it('closes the EventSource on unmount', () => {
    const { unmount } = renderHook(() => useOperatorChatStream());
    const source = FakeEventSource.instances[0]!;
    expect(source.closed).toBe(false);
    unmount();
    expect(source.closed).toBe(true);
  });

  it('does not open a connection when disabled', () => {
    renderHook(() => useOperatorChatStream(false));
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it('bounds retained items to the given capacity', async () => {
    const { result } = renderHook(() => useOperatorChatStream(true, 2));
    const source = FakeEventSource.instances[0]!;
    source.emit('operator-chat.item', baseItem('a', 1));
    source.emit('operator-chat.item', baseItem('b', 2));
    source.emit('operator-chat.item', baseItem('c', 3));
    await waitFor(() => expect(result.current.items.map((i) => i.id)).toEqual(['b', 'c']));
  });
});
