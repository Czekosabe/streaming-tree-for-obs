import { describe, expect, it } from 'vitest';

import {
  outboundChatCapabilitySchema,
  outboundChatDispatcherStateSchema,
  outboundChatStatusSchema,
  sendOutboundChatMessageResponseSchema,
} from './outbound-chat-schemas';

describe('outboundChatCapabilitySchema', () => {
  it.each(['unsupported', 'permission_required', 'ready', 'error'])('accepts %s', (value) => {
    expect(outboundChatCapabilitySchema.parse(value)).toBe(value);
  });

  it('rejects an unrecognized value', () => {
    expect(outboundChatCapabilitySchema.safeParse('made_up').success).toBe(false);
  });
});

describe('outboundChatDispatcherStateSchema', () => {
  it.each(['idle', 'queued', 'sending', 'rate_limited', 'stopping', 'error'])('accepts %s', (value) => {
    expect(outboundChatDispatcherStateSchema.parse(value)).toBe(value);
  });
});

describe('outboundChatStatusSchema', () => {
  it('parses a minimal ready status', () => {
    const parsed = outboundChatStatusSchema.parse({
      providerId: 'twitch',
      capability: 'ready',
      dispatcherState: 'idle',
      queueDepth: 0,
      queueCapacity: 20,
      canSendNow: true,
      sharedChatWarning: 'twitch_shared_chat_distribution_possible',
    });
    expect(parsed.capability).toBe('ready');
    expect(parsed.canSendNow).toBe(true);
  });

  it('parses a full permission_required status with scope lists', () => {
    const parsed = outboundChatStatusSchema.parse({
      providerId: 'twitch',
      capability: 'permission_required',
      requiredScopes: ['user:write:chat'],
      grantedScopes: [],
      missingScopes: ['user:write:chat'],
      dispatcherState: 'idle',
      queueDepth: 0,
      queueCapacity: 20,
      canSendNow: false,
      sharedChatWarning: 'twitch_shared_chat_distribution_possible',
    });
    expect(parsed.missingScopes).toEqual(['user:write:chat']);
  });

  it('rejects a status missing required fields', () => {
    expect(outboundChatStatusSchema.safeParse({ providerId: 'twitch' }).success).toBe(false);
  });

  it('never requires a token-shaped field to parse successfully', () => {
    // The schema's own shape has no token/credential field at all - this
    // asserts the parsed keys never include one, even if a hostile server
    // tried to add it (extra keys are simply ignored by Zod's default
    // object parsing, never surfaced).
    const parsed = outboundChatStatusSchema.parse({
      providerId: 'twitch',
      capability: 'ready',
      dispatcherState: 'idle',
      queueDepth: 0,
      queueCapacity: 20,
      canSendNow: true,
      sharedChatWarning: 'x',
      accessToken: 'should-be-ignored',
    });
    expect(Object.keys(parsed)).not.toContain('accessToken');
  });
});

describe('sendOutboundChatMessageResponseSchema', () => {
  it('parses a successful send response', () => {
    const parsed = sendOutboundChatMessageResponseSchema.parse({
      sent: true,
      providerMessageId: 'msg_1',
      sentAt: '2026-08-07T00:00:00Z',
    });
    expect(parsed.sent).toBe(true);
  });

  it('never has a message-text field in its own shape', () => {
    expect(Object.keys(sendOutboundChatMessageResponseSchema.shape)).toEqual([
      'sent',
      'providerMessageId',
      'sentAt',
    ]);
  });
});
