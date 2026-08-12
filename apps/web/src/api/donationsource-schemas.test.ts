import { describe, expect, it } from 'vitest';

import {
  donationConnectorSchema,
  donationConnectorStateSchema,
  donationSourceProviderSchema,
  donationSourceSchema,
  donationSourcesResponseSchema,
} from './donationsource-schemas';

const SOURCE = {
  id: 'donsrc_1',
  providerId: 'streamelements',
  label: 'Main channel',
  enabled: true,
  remoteChannelId: 'chan_1',
  credentialConfigured: true,
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
};

describe('donationSourceProviderSchema', () => {
  it('accepts streamelements', () => {
    expect(donationSourceProviderSchema.parse('streamelements')).toBe('streamelements');
  });

  it('rejects an unsupported provider', () => {
    expect(donationSourceProviderSchema.safeParse('streamlabs').success).toBe(false);
  });
});

describe('donationConnectorStateSchema', () => {
  it.each([
    'disabled',
    'connecting',
    'connected',
    'reconnecting',
    'possible_gap',
    'reconnect_required',
    'error',
    'stopping',
  ])('accepts %s', (value) => {
    expect(donationConnectorStateSchema.parse(value)).toBe(value);
  });
});

describe('donationSourceSchema', () => {
  it('parses a full source and never expects a credential field', () => {
    const parsed = donationSourceSchema.parse(SOURCE);
    expect(parsed.id).toBe('donsrc_1');
    expect(parsed.credentialConfigured).toBe(true);
    expect('token' in parsed).toBe(false);
  });

  it('rejects a source with an unknown extra field silently stripped rather than accepted as truthy', () => {
    // Zod's default object parsing strips unknown keys rather than
    // rejecting them - this test documents that a stray `token` field
    // in a hypothetical malformed response is discarded, never surfaced.
    const parsed = donationSourceSchema.parse({ ...SOURCE, token: 'super-secret-jwt' });
    expect(JSON.stringify(parsed)).not.toContain('super-secret-jwt');
  });
});

describe('donationSourcesResponseSchema', () => {
  it('parses an items envelope', () => {
    const parsed = donationSourcesResponseSchema.parse({ items: [SOURCE] });
    expect(parsed.items).toHaveLength(1);
  });
});

describe('donationConnectorSchema', () => {
  it('parses a minimal disabled snapshot', () => {
    const parsed = donationConnectorSchema.parse({
      sourceId: 'donsrc_1',
      enabled: false,
      state: 'disabled',
      reconnectCount: 0,
      possibleGapCount: 0,
    });
    expect(parsed.state).toBe('disabled');
  });

  it('never carries a token, reconnect token, or raw provider payload field', () => {
    const parsed = donationConnectorSchema.parse({
      sourceId: 'donsrc_1',
      enabled: true,
      state: 'connected',
      reconnectCount: 2,
      possibleGapCount: 0,
      lastError: 'streamelements_auth_failed',
    });
    const rendered = JSON.stringify(parsed);
    expect(rendered).not.toMatch(/token/i);
    expect(rendered).not.toMatch(/jwt/i);
  });
});
