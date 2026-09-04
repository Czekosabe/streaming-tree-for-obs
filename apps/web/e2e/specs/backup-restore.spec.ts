/**
 * Real-browser smoke coverage for the Settings "Backup & Restore" panel
 * (docs/backup-restore.md), added alongside the Stage 23F persisted-
 * data-integrity hardening pass. Exercises the real export -> real
 * browser download -> real restore-preview round trip against the
 * hermetic backend.
 *
 * Deliberately never clicks "Restore this backup…": a committed restore
 * is REPLACE-semantics-destructive and always leaves the server
 * reporting `restartRequired: true` (docs/backup-restore.md §7 step 8),
 * which this suite's single shared hermetic backend process can never
 * actually perform mid-run - the panel's own post-commit screen only
 * offers "quit the app", not "restart it". This spec proves the
 * non-destructive half of the flow for real (a real exported file is
 * uploaded back and the backend validates/stages/previews it for real),
 * then cancels the staged preview to leave the shared backend
 * untouched for every other spec in this run.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('Settings / Backup & Restore', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('exports a real backup and previews restoring it, without committing', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.getByRole('heading', { name: 'Backup & Restore' })).toBeVisible();

    const downloadPromise = page.waitForEvent('download');
    await page.getByTestId('backup-export-button').click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toMatch(/\.streaming-tree-backup$/);

    const backupPath = await download.path();
    if (backupPath === null) {
      throw new Error('the browser did not save the downloaded backup to disk');
    }

    await page.getByTestId('backup-restore-file-input').setInputFiles(backupPath);

    // Real backend round trip: the uploaded bytes were validated,
    // staged under a token, and summarized - a fabricated/garbage file
    // would surface previewError instead of this panel.
    const preview = page.getByTestId('backup-restore-preview');
    await expect(preview).toBeVisible();
    await expect(preview).toContainText(/destinations?/i);

    await preview.getByRole('button', { name: 'Cancel' }).click();
    await expect(preview).toHaveCount(0);
    await expect(page.getByTestId('backup-restore-choose-button')).toBeVisible();
  });

  test('rejects a file that is not a valid backup, without staging anything', async ({ page, expectFailedResource }) => {
    // Deliberately provoked: the backend correctly answers 422 for a
    // garbage upload, which Chrome itself always logs as a console.error
    // regardless of whether the application handled it (fixtures.ts's own
    // doc comment) - the real assertion below is that the UI's own
    // handling is correct, not that the network layer stays silent.
    expectFailedResource(/\/api\/backup\/restore\/preview$/);
    await page.goto('/settings');

    await page.getByTestId('backup-restore-file-input').setInputFiles({
      name: 'not-a-backup.streaming-tree-backup',
      mimeType: 'application/octet-stream',
      buffer: Buffer.from('this is not a real zip archive'),
    });

    // The backend's real validation rejects it - no raw stack trace or
    // internal path, just the panel's own translated error copy - and
    // the preview panel never renders.
    await expect(page.getByText('This file could not be read as a backup.')).toBeVisible();
    await expect(page.getByTestId('backup-restore-preview')).toHaveCount(0);
  });
});
