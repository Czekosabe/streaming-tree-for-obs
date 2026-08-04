import { describe, expect, it } from 'vitest';

import {
  configuredPlatformSchema,
  configuredPlatformsResponseSchema,
  platformMetadataSchema,
  providerDefinitionSchema,
  providerDefinitionsResponseSchema,
} from './platform-schemas';

/** A definition shaped exactly like the backend sends one. */
const validDefinition = {
  id: 'twitch',
  brandName: 'Twitch',
  shortLabel: 'TW',
  categoryFieldType: 'category',
  categoryRequiresRemoteId: true,
  capabilities: {
    title: true,
    description: false,
    category: true,
    tags: true,
    language: true,
    visibility: false,
    matureContent: true,
    dvr: false,
    latencyMode: true,
  },
  limits: { titleMaxLength: 140, descriptionMaxLength: 0, maxTags: 10, tagMaxLength: 25 },
  visibilityOptions: [],
  latencyOptions: ['low', 'normal'],
  languageOptions: ['en', 'pl'],
};

const validMetadata = {
  title: 'Live coding',
  description: '',
  category: 'Software and Game Development',
  categoryId: '1469308723',
  tags: ['go', 'react'],
  language: 'pl',
  visibility: '',
  matureContent: false,
  dvr: false,
  latencyMode: 'low',
  updatedAt: '2026-08-03T12:00:00Z',
};

const validPlatform = {
  id: 'pf_seed_twitch',
  providerId: 'twitch',
  displayName: 'Main channel',
  enabled: false,
  sortOrder: 0,
  createdAt: '2026-08-03T12:00:00Z',
  updatedAt: '2026-08-03T12:00:00Z',
  provider: validDefinition,
  metadata: validMetadata,
};

describe('provider definition parsing', () => {
  it('accepts a well formed definition', () => {
    const result = providerDefinitionSchema.safeParse(validDefinition);

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.capabilities.tags).toBe(true);
      expect(result.data.limits.maxTags).toBe(10);
    }
  });

  it('accepts option identifiers this build does not recognise', () => {
    // A newer backend adding an option must not break parsing; unknown values
    // are handled at render time instead.
    const result = providerDefinitionSchema.safeParse({
      ...validDefinition,
      categoryFieldType: 'brand-new-concept',
      latencyOptions: ['low', 'quantum'],
      visibilityOptions: ['members-only'],
    });

    expect(result.success).toBe(true);
  });

  it('rejects a definition missing a capability flag', () => {
    const { tags: _removed, ...incomplete } = validDefinition.capabilities;
    const result = providerDefinitionSchema.safeParse({
      ...validDefinition,
      capabilities: incomplete,
    });

    expect(result.success).toBe(false);
  });

  it('rejects negative limits', () => {
    const result = providerDefinitionSchema.safeParse({
      ...validDefinition,
      limits: { ...validDefinition.limits, maxTags: -1 },
    });

    expect(result.success).toBe(false);
  });

  it('parses the definitions envelope', () => {
    const result = providerDefinitionsResponseSchema.safeParse({
      definitions: [validDefinition],
    });

    expect(result.success).toBe(true);
  });
});

describe('configured platform parsing', () => {
  it('accepts a well formed platform', () => {
    const result = configuredPlatformSchema.safeParse(validPlatform);

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.metadata.tags).toEqual(['go', 'react']);
    }
  });

  it('accepts a platform whose provider is unknown to this build', () => {
    // The backend omits `provider` when it cannot resolve the definition. The
    // card then renders a degraded state rather than crashing.
    const { provider: _omitted, ...withoutProvider } = validPlatform;
    const result = configuredPlatformSchema.safeParse({
      ...withoutProvider,
      providerId: 'some-future-platform',
    });

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.provider).toBeUndefined();
    }
  });

  it('rejects a platform without an id', () => {
    const result = configuredPlatformSchema.safeParse({ ...validPlatform, id: '' });

    expect(result.success).toBe(false);
  });

  it('rejects a platform whose enabled flag is not a boolean', () => {
    const result = configuredPlatformSchema.safeParse({ ...validPlatform, enabled: 'yes' });

    expect(result.success).toBe(false);
  });

  it('parses the platforms envelope', () => {
    const result = configuredPlatformsResponseSchema.safeParse({ platforms: [validPlatform] });

    expect(result.success).toBe(true);
  });
});

describe('metadata parsing', () => {
  it('accepts well formed metadata', () => {
    expect(platformMetadataSchema.safeParse(validMetadata).success).toBe(true);
  });

  it('preserves user authored Unicode exactly', () => {
    const title = 'Zażółć gęślą jaźń — 日本語 ✨';
    const result = platformMetadataSchema.safeParse({ ...validMetadata, title, tags: ['日本語'] });

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.title).toBe(title);
      expect(result.data.tags[0]).toBe('日本語');
    }
  });

  it('rejects tags that are not an array of strings', () => {
    expect(platformMetadataSchema.safeParse({ ...validMetadata, tags: 'go,react' }).success).toBe(
      false,
    );
  });
});
