import { describe, expect, it } from 'vitest';

import { visualTemplatePackagePreviewSchema } from './visualpackage-schemas';

function validPreview(overrides: Record<string, unknown> = {}) {
  return {
    token: 'preview_tok_abc',
    target: 'alert',
    name: 'Package test template',
    description: '',
    author: '',
    license: '',
    document: {
      version: 3,
      canvas: { width: 1920, height: 1080, transparent: true },
      layers: [
        {
          id: 'layer_1', name: 'Badge', kind: 'image', visible: true, locked: false, order: 0,
          frame: { x: 0, y: 0, width: 200, height: 200 }, opacity: 1,
          image: { assetId: 'pkgasset_0001', fit: 'contain', alt: '' },
          entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
        },
      ],
    },
    assets: [
      {
        packageAssetId: 'pkgasset_0001',
        kind: 'image',
        mediaType: 'image/png',
        sizeBytes: 1234,
        displayName: 'Corner badge',
        author: '',
        license: '',
        notice: '',
        url: '/api/visual-template-packages/preview/preview_tok_abc/assets/pkgasset_0001',
      },
    ],
    expiresAt: '2026-08-12T00:10:00.000Z',
    ...overrides,
  };
}

describe('visualTemplatePackagePreviewSchema', () => {
  it('parses a valid preview session, document still referencing package-local ids', () => {
    const parsed = visualTemplatePackagePreviewSchema.parse(validPreview());
    expect(parsed.token).toBe('preview_tok_abc');
    expect(parsed.assets).toHaveLength(1);
    expect(parsed.document.layers[0]?.image?.assetId).toBe('pkgasset_0001');
  });

  it('parses an asset-free package preview with zero assets', () => {
    const parsed = visualTemplatePackagePreviewSchema.parse(
      validPreview({ assets: [], document: { version: 3, canvas: { width: 1920, height: 1080, transparent: true }, layers: [] } }),
    );
    expect(parsed.assets).toEqual([]);
  });

  it('rejects a preview missing a required field', () => {
    const { token: _token, ...withoutToken } = validPreview();
    expect(visualTemplatePackagePreviewSchema.safeParse(withoutToken).success).toBe(false);
  });
});
