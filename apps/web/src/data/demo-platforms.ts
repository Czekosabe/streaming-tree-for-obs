/**
 * DEMO DATA - not backed by any real platform API.
 *
 * Every value in this file is hand-written. Capability tables reflect a rough,
 * approximate reading of what each platform offers and exist to exercise the
 * capability-driven metadata editor. They are NOT an authoritative description
 * of any platform's API and will be replaced once real integrations land.
 */

import type {
  PlatformDefinition,
  PlatformId,
  PlatformMetadata,
  PlatformRuntimeState,
  SelectOption,
  StreamPlatform,
} from '@/models/platform';

/**
 * Stream languages, written as endonyms exactly like the platforms themselves
 * present them. These are proper names, so they stay identical in every UI
 * language and are not part of the translation resources.
 */
const COMMON_LANGUAGES: readonly SelectOption[] = [
  { value: 'en', label: 'English' },
  { value: 'pl', label: 'Polski' },
  { value: 'de', label: 'Deutsch' },
  { value: 'es', label: 'Espanol' },
  { value: 'fr', label: 'Francais' },
];

/**
 * Static platform definitions.
 *
 * Note the deliberate differences: Twitch is the only platform with tag support
 * enabled in this demo configuration, YouTube is the only one exposing DVR, and
 * TikTok has the smallest capability surface.
 */
export const DEMO_PLATFORM_DEFINITIONS: readonly PlatformDefinition[] = [
  {
    id: 'twitch',
    name: 'Twitch',
    shortLabel: 'TW',
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
    limits: {
      titleMaxLength: 140,
      descriptionMaxLength: 0,
      maxTags: 10,
      tagMaxLength: 25,
    },
    options: {
      categoryLabelKey: 'fields.category',
      categoryPlaceholderKey: 'categoryPlaceholder.twitch',
      visibility: [],
      latencyModes: [
        { value: 'low', labelKey: 'latency.low' },
        { value: 'normal', labelKey: 'latency.normal' },
      ],
      languages: COMMON_LANGUAGES,
    },
  },
  {
    id: 'youtube',
    name: 'YouTube Live',
    shortLabel: 'YT',
    capabilities: {
      title: true,
      description: true,
      category: true,
      tags: false,
      language: true,
      visibility: true,
      matureContent: true,
      dvr: true,
      latencyMode: true,
    },
    limits: {
      titleMaxLength: 100,
      descriptionMaxLength: 5000,
      maxTags: 0,
      tagMaxLength: 0,
    },
    options: {
      categoryLabelKey: 'fields.category',
      categoryPlaceholderKey: 'categoryPlaceholder.youtube',
      visibility: [
        { value: 'public', labelKey: 'visibility.public' },
        { value: 'unlisted', labelKey: 'visibility.unlisted' },
        { value: 'private', labelKey: 'visibility.private' },
      ],
      latencyModes: [
        { value: 'normal', labelKey: 'latency.normal' },
        { value: 'low', labelKey: 'latency.low' },
        { value: 'ultra-low', labelKey: 'latency.ultraLow' },
      ],
      languages: COMMON_LANGUAGES,
    },
  },
  {
    id: 'kick',
    name: 'Kick',
    shortLabel: 'KI',
    capabilities: {
      title: true,
      description: false,
      category: true,
      tags: false,
      language: true,
      visibility: false,
      matureContent: true,
      dvr: false,
      latencyMode: false,
    },
    limits: {
      titleMaxLength: 100,
      descriptionMaxLength: 0,
      maxTags: 0,
      tagMaxLength: 0,
    },
    options: {
      categoryLabelKey: 'fields.category',
      categoryPlaceholderKey: 'categoryPlaceholder.kick',
      visibility: [],
      latencyModes: [],
      languages: COMMON_LANGUAGES,
    },
  },
  {
    id: 'tiktok',
    name: 'TikTok Live',
    shortLabel: 'TT',
    capabilities: {
      title: true,
      description: false,
      category: true,
      tags: false,
      language: false,
      visibility: false,
      matureContent: false,
      dvr: false,
      latencyMode: false,
    },
    limits: {
      titleMaxLength: 60,
      descriptionMaxLength: 0,
      maxTags: 0,
      tagMaxLength: 0,
    },
    options: {
      categoryLabelKey: 'fields.topic',
      categoryPlaceholderKey: 'categoryPlaceholder.tiktok',
      visibility: [],
      latencyModes: [],
      languages: [],
    },
  },
];

function emptyMetadata(): PlatformMetadata {
  return {
    title: '',
    description: '',
    category: '',
    tags: [],
    language: 'en',
    visibility: 'public',
    matureContent: false,
    dvr: false,
    latencyMode: 'normal',
  };
}

/** DEMO runtime state, one entry per platform. */
const DEMO_RUNTIME: Record<PlatformId, PlatformRuntimeState> = {
  twitch: {
    status: 'live',
    viewers: 1284,
    quality: 'excellent',
    statusDetailKey: null,
    metadata: {
      ...emptyMetadata(),
      title: 'Building Streaming Tree for OBS - foundations',
      category: 'Software and Game Development',
      tags: ['programming', 'go', 'react', 'obs'],
      language: 'pl',
      latencyMode: 'low',
    },
  },
  youtube: {
    status: 'starting',
    viewers: null,
    quality: 'good',
    statusDetailKey: null,
    metadata: {
      ...emptyMetadata(),
      title: 'Building Streaming Tree for OBS - foundations',
      description:
        'Live coding session. We are building a local multistreaming control panel for OBS.',
      category: 'Science & Technology',
      language: 'pl',
      visibility: 'public',
      dvr: true,
      latencyMode: 'low',
    },
  },
  kick: {
    status: 'offline',
    viewers: null,
    quality: 'unknown',
    statusDetailKey: null,
    metadata: {
      ...emptyMetadata(),
      title: 'Building Streaming Tree for OBS',
      category: 'Just Chatting',
      language: 'pl',
    },
  },
  tiktok: {
    status: 'error',
    viewers: null,
    quality: 'poor',
    statusDetailKey: 'statusDetail.noStreamKey',
    metadata: {
      ...emptyMetadata(),
      title: 'Building a multistream tool',
      category: 'Gaming',
    },
  },
};

/** Builds the initial DEMO platform list consumed by the dashboard. */
export function createDemoPlatforms(): StreamPlatform[] {
  return DEMO_PLATFORM_DEFINITIONS.map((definition) => ({
    ...definition,
    ...DEMO_RUNTIME[definition.id],
    metadata: { ...DEMO_RUNTIME[definition.id].metadata },
  }));
}
