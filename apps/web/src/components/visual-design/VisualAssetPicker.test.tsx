import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import * as visualAssetApi from '@/api/visualasset';
import type { VisualAsset } from '@/api/visualasset-schemas';
import { renderWithProviders } from '@/test/render';

import { VisualAssetPicker } from './VisualAssetPicker';

vi.mock('@/api/visualasset');

function imageAsset(overrides: Partial<VisualAsset> = {}): VisualAsset {
  return {
    id: 'asset_1', kind: 'image', mediaType: 'image/png', sizeBytes: 100,
    displayName: 'Badge', author: '', license: '', notice: '', source: 'upload',
    url: '/api/public/visual-assets/tok1', referenceCount: 0,
    createdAt: '2026-08-12T00:00:00.000Z', updatedAt: '2026-08-12T00:00:00.000Z',
    ...overrides,
  };
}

afterEach(() => {
  vi.clearAllMocks();
});

describe('VisualAssetPicker', () => {
  it('lists only assets of the requested kind', async () => {
    vi.mocked(visualAssetApi).fetchVisualAssets.mockResolvedValue([
      imageAsset({ id: 'asset_img' }),
      imageAsset({ id: 'asset_font', kind: 'font', displayName: 'MyFont' }),
    ]);
    renderWithProviders(<VisualAssetPicker open onClose={vi.fn()} kind="image" onSelect={vi.fn()} />);
    await screen.findByTestId('asset-picker-item-asset_img');
    expect(screen.queryByTestId('asset-picker-item-asset_font')).not.toBeInTheDocument();
  });

  it('shows an empty state when no asset of that kind exists', async () => {
    vi.mocked(visualAssetApi).fetchVisualAssets.mockResolvedValue([]);
    renderWithProviders(<VisualAssetPicker open onClose={vi.fn()} kind="video" onSelect={vi.fn()} />);
    await waitFor(() => expect(vi.mocked(visualAssetApi).fetchVisualAssets).toHaveBeenCalled());
    expect(screen.queryByTestId('asset-picker-list')).not.toBeInTheDocument();
  });

  it('selecting an existing asset calls onSelect with it', async () => {
    vi.mocked(visualAssetApi).fetchVisualAssets.mockResolvedValue([imageAsset()]);
    const onSelect = vi.fn();
    renderWithProviders(<VisualAssetPicker open onClose={vi.fn()} kind="image" onSelect={onSelect} />);
    fireEvent.click(await screen.findByTestId('asset-picker-item-asset_1'));
    expect(onSelect).toHaveBeenCalledWith(imageAsset());
  });

  it('uploading a new file calls the upload API and selects the result, never a URL-import path', async () => {
    vi.mocked(visualAssetApi).fetchVisualAssets.mockResolvedValue([]);
    const uploaded = imageAsset({ id: 'asset_new' });
    vi.mocked(visualAssetApi).uploadVisualAsset.mockResolvedValue(uploaded);
    const onSelect = vi.fn();
    renderWithProviders(<VisualAssetPicker open onClose={vi.fn()} kind="image" onSelect={onSelect} />);

    const file = new File(['fake image bytes'], 'badge.png', { type: 'image/png' });
    const input = screen.getByLabelText(/upload a new image/i);
    fireEvent.change(input, { target: { files: [file] } });

    await waitFor(() => expect(vi.mocked(visualAssetApi).uploadVisualAsset).toHaveBeenCalled());
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith(uploaded));
  });

  it('never renders a URL-input affordance - a file picker is the only upload path', () => {
    vi.mocked(visualAssetApi).fetchVisualAssets.mockResolvedValue([]);
    renderWithProviders(<VisualAssetPicker open onClose={vi.fn()} kind="image" onSelect={vi.fn()} />);
    expect(screen.queryByRole('textbox', { name: /url/i })).not.toBeInTheDocument();
  });
});
