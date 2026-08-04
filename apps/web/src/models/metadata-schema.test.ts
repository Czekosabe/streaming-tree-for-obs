import { describe, expect, it } from 'vitest';

import type { ProviderCapabilities, ProviderLimits, SaveMetadataInput } from '@/api/platform-schemas';

import {
  pickSupportedFields,
  validateMetadata,
  type MetadataValidationContext,
  type MetadataValidationMessages,
} from './metadata-schema';

/**
 * Messages are English here only so assertions are readable; production passes
 * translated strings in. What is under test is which rule fires, not wording.
 */
const messages: MetadataValidationMessages = {
  titleRequired: 'title required',
  titleMaxLength: (max) => `title max ${max}`,
  descriptionMaxLength: (max) => `description max ${max}`,
  categoryRequired: (field) => `${field} required`,
  categoryMaxLength: (field, max) => `${field} max ${max}`,
  tagMinLength: (min) => `tag min ${min}`,
  tagMaxLength: (max) => `tag max ${max}`,
  tagPattern: 'tag pattern',
  tagsMaxCount: (max) => `tags max ${max}`,
  tagsUnique: 'tags unique',
  languageUnsupported: 'language unsupported',
  visibilityUnsupported: 'visibility unsupported',
  latencyModeUnsupported: 'latency unsupported',
};

const twitchCapabilities: ProviderCapabilities = {
  title: true,
  description: false,
  category: true,
  tags: true,
  language: true,
  visibility: false,
  matureContent: true,
  dvr: false,
  latencyMode: true,
};

const twitchLimits: ProviderLimits = {
  titleMaxLength: 140,
  descriptionMaxLength: 0,
  maxTags: 10,
  tagMaxLength: 25,
};

const twitchContext: MetadataValidationContext = {
  capabilities: twitchCapabilities,
  limits: twitchLimits,
  categoryLabel: 'Category',
  visibilityOptions: [],
  latencyOptions: ['low', 'normal'],
  languageOptions: ['en', 'pl'],
};

function draft(overrides: Partial<SaveMetadataInput> = {}): SaveMetadataInput {
  return {
    title: 'Building a multistream tool',
    description: '',
    category: 'Software and Game Development',
    categoryId: '1469308723',
    tags: [],
    language: 'pl',
    visibility: '',
    matureContent: false,
    dvr: false,
    latencyMode: 'low',
    ...overrides,
  };
}

describe('capability-driven field selection', () => {
  it('only submits fields the provider supports', () => {
    const picked = pickSupportedFields(twitchCapabilities, draft({ description: 'ignored' }));

    expect(Object.keys(picked).sort()).toEqual(
      ['category', 'latencyMode', 'language', 'matureContent', 'tags', 'title'].sort(),
    );
    // Twitch has no description or DVR, so neither is ever sent.
    expect(picked).not.toHaveProperty('description');
    expect(picked).not.toHaveProperty('dvr');
  });
});

describe('metadata validation', () => {
  it('accepts a valid Twitch document', () => {
    const result = validateMetadata(twitchContext, draft({ tags: ['go', 'react'] }), messages);

    expect(result.success).toBe(true);
    expect(result.errors).toEqual({});
  });

  it('requires a title', () => {
    const result = validateMetadata(twitchContext, draft({ title: '   ' }), messages);

    expect(result.success).toBe(false);
    expect(result.errors.title).toBe('title required');
  });

  it('rejects an oversized title', () => {
    const result = validateMetadata(twitchContext, draft({ title: 'x'.repeat(141) }), messages);

    expect(result.success).toBe(false);
    expect(result.errors.title).toBe('title max 140');
  });

  it('rejects duplicate tags differing only by case', () => {
    const result = validateMetadata(
      twitchContext,
      draft({ tags: ['Programming', 'programming'] }),
      messages,
    );

    expect(result.success).toBe(false);
    expect(result.errors.tags).toBe('tags unique');
  });

  it('rejects more tags than the provider allows', () => {
    const tags = Array.from({ length: 11 }, (_value, index) => `tag${index}`);
    const result = validateMetadata(twitchContext, draft({ tags }), messages);

    expect(result.success).toBe(false);
    expect(result.errors.tags).toBe('tags max 10');
  });

  it('rejects an oversized tag', () => {
    const result = validateMetadata(twitchContext, draft({ tags: ['x'.repeat(26)] }), messages);

    expect(result.success).toBe(false);
    expect(result.errors.tags).toBe('tag max 25');
  });

  it('rejects a tag with forbidden punctuation', () => {
    const result = validateMetadata(twitchContext, draft({ tags: ['bad!tag'] }), messages);

    expect(result.success).toBe(false);
    expect(result.errors.tags).toBe('tag pattern');
  });

  it('accepts Unicode tags from any script', () => {
    const result = validateMetadata(twitchContext, draft({ tags: ['zażółć', '日本語'] }), messages);

    expect(result.success).toBe(true);
  });

  it('rejects a latency mode the provider does not offer', () => {
    // "ultra-low" is a YouTube option, not a Twitch one.
    const result = validateMetadata(twitchContext, draft({ latencyMode: 'ultra-low' }), messages);

    expect(result.success).toBe(false);
    expect(result.errors.latencyMode).toBe('latency unsupported');
  });

  it('rejects an unsupported language', () => {
    const result = validateMetadata(twitchContext, draft({ language: 'kl' }), messages);

    expect(result.success).toBe(false);
    expect(result.errors.language).toBe('language unsupported');
  });

  it('never validates tags for a provider without tag support', () => {
    const withoutTags: MetadataValidationContext = {
      ...twitchContext,
      capabilities: { ...twitchCapabilities, tags: false },
    };

    // The tag rules simply do not exist for this provider, so a value that
    // would be rejected for Twitch is not even inspected here.
    const result = validateMetadata(withoutTags, draft({ tags: ['bad!tag'] }), messages);

    expect(result.success).toBe(true);
  });

  it('reports one message per field', () => {
    const result = validateMetadata(
      twitchContext,
      draft({ title: '', category: '' }),
      messages,
    );

    expect(result.success).toBe(false);
    expect(result.errors.title).toBe('title required');
    expect(result.errors.category).toBe('Category required');
  });
});
