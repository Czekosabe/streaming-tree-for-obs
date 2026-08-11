import { describe, expect, it } from 'vitest';

import { visualAssetListSchema, visualAssetSchema } from './visualasset-schemas';

function validAsset(overrides: Record<string, unknown> = {}) {
  return {
    id: 'asset_abc123',
    kind: 'image',
    mediaType: 'image/png',
    sizeBytes: 12345,
    displayName: 'Corner badge',
    author: '',
    license: '',
    notice: '',
    source: 'upload',
    url: '/api/public/visual-assets/tok_abc',
    referenceCount: 0,
    createdAt: '2026-08-12T00:00:00.000Z',
    updatedAt: '2026-08-12T00:00:00.000Z',
    ...overrides,
  };
}

describe('visualAssetSchema', () => {
  it('parses a valid managed asset', () => {
    const parsed = visualAssetSchema.parse(validAsset());
    expect(parsed.id).toBe('asset_abc123');
    expect(parsed.kind).toBe('image');
    expect(parsed.url).toBe('/api/public/visual-assets/tok_abc');
  });

  it.each(['image', 'video', 'font'])('accepts kind %s', (kind) => {
    expect(visualAssetSchema.parse(validAsset({ kind })).kind).toBe(kind);
  });

  it.each(['upload', 'package'])('accepts source %s', (source) => {
    expect(visualAssetSchema.parse(validAsset({ source })).source).toBe(source);
  });

  it('rejects an unrecognized kind', () => {
    expect(visualAssetSchema.safeParse(validAsset({ kind: 'audio' })).success).toBe(false);
  });

  it('never exposes a local filesystem path or blob hash field - only the fields this contract names', () => {
    const parsed = visualAssetSchema.parse(validAsset());
    expect('path' in parsed).toBe(false);
    expect('sha256' in parsed).toBe(false);
  });
});

describe('visualAssetListSchema', () => {
  it('parses an array of assets', () => {
    const parsed = visualAssetListSchema.parse([validAsset(), validAsset({ id: 'asset_2' })]);
    expect(parsed).toHaveLength(2);
  });

  it('parses an empty list', () => {
    expect(visualAssetListSchema.parse([])).toEqual([]);
  });
});
