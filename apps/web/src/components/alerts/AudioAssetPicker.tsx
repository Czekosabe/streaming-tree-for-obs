import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { AudioAsset } from '@/api/audioasset-schemas';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { useAudioAssetsQuery, useUploadAudioAssetMutation } from '@/hooks/use-audio-assets';

/**
 * Managed audio-asset picker (Stage 17B, docs/alert-audio.md §5/§7) -
 * mirrors components/visual-design/VisualAssetPicker.tsx's own upload/
 * gallery shape exactly, for the same UX consistency across every
 * managed-asset kind this app has. Unlike a visual asset, an audio
 * asset has no public content URL to preview inline (audioAssetSchema
 * has no `url` field) - each entry shows its display name and duration
 * only. Always offers a real file-picker upload path, never a URL
 * import.
 */

function formatDurationMs(durationMs: number): string {
  const totalSeconds = Math.round(durationMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}

export function AudioAssetPicker({
  open,
  onClose,
  onSelect,
}: {
  open: boolean;
  onClose: () => void;
  onSelect: (asset: AudioAsset) => void;
}) {
  const { t } = useTranslation('alerts');
  const assetsQuery = useAudioAssetsQuery({ enabled: open });
  const uploadMutation = useUploadAudioAssetMutation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);

  const assets = assetsQuery.data ?? [];

  function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (file === undefined) return;
    setUploadError(null);
    uploadMutation.mutate(
      { file, displayName: file.name },
      {
        onSuccess: (asset) => onSelect(asset),
        onError: (error) => setUploadError(error.message),
      },
    );
  }

  return (
    <Modal open={open} onClose={onClose} title={t('audioAssetPicker.title')} size="md">
      <div className="space-y-3">
        <Button
          variant="secondary"
          onClick={() => fileInputRef.current?.click()}
          disabled={uploadMutation.isPending}
          data-testid="audio-asset-picker-upload"
        >
          {uploadMutation.isPending ? t('audioAssetPicker.uploading') : t('audioAssetPicker.uploadNew')}
        </Button>
        <input
          ref={fileInputRef}
          type="file"
          accept=".wav,audio/wav,audio/x-wav,audio/wave"
          className="hidden"
          onChange={handleFileChange}
          aria-label={t('audioAssetPicker.uploadLabel')}
        />
        {uploadError !== null && <p className="text-xs text-status-error">{uploadError}</p>}

        {assetsQuery.isLoading ? (
          <p className="text-xs text-ink-muted">{t('audioAssetPicker.loading')}</p>
        ) : assets.length === 0 ? (
          <p className="text-xs text-ink-muted">{t('audioAssetPicker.empty')}</p>
        ) : (
          <ul className="space-y-1" data-testid="audio-asset-picker-list">
            {assets.map((asset) => (
              <li key={asset.id}>
                <button
                  type="button"
                  onClick={() => onSelect(asset)}
                  className="flex w-full items-center justify-between gap-2 rounded-lg border border-line p-2 text-left hover:border-accent"
                  data-testid={`audio-asset-picker-item-${asset.id}`}
                >
                  <span className="truncate text-sm text-ink">{asset.displayName || asset.id}</span>
                  <span className="shrink-0 text-[11px] text-ink-muted">{formatDurationMs(asset.durationMs)}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
