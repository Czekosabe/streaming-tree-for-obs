import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import * as backupApi from '@/api/backup';
import * as systemApi from '@/api/system';
import { ApiError } from '@/lib/api-client';
import { i18n } from '@/i18n';
import * as visualtemplateModel from '@/models/visualtemplate';
import { renderWithProviders } from '@/test/render';

import { BackupRestorePanel } from './BackupRestorePanel';

vi.mock('@/api/backup');
vi.mock('@/api/system');
vi.mock('@/models/visualtemplate', async (importOriginal) => {
  const actual = await importOriginal<typeof visualtemplateModel>();
  return { ...actual, downloadBlob: vi.fn() };
});

const PREVIEW = {
  token: 'rst_abc',
  manifest: {
    formatVersion: 1,
    product: 'streaming-tree-for-obs-backup',
    createdAt: '2026-08-01T12:00:00Z',
    sourceAppVersion: '0.9.0',
    sourcePlatform: 'windows',
  },
  counts: {
    platforms: 2,
    connectedAccounts: 1,
    chatOverlays: 0,
    chatSchedules: 0,
    chatCommands: 0,
    alertProfiles: 0,
    alertRules: 0,
    visualTemplates: 0,
    visualAssets: 0,
    audioAssets: 0,
    goals: 0,
    widgetProfiles: 0,
    metadataPresets: 0,
    donationSources: 0,
  },
  assetCount: 0,
  assetTotalBytes: 0,
  expiresAt: '2026-08-01T12:10:00Z',
  connectedAccountsRequireReconnect: 1,
  destinationsNeedStreamKey: 2,
  donationSourcesNeedCredential: 0,
};

function renderPanel() {
  return renderWithProviders(<BackupRestorePanel />);
}

async function chooseFile() {
  const user = userEvent.setup();
  const file = new File(['fake-backup-bytes'], 'my.streaming-tree-backup');
  const input = screen.getByTestId('backup-restore-file-input');
  await user.upload(input, file);
  return user;
}

describe('BackupRestorePanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    void i18n.changeLanguage('en');
  });

  it('shows what a backup includes and never includes', () => {
    renderPanel();
    expect(screen.getByText(/destinations and output settings/i)).toBeInTheDocument();
    expect(screen.getByText(/stream keys/i)).toBeInTheDocument();
    expect(screen.getByText(/oauth\/refresh tokens/i)).toBeInTheDocument();
  });

  it('downloads a backup package when Export is clicked', async () => {
    const user = userEvent.setup();
    const blob = new Blob(['zip-bytes']);
    vi.mocked(backupApi).exportBackup.mockResolvedValue({ blob, filename: 'my-backup.streaming-tree-backup' });
    renderPanel();

    await user.click(screen.getByTestId('backup-export-button'));

    await waitFor(() => expect(backupApi.exportBackup).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(visualtemplateModel.downloadBlob).toHaveBeenCalledWith(blob, 'my-backup.streaming-tree-backup'),
    );
  });

  it('previews a chosen backup file and shows its counts and needs-attention items', async () => {
    vi.mocked(backupApi).previewRestoreBackup.mockResolvedValue(PREVIEW);
    renderPanel();

    await chooseFile();

    const preview = await screen.findByTestId('backup-restore-preview');
    expect(within(preview).getByText(/^2 destinations$/i)).toBeInTheDocument();
    expect(within(preview).getByText(/1 connected account will need to be reconnected/i)).toBeInTheDocument();
    expect(within(preview).getByText(/2 destinations will need their stream keys re-entered/i)).toBeInTheDocument();
  });

  it('cancels a staged preview server-side and returns to the choose-file state', async () => {
    vi.mocked(backupApi).previewRestoreBackup.mockResolvedValue(PREVIEW);
    vi.mocked(backupApi).cancelRestoreBackupPreview.mockResolvedValue(undefined);
    renderPanel();
    const user = await chooseFile();
    await screen.findByTestId('backup-restore-preview');

    await user.click(screen.getByRole('button', { name: /^cancel$/i }));

    await waitFor(() => expect(backupApi.cancelRestoreBackupPreview).toHaveBeenCalledWith('rst_abc'));
    expect(screen.getByTestId('backup-restore-choose-button')).toBeInTheDocument();
  });

  it('commits the previewed token after confirmation and shows the restart-required notice', async () => {
    vi.mocked(backupApi).previewRestoreBackup.mockResolvedValue(PREVIEW);
    vi.mocked(backupApi).commitRestoreBackup.mockResolvedValue({
      counts: PREVIEW.counts,
      connectedAccountsRequireReconnect: 1,
      destinationsNeedStreamKey: 2,
      donationSourcesNeedCredential: 0,
      restartRequired: true,
    });
    renderPanel();
    const user = await chooseFile();
    await screen.findByTestId('backup-restore-preview');

    await user.click(screen.getByTestId('backup-restore-confirm-button'));
    const dialog = await screen.findByRole('dialog', { name: /restore this backup\?/i });
    await user.click(within(dialog).getByRole('button', { name: /restore and prepare to restart/i }));

    await waitFor(() => expect(backupApi.commitRestoreBackup).toHaveBeenCalledWith('rst_abc'));
    expect(await screen.findByText(/restart required/i)).toBeInTheDocument();
  });

  it('quitting from the restart notice stops the application', async () => {
    vi.mocked(backupApi).previewRestoreBackup.mockResolvedValue(PREVIEW);
    vi.mocked(backupApi).commitRestoreBackup.mockResolvedValue({
      counts: PREVIEW.counts,
      connectedAccountsRequireReconnect: 0,
      destinationsNeedStreamKey: 0,
      donationSourcesNeedCredential: 0,
      restartRequired: true,
    });
    vi.mocked(systemApi).shutdownApplication.mockResolvedValue(undefined);
    renderPanel();
    const user = await chooseFile();
    await screen.findByTestId('backup-restore-preview');
    await user.click(screen.getByTestId('backup-restore-confirm-button'));
    const restoreDialog = await screen.findByRole('dialog', { name: /restore this backup\?/i });
    await user.click(within(restoreDialog).getByRole('button', { name: /restore and prepare to restart/i }));
    await screen.findByText(/restart required/i);

    await user.click(screen.getByRole('button', { name: /quit streaming tree now/i }));
    const quitDialog = await screen.findByRole('dialog', { name: /quit streaming tree\?/i });
    await user.click(within(quitDialog).getByRole('button', { name: /quit streaming tree now/i }));

    await waitFor(() => expect(systemApi.shutdownApplication).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(/streaming tree has stopped/i)).toBeInTheDocument();
  });

  it('shows the real reason when restore is refused because streaming is active', async () => {
    vi.mocked(backupApi).previewRestoreBackup.mockResolvedValue(PREVIEW);
    vi.mocked(backupApi).commitRestoreBackup.mockRejectedValue(
      new ApiError('http', 'Streaming is active.', { status: 409, code: 'restore_blocked_streaming_active' }),
    );
    renderPanel();
    const user = await chooseFile();
    await screen.findByTestId('backup-restore-preview');
    await user.click(screen.getByTestId('backup-restore-confirm-button'));
    const dialog = await screen.findByRole('dialog', { name: /restore this backup\?/i });
    await user.click(within(dialog).getByRole('button', { name: /restore and prepare to restart/i }));

    expect(await screen.findByText(/stop streaming before restoring a backup/i)).toBeInTheDocument();
    // The existing configuration was not touched - the panel stays on the preview, not the restart notice.
    expect(screen.getByTestId('backup-restore-preview')).toBeInTheDocument();
  });
});
