import { screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import * as alertsApi from '@/api/alerts';
import { renderWithProviders } from '@/test/render';

import { PublicAlertPage } from './PublicAlertPage';

vi.mock('@/api/alerts');

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
    <MemoryRouter initialEntries={[`/overlay/alerts/${slug}`]}>
      <Routes>
        <Route path="/overlay/alerts/:publicSlug" element={<PublicAlertPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

const baseConfig = {
  schemaVersion: 1,
  theme: 'minimal' as const,
  position: 'bottom' as const,
  textAlign: 'center' as const,
  language: 'en' as const,
};

function alertPayload(overrides: Record<string, unknown> = {}) {
  return {
    schemaVersion: 1,
    alertId: 'alinst_1',
    eventType: 'follow',
    providerId: 'twitch',
    synthetic: false,
    replayed: false,
    renderedText: 'Ann just followed!',
    groupCount: 1,
    durationMs: 5000,
    entryAnimation: 'none',
    exitAnimation: 'none',
    animationDurationMs: 0,
    ...overrides,
  };
}

/** Stage 12B: `alert.hide` has its own distinct payload shape - never
 * `{alert}`, only the hidden alert's own id and a stable reason. */
function hidePayload(overrides: Record<string, unknown> = {}) {
  return { paused: false, alertId: 'alinst_1', reason: 'completed', ...overrides };
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
  FakeEventSource.instances = [];
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('PublicAlertPage', () => {
  it('renders nothing but a bare div while the config has not loaded', () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(screen.getByTestId('alert-page-empty')).toBeInTheDocument();
  });

  it('never renders the application shell (no sidebar navigation landmark)', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('shows nothing before any alert.show event arrives', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');
    expect(screen.queryByTestId('alert-item')).not.toBeInTheDocument();
  });

  it('renders a live alert once the config loads and the stream shows one', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');

    FakeEventSource.instances[0]!.emit('alert.show', { paused: false, alert: alertPayload() });

    expect(await screen.findByText('Ann just followed!')).toBeInTheDocument();
  });

  it('hides the alert immediately when the server sends alert.hide (no animation configured)', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('alert.show', { paused: false, alert: alertPayload() });
    await screen.findByText('Ann just followed!');

    source.emit('alert.hide', hidePayload());

    await waitFor(() => expect(screen.queryByText('Ann just followed!')).not.toBeInTheDocument());
  });

  it('replaces the current alert immediately when a new one shows mid-display', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('alert.show', { paused: false, alert: alertPayload({ alertId: 'alinst_1', renderedText: 'First alert' }) });
    await screen.findByText('First alert');

    source.emit('alert.show', { paused: false, alert: alertPayload({ alertId: 'alinst_2', renderedText: 'Second alert' }) });

    expect(await screen.findByText('Second alert')).toBeInTheDocument();
    expect(screen.queryByText('First alert')).not.toBeInTheDocument();
  });

  it('an initial alert.reset with a non-null alert shows it immediately', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');

    FakeEventSource.instances[0]!.emit('alert.reset', {
      paused: false,
      alert: alertPayload({ renderedText: 'Reset carried a live alert' }),
    });

    expect(await screen.findByText('Reset carried a live alert')).toBeInTheDocument();
  });

  it('shows a missing-avatar/anonymous alert correctly (no username rendered)', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');

    FakeEventSource.instances[0]!.emit('alert.show', {
      paused: false,
      alert: alertPayload({ username: null, renderedText: 'An anonymous cheerer gave 500 bits!', quantity: 500 }),
    });

    expect(await screen.findByText('An anonymous cheerer gave 500 bits!')).toBeInTheDocument();
    expect(await screen.findByTestId('alert-quantity')).toHaveTextContent('500');
  });

  it('marks a synthetic (test) alert with data-synthetic so an operator preview can be distinguished, while still rendering identically', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');

    FakeEventSource.instances[0]!.emit('alert.show', {
      paused: false,
      alert: alertPayload({ synthetic: true, renderedText: 'Test alert text' }),
    });

    const item = await screen.findByTestId('alert-item');
    expect(item).toHaveAttribute('data-synthetic', 'true');
  });

  it('renders a grouped alert with its own group-count badge (Stage 12B)', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');

    FakeEventSource.instances[0]!.emit('alert.show', {
      paused: false,
      alert: alertPayload({ renderedText: 'Ann cheered 150 bits (x2)', quantity: 150, groupCount: 3 }),
    });

    expect(await screen.findByTestId('alert-group-count')).toHaveTextContent('×3');
    expect(await screen.findByTestId('alert-quantity')).toHaveTextContent('150');
  });

  it('never shows a group-count badge for an ungrouped (groupCount=1) alert', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');

    FakeEventSource.instances[0]!.emit('alert.show', { paused: false, alert: alertPayload({ groupCount: 1 }) });

    await screen.findByTestId('alert-item');
    expect(screen.queryByTestId('alert-group-count')).not.toBeInTheDocument();
  });

  it('a preempted alert leaves no old text behind once the urgent alert shows (Stage 12B)', async () => {
    vi.mocked(alertsApi).fetchPublicAlertProfileConfig.mockResolvedValue(baseConfig);
    renderPage();
    await screen.findByTestId('alert-root');
    const source = FakeEventSource.instances[0]!;

    source.emit('alert.show', { paused: false, alert: alertPayload({ alertId: 'alinst_1', renderedText: 'Low priority alert' }) });
    await screen.findByText('Low priority alert');

    // The real backend never sends prior content in a hide payload - only
    // id and reason (Part 20/36) - and the renderer must never keep
    // showing the outgoing alert's own text once it is gone.
    source.emit('alert.hide', hidePayload({ alertId: 'alinst_1', reason: 'preempted' }));
    source.emit('alert.show', { paused: false, alert: alertPayload({ alertId: 'alinst_2', renderedText: 'Urgent raid alert' }) });

    expect(await screen.findByText('Urgent raid alert')).toBeInTheDocument();
    expect(screen.queryByText('Low priority alert')).not.toBeInTheDocument();
  });
});
