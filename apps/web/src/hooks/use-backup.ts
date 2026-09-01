import { useMutation, useQueryClient, type UseMutationResult } from '@tanstack/react-query';

import { cancelRestoreBackupPreview, commitRestoreBackup, exportBackup, previewRestoreBackup } from '@/api/backup';
import type { RestoreBackupPreview, RestoreBackupResult } from '@/api/backup-schemas';

/** Never cached by React Query - each of these is a one-shot action,
 * not read state, exactly like the visual-template-package hooks this
 * mirrors. */

export function useExportBackupMutation(): UseMutationResult<{ blob: Blob; filename: string }, Error, void> {
  return useMutation({
    mutationFn: exportBackup,
  });
}

/** Persists nothing - see previewRestoreBackup's own doc comment. */
export function usePreviewRestoreBackupMutation(): UseMutationResult<RestoreBackupPreview, Error, File> {
  return useMutation({
    mutationFn: (file) => previewRestoreBackup(file),
  });
}

/** Best-effort - see cancelRestoreBackupPreview's own doc comment.
 * Never surfaces an error to the caller. */
export function useCancelRestoreBackupPreviewMutation(): UseMutationResult<void, Error, string> {
  return useMutation({
    mutationFn: (token) => cancelRestoreBackupPreview(token),
  });
}

/** A successful commit replaces the ENTIRE current configuration
 * (docs/backup-restore.md §7's "Mode: REPLACE"), so every previously
 * cached query is invalidated wholesale rather than picking specific
 * keys - anything narrower would risk leaving some panel showing
 * pre-restore state. `RestoreBackupResult.restartRequired` is always
 * true regardless; the caller is responsible for prompting the
 * operator to restart. */
export function useCommitRestoreBackupMutation(): UseMutationResult<RestoreBackupResult, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (token) => commitRestoreBackup(token),
    onSuccess: () => {
      void queryClient.invalidateQueries();
    },
  });
}
