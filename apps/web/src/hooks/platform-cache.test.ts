import { describe, expect, it } from 'vitest';

import type { ConfiguredPlatform, PlatformMetadata } from '@/api/platform-schemas';

import { removePlatform, replaceMetadata, replacePlatform } from './platform-cache';

function metadata(overrides: Partial<PlatformMetadata> = {}): PlatformMetadata {
  return {
    title: '',
    description: '',
    category: '',
    categoryId: '',
    tags: [],
    language: '',
    visibility: '',
    matureContent: false,
    dvr: false,
    latencyMode: '',
    updatedAt: '2026-08-03T12:00:00Z',
    ...overrides,
  };
}

function platform(id: string, overrides: Partial<ConfiguredPlatform> = {}): ConfiguredPlatform {
  return {
    id,
    providerId: 'twitch',
    displayName: id,
    enabled: false,
    sortOrder: 0,
    createdAt: '2026-08-03T12:00:00Z',
    updatedAt: '2026-08-03T12:00:00Z',
    metadata: metadata(),
    ...overrides,
  };
}

const cache = [platform('a'), platform('b'), platform('c')];

describe('replacePlatform', () => {
  it('replaces the matching row and keeps the order', () => {
    const updated = platform('b', { displayName: 'renamed', enabled: true });
    const result = replacePlatform(cache, updated);

    expect(result?.map((entry) => entry.id)).toEqual(['a', 'b', 'c']);
    expect(result?.[1]?.displayName).toBe('renamed');
    expect(result?.[1]?.enabled).toBe(true);
  });

  it('leaves the list unchanged when the id is not cached', () => {
    const result = replacePlatform(cache, platform('missing'));

    expect(result?.map((entry) => entry.id)).toEqual(['a', 'b', 'c']);
  });

  it('passes an empty cache through', () => {
    expect(replacePlatform(undefined, platform('a'))).toBeUndefined();
  });
});

describe('removePlatform', () => {
  it('removes only the deleted row', () => {
    const result = removePlatform(cache, 'b');

    expect(result?.map((entry) => entry.id)).toEqual(['a', 'c']);
  });

  it('is a no-op for an id that is not cached', () => {
    expect(removePlatform(cache, 'missing')?.map((entry) => entry.id)).toEqual(['a', 'b', 'c']);
  });

  it('passes an empty cache through', () => {
    expect(removePlatform(undefined, 'a')).toBeUndefined();
  });
});

describe('replaceMetadata', () => {
  it('replaces metadata without touching configuration fields', () => {
    const enabledCache = [platform('a', { enabled: true, sortOrder: 7, displayName: 'Main' })];
    const saved = metadata({ title: 'New title', tags: ['go', 'react'] });

    const result = replaceMetadata(enabledCache, 'a', saved);

    expect(result?.[0]?.metadata.title).toBe('New title');
    expect(result?.[0]?.metadata.tags).toEqual(['go', 'react']);
    // Configuration must survive a metadata save untouched.
    expect(result?.[0]?.enabled).toBe(true);
    expect(result?.[0]?.sortOrder).toBe(7);
    expect(result?.[0]?.displayName).toBe('Main');
  });

  it('leaves other platforms alone', () => {
    const result = replaceMetadata(cache, 'b', metadata({ title: 'Only b' }));

    expect(result?.[0]?.metadata.title).toBe('');
    expect(result?.[1]?.metadata.title).toBe('Only b');
    expect(result?.[2]?.metadata.title).toBe('');
  });

  it('passes an empty cache through', () => {
    expect(replaceMetadata(undefined, 'a', metadata())).toBeUndefined();
  });
});
