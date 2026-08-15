import { screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import * as audioApi from '@/api/audio';
import { renderWithProviders } from '@/test/render';

import { PublicAudioPage } from './PublicAudioPage';

vi.mock('@/api/audio');

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
    <MemoryRouter initialEntries={[`/overlay/audio/${slug}`]}>
      <Routes>
        <Route path="/overlay/audio/:publicSlug" element={<PublicAudioPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
  FakeEventSource.instances = [];
  vi.mocked(audioApi).ackPublicAudio.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('PublicAudioPage', () => {
  it('never renders the application shell', () => {
    renderPage();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('renders a hidden audio element and nothing else visible', () => {
    renderPage();
    const audio = screen.getByTestId('audio-renderer-element');
    expect(audio).toBeInTheDocument();
    expect(audio).toHaveClass('hidden');
  });

  it('has no src before any audio.current event arrives', () => {
    renderPage();
    const audio = screen.getByTestId('audio-renderer-element') as HTMLAudioElement;
    expect(audio.getAttribute('src')).toBeNull();
  });

  it('sets the audio element src once audio.reset and audio.current arrive', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    expect(source).toBeDefined();
    source?.emit('audio.reset', { rendererToken: 'tok_1' });
    source?.emit('audio.current', {
      itemId: 'auditem_1',
      bytesUrl: '/api/public/audio/slug_1/bytes/tok',
      contentType: 'audio/wav',
      volume: 1,
    });

    await waitFor(() => {
      const audio = screen.getByTestId('audio-renderer-element') as HTMLAudioElement;
      expect(audio.src).toContain('/api/public/audio/slug_1/bytes/tok');
    });
  });

  it('clears the src again once audio.idle arrives', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    source?.emit('audio.reset', { rendererToken: 'tok_1' });
    source?.emit('audio.current', {
      itemId: 'auditem_1',
      bytesUrl: '/api/public/audio/slug_1/bytes/tok',
      contentType: 'audio/wav',
      volume: 1,
    });
    await waitFor(() => {
      const audio = screen.getByTestId('audio-renderer-element') as HTMLAudioElement;
      expect(audio.src).not.toBe('');
    });

    source?.emit('audio.idle', {});

    await waitFor(() => {
      const audio = screen.getByTestId('audio-renderer-element') as HTMLAudioElement;
      expect(audio.getAttribute('src')).toBeNull();
    });
  });

  it('connects to the exact public audio stream URL for the given slug', () => {
    renderPage('my-slug-123');
    expect(FakeEventSource.instances[0]?.url).toContain('/api/public/audio/my-slug-123/stream');
  });
});
