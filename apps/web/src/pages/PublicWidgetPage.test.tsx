import { screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { PublicWidgetPage } from './PublicWidgetPage';

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
    <MemoryRouter initialEntries={[`/overlay/widgets/${slug}`]}>
      <Routes>
        <Route path="/overlay/widgets/:publicSlug" element={<PublicWidgetPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

function snapshot(overrides: Record<string, unknown> = {}) {
  return {
    revision: 1, kind: 'goal', goalKind: 'followers', title: 'Followers',
    current: 825, target: 1000, progressBasisPoints: 8250, completed: false,
    presentation: {
      showCurrent: true, showTarget: true, showPercent: true,
      orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
      backgroundColor: '#00000080', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
      borderRadiusPx: 12, opacity: 1.0,
    },
    ...overrides,
  };
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
  FakeEventSource.instances = [];
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('PublicWidgetPage', () => {
  it('never renders the application shell', () => {
    renderPage();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('renders nothing before the first widget.reset event arrives', () => {
    renderPage();
    expect(screen.queryByText('Followers')).not.toBeInTheDocument();
  });

  it('renders the goal title and progress once widget.reset arrives', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    expect(source).toBeDefined();
    source?.emit('widget.reset', snapshot());

    await waitFor(() => {
      expect(screen.getByText('Followers')).toBeInTheDocument();
    });
    expect(screen.getByText('825 / 1,000')).toBeInTheDocument();
  });

  it('updates when a second widget.reset arrives with a new snapshot', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    source?.emit('widget.reset', snapshot());
    await waitFor(() => expect(screen.getByText('825 / 1,000')).toBeInTheDocument());

    source?.emit('widget.reset', snapshot({ current: 999, revision: 2 }));
    await waitFor(() => expect(screen.getByText('999 / 1,000')).toBeInTheDocument());
  });

  it('never leaks internal identifiers into the rendered DOM', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    source?.emit('widget.reset', snapshot());
    await waitFor(() => expect(screen.getByText('Followers')).toBeInTheDocument());

    expect(document.body.textContent).not.toContain('goal_');
    expect(document.body.textContent).not.toContain('widget_');
  });

  it('connects to the exact public widget stream URL for the given slug', () => {
    renderPage('my-slug-123');
    expect(FakeEventSource.instances[0]?.url).toContain('/api/public/widgets/my-slug-123/stream');
  });

  it('renders a latest_follower widget\'s real display name once observed', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    source?.emit('widget.reset', snapshot({
      kind: 'latest_follower', goalKind: undefined, current: undefined, target: undefined,
      progressBasisPoints: undefined, completed: undefined,
      latest: { itemId: 'supitem_1', displayName: 'Ada', provider: 'twitch', observedAt: '2026-08-17T00:00:00Z' },
    }));
    await waitFor(() => expect(screen.getByText('Ada')).toBeInTheDocument());
  });

  it('shows the kind-specific empty state before any event is observed', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    source?.emit('widget.reset', snapshot({
      kind: 'latest_follower', goalKind: undefined, current: undefined, target: undefined,
      progressBasisPoints: undefined, completed: undefined,
    }));
    await waitFor(() => expect(screen.getByText(/no follower observed yet/i)).toBeInTheDocument());
  });

  it('renders a dashboard composing its own child snapshot, never an internal id', async () => {
    renderPage('slug_1');
    const source = FakeEventSource.instances[0];
    source?.emit('widget.reset', snapshot({
      kind: 'dashboard', goalKind: undefined, current: undefined, target: undefined,
      progressBasisPoints: undefined, completed: undefined,
      dashboard: [
        {
          key: 'dashboard_child_0', column: 1, columnSpan: 1, row: 1, rowSpan: 1,
          snapshot: {
            revision: 1, kind: 'latest_follower', title: 'Latest Follower',
            latest: { itemId: 'supitem_1', displayName: 'Bea', provider: 'twitch', observedAt: '2026-08-17T00:00:00Z' },
            presentation: {
              showCurrent: false, showTarget: false, showPercent: false,
              orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
              backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff',
              borderRadiusPx: 0, opacity: 1,
            },
          },
        },
      ],
      presentation: { showCurrent: false, showTarget: false, showPercent: false, columns: 1,
        orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
        backgroundColor: '#000', foregroundColor: '#fff', fillColor: '#fff', borderColor: '#fff', borderRadiusPx: 0, opacity: 1 },
    }));
    await waitFor(() => expect(screen.getByText('Bea')).toBeInTheDocument());
    expect(document.body.textContent).not.toContain('widget_');
  });
});
