import { describe, expect, it } from 'vitest';

import {
  broadcastListResponseSchema,
  connectedAccountSchema,
  deviceFlowSnapshotSchema,
  deviceFlowStateSchema,
  integrationConfigSchema,
  oauthAttemptSnapshotSchema,
  oauthAttemptStateSchema,
  platformAccountLinkResponseSchema,
  publishPreviewSchema,
  publishResultSchema,
  remoteTargetResponseSchema,
} from './account-schemas';

describe('integrationConfigSchema', () => {
  it('accepts a database-managed configuration', () => {
    const result = integrationConfigSchema.safeParse({
      configured: true,
      source: 'database',
      clientId: 'abc123',
    });
    expect(result.success).toBe(true);
  });

  it('accepts an environment-managed configuration without a clientId echoed', () => {
    const result = integrationConfigSchema.safeParse({ configured: true, source: 'environment' });
    expect(result.success).toBe(true);
  });

  it('rejects an unrecognised source', () => {
    const result = integrationConfigSchema.safeParse({ configured: true, source: 'somewhere-else' });
    expect(result.success).toBe(false);
  });
});

describe('deviceFlowStateSchema', () => {
  it.each([
    'requesting_code',
    'waiting_for_user',
    'polling',
    'authorized',
    'denied',
    'expired',
    'cancelled',
    'error',
  ])('accepts %s', (state) => {
    expect(deviceFlowStateSchema.safeParse(state).success).toBe(true);
  });

  it('rejects an unrecognised state', () => {
    expect(deviceFlowStateSchema.safeParse('mystery').success).toBe(false);
  });
});

describe('deviceFlowSnapshotSchema', () => {
  it('accepts a waiting-for-user snapshot', () => {
    const result = deviceFlowSnapshotSchema.safeParse({
      attemptId: 'devflow_abc',
      providerId: 'twitch',
      state: 'waiting_for_user',
      userCode: 'ABCD-EFGH',
      verificationUri: 'https://www.twitch.tv/activate',
      createdAt: '2026-08-04T12:00:00Z',
      expiresAt: '2026-08-04T12:30:00Z',
      intervalSeconds: 5,
    });
    expect(result.success).toBe(true);
  });

  it('has no field for a device code, however it is spelled', () => {
    const parsed = deviceFlowSnapshotSchema.parse({
      attemptId: 'devflow_abc',
      providerId: 'twitch',
      state: 'authorized',
      createdAt: '2026-08-04T12:00:00Z',
    });
    expect(parsed).not.toHaveProperty('deviceCode');
    expect(parsed).not.toHaveProperty('device_code');
  });

  it('rejects a payload with no attemptId', () => {
    const result = deviceFlowSnapshotSchema.safeParse({
      providerId: 'twitch',
      state: 'polling',
      createdAt: '2026-08-04T12:00:00Z',
    });
    expect(result.success).toBe(false);
  });
});

describe('connectedAccountSchema', () => {
  it('accepts a real-shaped account and carries no token field', () => {
    const parsed = connectedAccountSchema.parse({
      id: 'acct_1',
      providerId: 'twitch',
      login: 'streamer',
      displayName: 'Streamer',
      status: 'connected',
      scopes: ['channel:manage:broadcast'],
      createdAt: '2026-08-04T12:00:00Z',
      updatedAt: '2026-08-04T12:00:00Z',
    });
    expect(parsed).not.toHaveProperty('accessToken');
    expect(parsed).not.toHaveProperty('refreshToken');
  });

  it('rejects an unrecognised status', () => {
    const result = connectedAccountSchema.safeParse({
      id: 'acct_1',
      providerId: 'twitch',
      login: 'streamer',
      displayName: 'Streamer',
      status: 'super-connected',
      scopes: [],
      createdAt: '2026-08-04T12:00:00Z',
      updatedAt: '2026-08-04T12:00:00Z',
    });
    expect(result.success).toBe(false);
  });
});

describe('platformAccountLinkResponseSchema', () => {
  it('accepts null for an unlinked platform', () => {
    expect(platformAccountLinkResponseSchema.safeParse(null).success).toBe(true);
  });

  it('accepts a real link', () => {
    const result = platformAccountLinkResponseSchema.safeParse({
      platformId: 'pf_1',
      accountId: 'acct_1',
      createdAt: '2026-08-04T12:00:00Z',
      updatedAt: '2026-08-04T12:00:00Z',
    });
    expect(result.success).toBe(true);
  });
});

describe('publishPreviewSchema', () => {
  it('accepts a blocked preview with no account resolved', () => {
    const result = publishPreviewSchema.safeParse({
      providerId: 'twitch',
      fields: [],
      skipped: ['description'],
      blockers: ['account_not_linked'],
      allowed: false,
    });
    expect(result.success).toBe(true);
  });

  it('accepts an allowed preview with field diffs', () => {
    const result = publishPreviewSchema.safeParse({
      providerId: 'twitch',
      accountId: 'acct_1',
      accountLogin: 'streamer',
      fields: [{ field: 'title', local: 'New title', remote: 'Old title', changed: true }],
      skipped: [],
      blockers: [],
      allowed: true,
    });
    expect(result.success).toBe(true);
  });
});

describe('publishResultSchema', () => {
  it('accepts a published result', () => {
    const result = publishResultSchema.safeParse({
      status: 'published',
      accountId: 'acct_1',
      publishedAt: '2026-08-04T12:00:00Z',
      fieldsChanged: ['title'],
      fieldsSkipped: ['description'],
    });
    expect(result.success).toBe(true);
  });

  it('accepts a blocked result', () => {
    const result = publishResultSchema.safeParse({
      status: 'blocked',
      blockers: ['category_not_selected'],
    });
    expect(result.success).toBe(true);
  });

  it('accepts a YouTube result carrying a broadcastId, warnings and fieldsFailed', () => {
    const result = publishResultSchema.safeParse({
      status: 'published',
      accountId: 'acct_yt_1',
      broadcastId: 'bcast1',
      publishedAt: '2026-08-05T12:00:00Z',
      fieldsChanged: ['title'],
      fieldsSkipped: ['matureContent'],
      fieldsFailed: [],
      warnings: ['testing_mode_seven_day_token'],
    });
    expect(result.success).toBe(true);
  });
});

describe('oauthAttemptStateSchema', () => {
  it('accepts every documented YouTube OAuth attempt state', () => {
    for (const state of [
      'creating',
      'waiting_for_browser',
      'processing_callback',
      'awaiting_channel_selection',
      'authorized',
      'denied',
      'expired',
      'cancelled',
      'error',
    ]) {
      expect(oauthAttemptStateSchema.safeParse(state).success).toBe(true);
    }
  });

  it('rejects an unrecognized state', () => {
    expect(oauthAttemptStateSchema.safeParse('polling').success).toBe(false);
  });
});

describe('oauthAttemptSnapshotSchema', () => {
  it('accepts a waiting-for-browser attempt with an authorization URL', () => {
    const result = oauthAttemptSnapshotSchema.safeParse({
      attemptId: 'ytauth_1',
      providerId: 'youtube',
      state: 'waiting_for_browser',
      authorizationUrl: 'https://accounts.google.com/o/oauth2/v2/auth?client_id=cid',
      createdAt: '2026-08-05T12:00:00Z',
      expiresAt: '2026-08-05T12:15:00Z',
    });
    expect(result.success).toBe(true);
  });

  it('accepts a channel-selection attempt with safe channel summaries only', () => {
    const result = oauthAttemptSnapshotSchema.safeParse({
      attemptId: 'ytauth_1',
      providerId: 'youtube',
      state: 'awaiting_channel_selection',
      createdAt: '2026-08-05T12:00:00Z',
      channels: [{ channelId: 'UC1', title: 'First', thumbnailUrl: 'https://example.com/a.jpg' }],
    });
    expect(result.success).toBe(true);
  });

  it('never carries a device code, authorization code, PKCE verifier, or OAuth CSRF state field - unknown fields fail strict parsing intent', () => {
    // Zod's default .parse (not .strict()) tolerates unknown fields, so this
    // asserts the schema's own declared keys instead, matching this
    // project's "absence is structural" convention for every other account
    // schema in this file. "state" itself IS a legitimate field here (the
    // attempt's own lifecycle state, e.g. "waiting_for_browser") - it is the
    // OAuth CSRF state value that must never appear as a separate field.
    const shape = Object.keys(oauthAttemptSnapshotSchema.shape);
    for (const forbidden of [
      'code',
      'authorizationCode',
      'oauthState',
      'csrfState',
      'pkceVerifier',
      'codeVerifier',
      'deviceCode',
      'accessToken',
      'refreshToken',
    ]) {
      expect(shape).not.toContain(forbidden);
    }
  });
});

describe('broadcastListResponseSchema', () => {
  it('accepts a normalized broadcast list with no ingestion fields', () => {
    const result = broadcastListResponseSchema.safeParse({
      items: [
        {
          id: 'bcast1',
          title: 'Live now',
          lifeCycleStatus: 'live',
          privacyStatus: 'public',
          actualStartTime: '2026-08-05T12:00:00Z',
        },
      ],
    });
    expect(result.success).toBe(true);
  });

  it('never declares a stream key or ingestion field', () => {
    const itemShape = Object.keys(broadcastListResponseSchema.shape.items.element.shape);
    for (const forbidden of ['streamKey', 'streamName', 'ingestionAddress', 'boundStreamId']) {
      expect(itemShape).not.toContain(forbidden);
    }
  });
});

describe('remoteTargetResponseSchema', () => {
  it('accepts null when no target is set', () => {
    expect(remoteTargetResponseSchema.safeParse(null).success).toBe(true);
  });

  it('accepts a set remote target', () => {
    const result = remoteTargetResponseSchema.safeParse({
      platformId: 'pf_1',
      providerId: 'youtube',
      resourceType: 'live_broadcast',
      resourceId: 'bcast1',
      displayName: 'My Stream',
      createdAt: '2026-08-05T12:00:00Z',
      updatedAt: '2026-08-05T12:00:00Z',
    });
    expect(result.success).toBe(true);
  });
});
