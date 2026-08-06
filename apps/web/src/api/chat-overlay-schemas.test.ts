import { describe, expect, it } from 'vitest';

import {
  chatOverlayRemoveReasonSchema,
  isCosmeticRemoveReason,
  publicChatOverlayRemovePayloadSchema,
} from './chat-overlay-schemas';

describe('chatOverlayRemoveReasonSchema', () => {
  it.each(['expired', 'capacity_evicted', 'message_deleted', 'chat_cleared', 'user_messages_cleared', 'unknown'])(
    'parses the known reason "%s" unchanged',
    (reason) => {
      expect(chatOverlayRemoveReasonSchema.parse(reason)).toBe(reason);
    },
  );

  it('falls back to "unknown" for an unrecognized string, never throwing', () => {
    expect(chatOverlayRemoveReasonSchema.parse('some_new_backend_reason')).toBe('unknown');
  });

  it('falls back to "unknown" for a non-string value', () => {
    expect(chatOverlayRemoveReasonSchema.parse(42)).toBe('unknown');
    expect(chatOverlayRemoveReasonSchema.parse(null)).toBe('unknown');
    expect(chatOverlayRemoveReasonSchema.parse(undefined)).toBe('unknown');
  });
});

describe('isCosmeticRemoveReason', () => {
  it('is true only for expired and capacity_evicted', () => {
    expect(isCosmeticRemoveReason('expired')).toBe(true);
    expect(isCosmeticRemoveReason('capacity_evicted')).toBe(true);
  });

  it('is false for every immediate reason', () => {
    expect(isCosmeticRemoveReason('message_deleted')).toBe(false);
    expect(isCosmeticRemoveReason('chat_cleared')).toBe(false);
    expect(isCosmeticRemoveReason('user_messages_cleared')).toBe(false);
    expect(isCosmeticRemoveReason('unknown')).toBe(false);
  });
});

describe('publicChatOverlayRemovePayloadSchema', () => {
  it('parses a well-formed payload', () => {
    const parsed = publicChatOverlayRemovePayloadSchema.parse({ id: 'chat_1', reason: 'expired' });
    expect(parsed).toEqual({ id: 'chat_1', reason: 'expired' });
  });

  it('defaults a missing reason to "unknown" rather than failing the whole payload', () => {
    const parsed = publicChatOverlayRemovePayloadSchema.parse({ id: 'chat_1' });
    expect(parsed.reason).toBe('unknown');
  });

  it('rejects a payload with no id', () => {
    expect(publicChatOverlayRemovePayloadSchema.safeParse({ reason: 'expired' }).success).toBe(false);
  });

  it('never carries a message/text-shaped field - the schema simply has none', () => {
    const shape = Object.keys(publicChatOverlayRemovePayloadSchema.shape);
    expect(shape).toEqual(['id', 'reason']);
  });
});
