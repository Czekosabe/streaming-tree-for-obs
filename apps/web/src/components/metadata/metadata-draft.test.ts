import { describe, expect, it } from 'vitest';

import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { isDirty, toDraft } from './metadata-draft';

function platform(tags: string[] = ['go', 'react']): ConfiguredPlatform {
  return {
    id: 'pf_1',
    providerId: 'twitch',
    displayName: 'Main',
    enabled: false,
    sortOrder: 0,
    createdAt: '2026-08-03T12:00:00Z',
    updatedAt: '2026-08-03T12:00:00Z',
    metadata: {
      title: 'Live coding',
      description: '',
      category: 'Software and Game Development',
      tags,
      language: 'pl',
      visibility: '',
      matureContent: false,
      dvr: false,
      latencyMode: 'low',
      updatedAt: '2026-08-03T12:00:00Z',
    },
  };
}

describe('building a draft', () => {
  it('copies the stored metadata', () => {
    const draft = toDraft(platform());

    expect(draft.title).toBe('Live coding');
    expect(draft.tags).toEqual(['go', 'react']);
  });

  it('copies the tag array so edits cannot mutate the cache', () => {
    const source = platform();
    const draft = toDraft(source);

    draft.tags.push('mutated');

    expect(source.metadata.tags).toEqual(['go', 'react']);
  });
});

describe('unsaved change detection', () => {
  it('reports a fresh draft as clean', () => {
    const source = platform();

    expect(isDirty(toDraft(source), toDraft(source))).toBe(false);
  });

  it.each([
    ['title', { title: 'Something else' }],
    ['category', { category: 'Just Chatting' }],
    ['language', { language: 'en' }],
    ['latency mode', { latencyMode: 'normal' }],
    ['mature content', { matureContent: true }],
  ])('detects a changed %s', (_field, change) => {
    const stored = toDraft(platform());
    const draft = { ...stored, ...change };

    expect(isDirty(draft, stored)).toBe(true);
  });

  it('detects an added tag', () => {
    const stored = toDraft(platform());
    const draft = { ...stored, tags: [...stored.tags, 'obs'] };

    expect(isDirty(draft, stored)).toBe(true);
  });

  it('detects a removed tag', () => {
    const stored = toDraft(platform());
    const draft = { ...stored, tags: ['go'] };

    expect(isDirty(draft, stored)).toBe(true);
  });

  it('detects reordered tags, because order is persisted and user visible', () => {
    const stored = toDraft(platform());
    const draft = { ...stored, tags: ['react', 'go'] };

    expect(isDirty(draft, stored)).toBe(true);
  });

  it('reports an identical tag list as clean', () => {
    const stored = toDraft(platform());
    const draft = { ...stored, tags: ['go', 'react'] };

    expect(isDirty(draft, stored)).toBe(false);
  });

  it('treats an empty tag list on both sides as clean', () => {
    const stored = toDraft(platform([]));
    const draft = toDraft(platform([]));

    expect(isDirty(draft, stored)).toBe(false);
  });
});
