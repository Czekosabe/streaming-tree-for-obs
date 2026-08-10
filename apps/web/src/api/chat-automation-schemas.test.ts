import { describe, expect, it } from 'vitest';

import {
  chatAutomationStatusSchema,
  commandRoleSchema,
  commandSchema,
  previewResponseSchema,
  scheduleSchema,
  scheduleStateSchema,
  sendNowResponseSchema,
} from './chat-automation-schemas';

describe('scheduleStateSchema', () => {
  it.each([
    'disabled',
    'scheduled',
    'waiting_for_stream',
    'waiting_for_activity',
    'rate_limited',
    'permission_required',
    'sending',
    'error',
  ])('accepts %s', (value) => {
    expect(scheduleStateSchema.parse(value)).toBe(value);
  });

  it('rejects an unrecognized value', () => {
    expect(scheduleStateSchema.safeParse('made_up').success).toBe(false);
  });
});

describe('commandRoleSchema', () => {
  it.each(['everyone', 'subscriber', 'vip', 'moderator', 'broadcaster'])('accepts %s', (value) => {
    expect(commandRoleSchema.parse(value)).toBe(value);
  });

  it('rejects follower - deliberately not a supported role', () => {
    expect(commandRoleSchema.safeParse('follower').success).toBe(false);
  });
});

describe('scheduleSchema', () => {
  it('parses a full schedule with targets, messages and runtime status', () => {
    const parsed = scheduleSchema.parse({
      id: 'sched_1', name: 'Reminder', enabled: true,
      intervalSeconds: 3600, firstDelaySeconds: 0, jitterSeconds: 0,
      onlyWhileIngestReceiving: false, minimumChatMessages: 0, maximumSendsPerHour: 10,
      targets: [{ accountId: 'acct_1' }],
      messages: [{ id: 'schedmsg_1', template: 'hello' }],
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
      state: 'scheduled', nextRunAt: '2026-01-01T01:00:00Z',
    });
    expect(parsed.state).toBe('scheduled');
    expect(parsed.targets).toHaveLength(1);
  });

  it('rejects a schedule missing required fields', () => {
    expect(scheduleSchema.safeParse({ id: 'sched_1' }).success).toBe(false);
  });
});

describe('commandSchema', () => {
  it('parses a full command', () => {
    const parsed = commandSchema.parse({
      id: 'cmd_1', name: 'discord', enabled: true, responseTemplate: 'join us',
      requiredRole: 'everyone', globalCooldownSeconds: 0, userCooldownSeconds: 0,
      aliases: ['disc'], targets: [{ accountId: 'acct_1' }],
      createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
      matchCount: 5, responseCount: 3,
    });
    expect(parsed.aliases).toEqual(['disc']);
  });
});

describe('chatAutomationStatusSchema', () => {
  it('parses an empty aggregate status', () => {
    const parsed = chatAutomationStatusSchema.parse({
      engine: {
        running: true, subscribedToBus: true, commandCount: 0, enabledCommandCount: 0,
        totalMatched: 0, totalResponses: 0, totalCooldownSkips: 0, totalRoleSkips: 0, totalSelfSkips: 0,
      },
      schedules: [], commands: [],
    });
    expect(parsed.engine.running).toBe(true);
  });
});

describe('sendNowResponseSchema', () => {
  it('parses a mixed sent/skipped result set', () => {
    const parsed = sendNowResponseSchema.parse({
      results: [
        { accountId: 'acct_1', sent: true, providerMessageId: 'm1' },
        { accountId: 'acct_2', sent: false, skipReason: 'waiting_for_stream' },
      ],
    });
    expect(parsed.results).toHaveLength(2);
    expect(parsed.results[1]?.skipReason).toBe('waiting_for_stream');
  });
});

describe('previewResponseSchema', () => {
  it('parses a response with unresolved placeholders', () => {
    const parsed = previewResponseSchema.parse({
      renderedText: 'Now playing: ', codePointCount: 13,
      unresolvedPlaceholders: ['streamTitle'], validForProvider: false,
    });
    expect(parsed.unresolvedPlaceholders).toEqual(['streamTitle']);
    expect(parsed.validForProvider).toBe(false);
  });
});
