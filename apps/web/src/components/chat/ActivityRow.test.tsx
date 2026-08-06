import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { OperatorChatItem } from '@/api/operator-chat-schemas';
import { DEFAULT_OPERATOR_CHAT_PREFERENCES } from '@/api/operator-chat-schemas';
import { renderWithProviders } from '@/test/render';

import { ActivityRow } from './ActivityRow';

function activityItem(activityType: string, overrides: Partial<OperatorChatItem> = {}): OperatorChatItem {
  return {
    version: 1,
    sequence: 1,
    id: `act_${activityType}`,
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    kind: 'activity',
    occurredAt: '2026-08-06T12:00:00Z',
    receivedAt: '2026-08-06T12:00:00Z',
    lifecycle: { deleted: false },
    synthetic: false,
    user: { providerUserId: 'u1', displayName: 'Viewer', anonymous: false },
    activity: { activityType },
    ...overrides,
  };
}

function renderRow(item: OperatorChatItem) {
  return renderWithProviders(
    <ul>
      <ActivityRow item={item} preferences={DEFAULT_OPERATOR_CHAT_PREFERENCES} accountLabel={null} />
    </ul>,
  );
}

describe('ActivityRow', () => {
  it.each([
    ['follow', 'Follow'],
    ['subscription', 'Subscription'],
    ['resubscription', 'Resubscription'],
    ['gifted_subscription', 'Gifted subscription'],
    ['subscription_gift_batch', 'Gift batch'],
    ['bits', 'Bits'],
    ['raid', 'Raid'],
    ['channel_point_redemption', 'Channel points'],
    ['stream.online', 'Stream online (remote)'],
    ['stream.offline', 'Stream offline (remote)'],
  ])('renders the correct label for %s', (activityType, label) => {
    renderRow(activityItem(activityType));
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it('never labels bits as a donation', () => {
    renderRow(activityItem('bits', { activity: { activityType: 'bits', quantity: 500 } }));
    expect(screen.getByText('Bits')).toBeInTheDocument();
    expect(screen.queryByText(/donation/i)).not.toBeInTheDocument();
  });

  it('shows the quantity when present', () => {
    renderRow(activityItem('raid', { activity: { activityType: 'raid', quantity: 42 } }));
    expect(screen.getByText('42')).toBeInTheDocument();
  });

  it('does not fabricate a "stream is live" claim for a remote stream.online notice', () => {
    renderRow(activityItem('stream.online', { user: undefined }));
    expect(screen.getByText('Stream online (remote)')).toBeInTheDocument();
  });

  it('renders an anonymous actor without a fabricated identity', () => {
    renderRow(activityItem('bits', { user: { anonymous: true } }));
    expect(screen.getByText(/anonymous/i)).toBeInTheDocument();
  });
});
