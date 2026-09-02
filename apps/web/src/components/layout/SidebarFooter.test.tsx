import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as aboutApi from '@/api/about';
import * as runtimeApi from '@/api/runtime';
import { renderWithProviders } from '@/test/render';

import { SidebarFooter } from './SidebarFooter';

vi.mock('@/api/runtime');
vi.mock('@/api/about');

const MEDIAMTX_READY = {
  supportedVersion: '1.19.3',
  source: 'managed' as const,
  state: 'ready' as const,
  autoStart: true,
  autoRestart: true,
  restartCount: 0,
  lastError: null,
};

/** Stage 20E defect B: the OBS connection card collapses by default so the
 * primary navigation above it gets materially more vertical room, while the
 * heading/status/error stay visible regardless of collapse state. */
describe('SidebarFooter', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(aboutApi).fetchAbout.mockReturnValue(new Promise(() => {}));
  });

  it('starts collapsed: the status line is visible, but stream key/server detail is not', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: MEDIAMTX_READY,
      ingest: { state: 'waiting', path: 'live', trackCount: null, tracks: [] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<SidebarFooter />);

    expect(await screen.findByText(/waiting for obs/i)).toBeInTheDocument();
    expect(screen.getByText('Stream key')).not.toBeVisible();

    const toggle = screen.getByRole('button', { name: /show connection details/i });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
  });

  it('expanding the toggle reveals the server/stream-key detail and controls', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: MEDIAMTX_READY,
      ingest: { state: 'waiting', path: 'live', trackCount: null, tracks: [] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<SidebarFooter />);
    await screen.findByText(/waiting for obs/i);

    await userEvent.click(screen.getByRole('button', { name: /show connection details/i }));

    expect(screen.getByText('Stream key')).toBeVisible();
    expect(screen.getByRole('button', { name: /hide connection details/i })).toHaveAttribute(
      'aria-expanded',
      'true',
    );
  });

  it('always shows a real ingest error, even while collapsed', async () => {
    vi.mocked(runtimeApi).fetchRuntime.mockResolvedValue({
      version: 1,
      mediaMtx: {
        ...MEDIAMTX_READY,
        state: 'error',
        lastError: { code: 'mediamtx_port_in_use', message: 'The port is already in use.' },
      },
      ingest: { state: 'unavailable', path: 'live', trackCount: null, tracks: [] },
      connection: { serverUrl: 'rtmp://127.0.0.1:1935', streamKey: 'live', publishUrl: 'rtmp://127.0.0.1:1935/live' },
    });

    renderWithProviders(<SidebarFooter />);

    expect(await screen.findByText(/configured port is already used/i)).toBeVisible();
    expect(screen.getByRole('button', { name: /show connection details/i })).toHaveAttribute(
      'aria-expanded',
      'false',
    );
  });
});
