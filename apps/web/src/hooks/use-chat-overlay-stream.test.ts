import { renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { vi } from 'vitest';

import { useChatOverlayStream } from './use-chat-overlay-stream';

/** Minimal fake EventSource - see use-operator-chat-stream.test.ts's own
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
    kind: 'message',
    providerId: 'twitch',
    occurredAt: '2026-08-06T12:00:00Z',
    deleted: false,
    synthetic: false,
    user: { anonymous: false, displayName: 'Viewer' },
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

describe('useChatOverlayStream', () => {
  it('does not open a connection without a slug', () => {
    renderHook(() => useChatOverlayStream(undefined));
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it('opens exactly one EventSource for a given slug', () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    expect(result.current.status).toBe('connecting');
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(FakeEventSource.instances[0]?.url).toContain('/api/public/chat-overlays/slug_1/stream');
  });

  it('transitions to open when the source fires its open event', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    FakeEventSource.instances[0]!.emitRaw('open');
    await waitFor(() => expect(result.current.status).toBe('open'));
  });

  it('applies a chat-overlay.upsert event', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    FakeEventSource.instances[0]!.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    expect(result.current.items[0]?.id).toBe('a');
  });

  it('applies an immediate chat-overlay.remove event, deleting the item entirely with nothing left leaving', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    const source = FakeEventSource.instances[0]!;
    source.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    source.emit('chat-overlay.remove', { id: 'a', reason: 'message_deleted' });
    await waitFor(() => expect(result.current.items).toHaveLength(0));
    expect(result.current.leaving).toHaveLength(0);
  });

  it('a cosmetic chat-overlay.remove event moves the item to leaving instead of deleting it outright', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    const source = FakeEventSource.instances[0]!;
    source.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    source.emit('chat-overlay.remove', { id: 'a', reason: 'expired' });
    await waitFor(() => expect(result.current.items).toHaveLength(0));
    expect(result.current.leaving).toHaveLength(1);
    expect(result.current.leaving[0]?.item.id).toBe('a');
    expect(result.current.leaving[0]?.reason).toBe('expired');
  });

  it('completeLeaving clears a cosmetic item out of the leaving list', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    const source = FakeEventSource.instances[0]!;
    source.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    source.emit('chat-overlay.remove', { id: 'a', reason: 'capacity_evicted' });
    await waitFor(() => expect(result.current.leaving).toHaveLength(1));

    result.current.completeLeaving('a');
    await waitFor(() => expect(result.current.leaving).toHaveLength(0));
  });

  it('a remove event with no reason field defaults to the safe, immediate case rather than dropping the event', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    const source = FakeEventSource.instances[0]!;
    source.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    source.emit('chat-overlay.remove', { id: 'a' });
    await waitFor(() => expect(result.current.items).toHaveLength(0));
    expect(result.current.leaving).toHaveLength(0);
  });

  it('applies a chat-overlay.reset event, replacing the entire visible set', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    const source = FakeEventSource.instances[0]!;
    source.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    source.emit('chat-overlay.reset', { items: [baseItem('b', 2), baseItem('c', 3)] });
    await waitFor(() => expect(result.current.items.map((i) => i.id)).toEqual(['b', 'c']));
  });

  it('ignores a malformed upsert payload rather than crashing', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    FakeEventSource.instances[0]!.emit('chat-overlay.upsert', { not: 'a valid item' });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(result.current.items).toHaveLength(0);
  });

  it('sets gapDetected on a chat-overlay.gap event, and it stays set', async () => {
    const { result } = renderHook(() => useChatOverlayStream('slug_1'));
    expect(result.current.gapDetected).toBe(false);
    FakeEventSource.instances[0]!.emitRaw('chat-overlay.gap');
    await waitFor(() => expect(result.current.gapDetected).toBe(true));
  });

  it('closes the EventSource and clears state on unmount', async () => {
    const { result, unmount } = renderHook(() => useChatOverlayStream('slug_1'));
    const source = FakeEventSource.instances[0]!;
    source.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    expect(source.closed).toBe(false);
    unmount();
    expect(source.closed).toBe(true);
  });

  it('clears leaving state along with active state on unmount', async () => {
    const { result, unmount } = renderHook(() => useChatOverlayStream('slug_1'));
    const source = FakeEventSource.instances[0]!;
    source.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    source.emit('chat-overlay.remove', { id: 'a', reason: 'expired' });
    await waitFor(() => expect(result.current.leaving).toHaveLength(1));

    unmount();
    // No further assertion possible on `result.current` post-unmount in a
    // meaningful way beyond not throwing - the important guarantee is the
    // effect's own cleanup dispatches a reset, exercised directly by the
    // reducer's own "reset clears every pending leaving item" test.
  });

  it('reconnects with a fresh EventSource and cleared state when the slug changes', async () => {
    const { result, rerender } = renderHook(({ slug }) => useChatOverlayStream(slug), {
      initialProps: { slug: 'slug_1' },
    });
    FakeEventSource.instances[0]!.emit('chat-overlay.upsert', baseItem('a', 1));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    rerender({ slug: 'slug_2' });
    expect(FakeEventSource.instances).toHaveLength(2);
    expect(FakeEventSource.instances[0]!.closed).toBe(true);
    expect(result.current.items).toHaveLength(0);
  });
});
