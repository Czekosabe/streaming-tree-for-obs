import { screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as logsApi from '@/api/logs';
import type { LogEntry } from '@/api/logs-schemas';
import * as visualtemplateModel from '@/models/visualtemplate';
import { renderWithProviders } from '@/test/render';

import { LogsPage } from './LogsPage';

vi.mock('@/api/logs');
vi.mock('@/models/visualtemplate');

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <LogsPage />
    </MemoryRouter>,
  );
}

function baseEntry(overrides: Partial<LogEntry> = {}): LogEntry {
  return {
    time: '2026-08-24T10:00:00Z',
    severity: 'INFO',
    subsystem: 'chatoverlay',
    message: 'chat overlay projection rebuilt',
    seq: 1,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(logsApi).fetchLogs.mockResolvedValue({ entries: [] });
});

describe('LogsPage', () => {
  it('shows the empty state with no entries', async () => {
    renderPage();
    expect(await screen.findByText(/no log entries yet/i)).toBeInTheDocument();
  });

  it('lists a fetched log entry with its severity, subsystem and message', async () => {
    vi.mocked(logsApi).fetchLogs.mockResolvedValue({ entries: [baseEntry()] });
    renderPage();

    expect(await screen.findByText('chat overlay projection rebuilt')).toBeInTheDocument();
    expect(screen.getByText('chatoverlay')).toBeInTheDocument();
    // "Info" also appears as a <select> option, so this proves the
    // severity badge rendered too, not just the option list.
    expect(screen.getAllByText('Info').length).toBeGreaterThan(1);
  });

  it('shows a backend-unavailable message when the request fails', async () => {
    vi.mocked(logsApi).fetchLogs.mockRejectedValue(new Error('network down'));
    renderPage();

    expect(await screen.findByText(/logs are unavailable right now/i)).toBeInTheDocument();
  });

  it('re-requests logs with the selected severity filter', async () => {
    vi.mocked(logsApi).fetchLogs.mockResolvedValue({ entries: [baseEntry()] });
    renderPage();
    await screen.findByText('chat overlay projection rebuilt');

    const severitySelect = screen.getByLabelText(/severity/i);
    severitySelect.dispatchEvent(new Event('focus'));
    (severitySelect as HTMLSelectElement).value = 'ERROR';
    severitySelect.dispatchEvent(new Event('change', { bubbles: true }));

    await waitFor(() =>
      expect(logsApi.fetchLogs).toHaveBeenCalledWith(
        expect.objectContaining({ severity: 'ERROR' }),
      ),
    );
  });

  it('shows a "load older" button when the backend reports a next cursor, and pages backward on click', async () => {
    vi.mocked(logsApi)
      .fetchLogs.mockResolvedValueOnce({ entries: [baseEntry({ seq: 5 })], nextCursor: 5 })
      .mockResolvedValueOnce({ entries: [baseEntry({ seq: 4, message: 'older entry' })] });
    renderPage();
    await screen.findByText('chat overlay projection rebuilt');

    const loadOlder = await screen.findByRole('button', { name: /load older entries/i });
    loadOlder.click();

    expect(await screen.findByText('older entry')).toBeInTheDocument();
    await waitFor(() =>
      expect(logsApi.fetchLogs).toHaveBeenCalledWith(expect.objectContaining({ before: 5 })),
    );
  });

  it('exports the support bundle and triggers a browser download of it', async () => {
    const blob = new Blob(['zip bytes']);
    vi.mocked(logsApi).fetchSupportBundle.mockResolvedValue({ blob, filename: 'bundle.zip' });
    renderPage();

    (await screen.findByRole('button', { name: /export support bundle/i })).click();

    await waitFor(() => expect(logsApi.fetchSupportBundle).toHaveBeenCalled());
    await waitFor(() =>
      expect(visualtemplateModel.downloadBlob).toHaveBeenCalledWith(blob, 'bundle.zip'),
    );
    expect(await screen.findByText(/support bundle downloaded/i)).toBeInTheDocument();
  });

  it('shows a failure message when the support bundle request fails', async () => {
    vi.mocked(logsApi).fetchSupportBundle.mockRejectedValue(new Error('boom'));
    renderPage();

    (await screen.findByRole('button', { name: /export support bundle/i })).click();

    expect(await screen.findByText(/support bundle could not be generated/i)).toBeInTheDocument();
  });
});
