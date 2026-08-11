import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { useUploadVisualAssetMutation, useVisualAssetsQuery } from '@/hooks/use-visual-assets';
import type { VisualAsset, VisualAssetKind } from '@/api/visualasset-schemas';

/**
 * Shared managed-asset picker (Stage 14B task Part 32/54: "Reuse ONE
 * shared asset-picker/library component for both Designers - do not
 * create separate alert/chat asset systems"). Used for choosing an
 * image/video asset for a new or existing layer, and for choosing an
 * optional custom WOFF2 font asset for a text-capable layer. Always
 * offers a real file-picker upload path (docs/visual-template-
 * packages.md §32: "no URL import... a normal file picker must always
 * exist").
 */

const ACCEPT_BY_KIND: Record<VisualAssetKind, string> = {
  image: 'image/png,image/jpeg,image/gif,image/webp',
  video: 'video/webm,video/mp4',
  font: '.woff2,font/woff2',
};

export function VisualAssetPicker({
  open,
  onClose,
  kind,
  onSelect,
}: {
  open: boolean;
  onClose: () => void;
  kind: VisualAssetKind;
  onSelect: (asset: VisualAsset) => void;
}) {
  const { t } = useTranslation('alertDesigner');
  const assetsQuery = useVisualAssetsQuery({ enabled: open });
  const uploadMutation = useUploadVisualAssetMutation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);

  const assets = (assetsQuery.data ?? []).filter((a) => a.kind === kind);

  function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (file === undefined) return;
    setUploadError(null);
    uploadMutation.mutate(
      { file, metadata: { displayName: file.name } },
      {
        onSuccess: (asset) => onSelect(asset),
        onError: (error) => setUploadError(error.message),
      },
    );
  }

  return (
    <Modal open={open} onClose={onClose} title={t(`assetPicker.title.${kind}`)} size="md">
      <div className="space-y-3">
        <Button
          variant="secondary"
          onClick={() => fileInputRef.current?.click()}
          disabled={uploadMutation.isPending}
          data-testid="asset-picker-upload"
        >
          {uploadMutation.isPending ? t('assetPicker.uploading') : t('assetPicker.uploadNew')}
        </Button>
        <input
          ref={fileInputRef}
          type="file"
          accept={ACCEPT_BY_KIND[kind]}
          className="hidden"
          onChange={handleFileChange}
          aria-label={t(`assetPicker.uploadLabel.${kind}`)}
        />
        {uploadError !== null && <p className="text-xs text-status-error">{uploadError}</p>}

        {assetsQuery.isLoading ? (
          <p className="text-xs text-ink-muted">{t('assetPicker.loading')}</p>
        ) : assets.length === 0 ? (
          <p className="text-xs text-ink-muted">{t('assetPicker.empty')}</p>
        ) : (
          <ul className="grid grid-cols-3 gap-2" data-testid="asset-picker-list">
            {assets.map((asset) => (
              <li key={asset.id}>
                <button
                  type="button"
                  onClick={() => onSelect(asset)}
                  className="flex w-full flex-col items-center gap-1 rounded-lg border border-line p-2 text-center hover:border-accent"
                  data-testid={`asset-picker-item-${asset.id}`}
                >
                  {asset.kind === 'image' ? (
                    <img src={asset.url} alt="" className="h-16 w-16 rounded object-cover" />
                  ) : (
                    <div className="flex h-16 w-16 items-center justify-center rounded bg-surface-raised text-[10px] uppercase text-ink-muted">
                      {asset.kind}
                    </div>
                  )}
                  <span className="w-full truncate text-[11px] text-ink-muted">{asset.displayName || asset.id}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
