import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as accountsApi from '@/api/accounts';
import * as operatorChatApi from '@/api/operator-chat';
import { renderWithProviders } from '@/test/render';

import { ChatPage } from './ChatPage';

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  listeners = new Map<string, Set<(event: MessageEvent<string>) => void>>();

  constructor(public url: string) {
    FakeEventSource.instances.push(this);
  }
  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)?.add(listener);
  }
  removeEventListener() {}
  close() {}
  emit(type: string, data: unknown) {
    const event = { data: JSON.stringify(data) } as MessageEvent<string>;
    for (const listener of this.listeners.get(type) ?? []) listener(event);
  }
  emitRaw(type: string) {
    for (const listener of this.listeners.get(type) ?? []) listener({} as MessageEvent<string>);
  }
}

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <ChatPage />
    </MemoryRouter>,
  );
}

vi.mock('@/api/accounts');
vi.mock('@/api/operator-chat');

const twitchAccount = {
  id: 'acct_1',
  providerId: 'twitch',
  login: 'streamer',
  displayName: 'Streamer',
  status: 'connected' as const,
  scopes: [],
  createdAt: '2026-08-06T00:00:00Z',
  updatedAt: '2026-08-06T00:00:00Z',
};

function baseChatItem(overrides: Record<string, unknown> = {}) {
  return {
    version: 1,
    sequence: 1,
    id: 'chat_1',
    providerId: 'twitch',
    connectedAccountId: 'acct_1',
    kind: 'message',
    occurredAt: '2026-08-06T12:00:00Z',
    receivedAt: '2026-08-06T12:00:00Z',
    lifecycle: { deleted: false },
    synthetic: false,
    user: { providerUserId: 'u1', displayName: 'Viewer', anonymous: false },
    message: { plainText: 'hello there', fragments: [{ type: 'text', text: 'hello there' }] },
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  FakeEventSource.instances = [];
  vi.stubGlobal('EventSource', FakeEventSource);

  vi.mocked(accountsApi).fetchAccounts.mockResolvedValue([twitchAccount]);
  vi.mocked(operatorChatApi).fetchOperatorChatStatus.mockResolvedValue({
    schemaVersion: 1,
    bufferCapacity: 500,
    retainedCount: 0,
    oldestSequence: 0,
    newestSequence: 0,
    activeSubscribers: 0,
    busGap: false,
  });
  vi.mocked(operatorChatApi).fetchOperatorChatPreferences.mockResolvedValue({
    showPlatformIcon: true,
    showPlatformName: false,
    showAccountLabel: true,
    showBadges: true,
    showTimestamps: true,
    showActivityEvents: true,
    showDeletedMessages: true,
    hideCommandMessages: false,
    compactMode: false,
  });
  vi.mocked(operatorChatApi).fetchOperatorChatAccountVisibility.mockResolvedValue([]);
  vi.mocked(operatorChatApi).fetchOperatorChatHiddenUsers.mockResolvedValue([]);
  vi.mocked(operatorChatApi).fetchOperatorChatBotUsers.mockResolvedValue([]);
});

describe('ChatPage', () => {
  it('shows the empty state before any message arrives', async () => {
    renderPage();
    expect(await screen.findByText(/no messages yet/i)).toBeInTheDocument();
  });

  it('renders a message received over the stream', async () => {
    renderPage();
    const source = FakeEventSource.instances[0]!;
    source.emit('operator-chat.item', baseChatItem());
    expect(await screen.findByText('hello there')).toBeInTheDocument();
    expect(screen.getByText('Viewer')).toBeInTheDocument();
  });

  it('shows a gap warning after an operator-chat.gap event', async () => {
    renderPage();
    const source = FakeEventSource.instances[0]!;
    source.emitRaw('operator-chat.gap');
    expect(await screen.findByText(/some messages may have been missed/i)).toBeInTheDocument();
  });

  it('opens the settings panel and shows every preference toggle', async () => {
    renderPage();
    const buttons = await screen.findAllByRole('button', { name: 'Display settings' });
    buttons[0]!.click();
    expect(await screen.findByText('Compact mode')).toBeInTheDocument();
    expect(screen.getByText('Badges')).toBeInTheDocument();
  });

  it('opens the filters panel', async () => {
    renderPage();
    const buttons = await screen.findAllByRole('button', { name: 'Filters' });
    buttons[0]!.click();
    expect(await screen.findByText('Hide bot messages')).toBeInTheDocument();
  });

  it('never renders a token or session-id shaped field anywhere on the page', async () => {
    renderPage();
    const source = FakeEventSource.instances[0]!;
    source.emit('operator-chat.item', baseChatItem());
    await screen.findByText('hello there');

    const body = document.body.innerHTML;
    for (const secretShaped of ['accessToken', 'refreshToken', 'sessionId', 'reconnectUrl']) {
      expect(body).not.toContain(secretShaped);
    }
  });

  it('distinguishes an activity item from a message item visually via a different test id', async () => {
    renderPage();
    const source = FakeEventSource.instances[0]!;
    source.emit(
      'operator-chat.item',
      baseChatItem({
        id: 'act_1',
        kind: 'activity',
        message: undefined,
        activity: { activityType: 'follow' },
      }),
    );
    expect(await screen.findByTestId('chat-activity-row')).toBeInTheDocument();
    expect(screen.queryByTestId('chat-message-row')).not.toBeInTheDocument();
  });
});
