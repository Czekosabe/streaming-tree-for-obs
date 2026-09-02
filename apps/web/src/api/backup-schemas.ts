import { z } from 'zod';

/**
 * Zod contracts for the Stage 23 configuration backup/restore API
 * (`internal/httpapi/backup.go`). See docs/backup-restore.md for the
 * full contract - notably §1/§2: a backup never carries a stream key,
 * OAuth token, donation-source credential, or any other secret, so
 * nothing here has a "credential" field to validate in the first
 * place.
 */

export const backupObjectCountsSchema = z.object({
  platforms: z.number(),
  connectedAccounts: z.number(),
  chatOverlays: z.number(),
  chatSchedules: z.number(),
  chatCommands: z.number(),
  alertProfiles: z.number(),
  alertRules: z.number(),
  visualTemplates: z.number(),
  visualAssets: z.number(),
  audioAssets: z.number(),
  goals: z.number(),
  widgetProfiles: z.number(),
  metadataPresets: z.number(),
  streamSetupProfiles: z.number(),
  donationSources: z.number(),
});
export type BackupObjectCounts = z.infer<typeof backupObjectCountsSchema>;

export const backupManifestSchema = z.object({
  formatVersion: z.number(),
  product: z.string(),
  createdAt: z.string(),
  sourceAppVersion: z.string(),
  sourcePlatform: z.string(),
});
export type BackupManifest = z.infer<typeof backupManifestSchema>;

/** Nothing here is a real database record - a bounded, named summary
 * only (docs/backup-restore.md §18), matching the preview session the
 * backend actually staged under `token`. */
export const restoreBackupPreviewSchema = z.object({
  token: z.string(),
  manifest: backupManifestSchema,
  counts: backupObjectCountsSchema,
  assetCount: z.number(),
  assetTotalBytes: z.number(),
  expiresAt: z.string(),
  connectedAccountsRequireReconnect: z.number(),
  destinationsNeedStreamKey: z.number(),
  donationSourcesNeedCredential: z.number(),
});
export type RestoreBackupPreview = z.infer<typeof restoreBackupPreviewSchema>;

export const restoreBackupResultSchema = z.object({
  counts: backupObjectCountsSchema,
  connectedAccountsRequireReconnect: z.number(),
  destinationsNeedStreamKey: z.number(),
  donationSourcesNeedCredential: z.number(),
  restartRequired: z.boolean(),
});
export type RestoreBackupResult = z.infer<typeof restoreBackupResultSchema>;
