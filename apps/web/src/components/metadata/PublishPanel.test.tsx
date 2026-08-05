import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import type { PlatformAccountLink, PublishPreview, PublishResult } from '@/api/account-schemas';
import * as accountsApi from '@/api/accounts';
import { renderWithProviders } from '@/test/render';

import { PublishPanel } from './PublishPanel';

vi.mock('@/api/accounts');

const accounts = vi.mocked(accountsApi);

const PLATFORM: ConfiguredPlatform = {
  id: 'pf_1',
  providerId: 'twitch',
  displayName: 'Twitch',
  enabled: true,
  sortOrder: 0,
  createdAt: '2026-08-01T00:00:00Z',
  updatedAt: '2026-08-01T00:00:00Z',
  provider: {
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
      matureContent: false,
      dvr: false,
      latencyMode: false,
    },
    limits: { titleMaxLength: 140, descriptionMaxLength: 0, maxTags: 10, tagMaxLength: 25 },
    visibilityOptions: [],
    latencyOptions: [],
    languageOptions: ['en'],
  },
  metadata: {
    title: 'Live coding',
    description: '',
    category: 'Software and Game Development',
    categoryId: '1469308723',
    tags: [],
    language: 'en',
    visibility: 'public',
    matureContent: false,
    dvr: true,
    latencyMode: 'normal',
    updatedAt: '2026-08-01T00:00:00Z',
  },
};

const LINK: PlatformAccountLink = {
  platformId: 'pf_1',
  accountId: 'acct_1',
  createdAt: '2026-08-01T00:00:00Z',
  updatedAt: '2026-08-01T00:00:00Z',
};

const PREVIEW_WITH_CHANGE: PublishPreview = {
  providerId: 'twitch',
  accountId: 'acct_1',
  accountLogin: 'streamer',
  fields: [{ field: 'title', local: 'Live coding', remote: 'Old title', changed: true }],
  skipped: ['description'],
  blockers: [],
  allowed: true,
};

// See TwitchDeviceFlowModal.test.tsx: restoreMocks does not clear call
// history on this automocked module between tests in the same file.
beforeEach(() => {
  vi.clearAllMocks();
});

describe('PublishPanel', () => {
  it('blocks publishing and explains why when the local form has unsaved edits', async () => {
    accounts.fetchPlatformAccountLink.mockResolvedValue(LINK);

    renderWithProviders(<PublishPanel platform={PLATFORM} dirty />);

    expect(await screen.findByText(/save your local changes before publishing/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /publish to twitch/i })).not.toBeInTheDocument();
    expect(accounts.fetchPublishPreview).not.toHaveBeenCalled();
  });

  it('requires an application-styled confirmation before publishing, showing the fields that will change', async () => {
    const user = userEvent.setup();
    accounts.fetchPlatformAccountLink.mockResolvedValue(LINK);
    accounts.fetchPublishPreview.mockResolvedValue(PREVIEW_WITH_CHANGE);
    const result: PublishResult = {
      status: 'published',
      accountId: 'acct_1',
      publishedAt: '2026-08-04T12:00:00Z',
      fieldsChanged: ['title'],
      fieldsSkipped: ['description'],
    };
    accounts.publishMetadata.mockResolvedValue(result);

    renderWithProviders(<PublishPanel platform={PLATFORM} dirty={false} />);

    await screen.findByText('title');
    expect(accounts.publishMetadata).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: /publish to twitch/i }));

    const dialog = await screen.findByRole('dialog', { name: /publish to twitch\?/i });
    expect(accounts.publishMetadata).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: /^publish$/i }));

    await waitFor(() => expect(accounts.publishMetadata.mock.calls[0]?.[0]).toBe('pf_1'));
    expect(await screen.findByText('Published to Twitch.')).toBeInTheDocument();
  });

  it('explains that linking is required when the destination has no linked account', async () => {
    accounts.fetchPlatformAccountLink.mockResolvedValue(null);

    renderWithProviders(<PublishPanel platform={PLATFORM} dirty={false} />);

    expect(await screen.findByText(/no twitch account linked to this destination/i)).toBeInTheDocument();
    expect(accounts.fetchPublishPreview).not.toHaveBeenCalled();
  });
});
