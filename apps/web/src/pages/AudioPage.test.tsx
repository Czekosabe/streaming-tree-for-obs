import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as audioApi from '@/api/audio';
import type { AudioPendingItem, AudioSettings, AudioStatus } from '@/api/audio-schemas';
import { renderWithProviders } from '@/test/render';

import { AudioPage } from './AudioPage';

vi.mock('@/api/audio');

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <AudioPage />
    </MemoryRouter>,
  );
}

function baseSettings(overrides: Partial<AudioSettings> = {}): AudioSettings {
  return {
    enabled: false,
    providerMode: 'disabled',
    enabledEventTypes: [],
    enabledProviderIds: [],
    enabledSourceIds: [],
    supporterOnlyMode: false,
    thresholdCurrency: '',
    thresholdMinimumAmountMicros: null,
    minimumBits: null,
    maxTextLengthCodePoints: 500,
    perUserCooldownSeconds: 30,
    globalCooldownSeconds: 3,
    blockedWords: [],
    removeUrls: true,
    normalizeRepeatedChars: true,
    suppressCommands: true,
    queueCapacity: 100,
    manualApproval: false,
    voiceId: '',
    language: '',
    speed: 1,
    volume: 1,
    publicSlug: 'abc123',
    createdAt: '2026-08-13T00:00:00Z',
    updatedAt: '2026-08-13T00:00:00Z',
    ...overrides,
  };
}

function baseStatus(overrides: Partial<AudioStatus> = {}): AudioStatus {
  return {
    enabled: false,
    providerMode: 'disabled',
    providerAvailable: true,
    rendererConnected: false,
    hasCurrentItem: false,
    currentSynthetic: false,
    pendingApprovalCount: 0,
    readyQueueCount: 0,
    capacity: 100,
    totalEnqueued: 0,
    totalCapacityDropped: 0,
    totalExpired: 0,
    totalRejected: 0,
    totalManuallySkipped: 0,
    totalSynthetic: 0,
    totalPlayed: 0,
    totalPlaybackFailed: 0,
    totalSynthesisFailed: 0,
    totalInterrupted: 0,
    inputGap: false,
    subscribed: true,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(audioApi).fetchAudioSettings.mockResolvedValue(baseSettings());
  vi.mocked(audioApi).fetchAudioCapabilities.mockResolvedValue({
    knownProviderModes: ['disabled', 'system', 'local', 'cloud'],
    implementedProviderModes: ['disabled', 'system'],
    systemProviderAvailable: true,
  });
  vi.mocked(audioApi).fetchAudioVoices.mockResolvedValue([
    { id: 'voice-1', name: 'Test Voice', language: 'en-US', isDefault: true },
  ]);
  vi.mocked(audioApi).fetchAudioStatus.mockResolvedValue(baseStatus());
  vi.mocked(audioApi).fetchAudioPending.mockResolvedValue([]);
});

describe('AudioPage', () => {
  it('renders the page title', async () => {
    renderPage();
    expect(await screen.findByRole('heading', { name: 'Audio' })).toBeInTheDocument();
  });

  it('loads and displays settings', async () => {
    renderPage();
    await waitFor(() => {
      expect(audioApi.fetchAudioSettings).toHaveBeenCalled();
    });
  });

  it('shows the system provider as available', async () => {
    renderPage();
    expect(await screen.findByText('System voice engine available')).toBeInTheDocument();
  });

  it('shows the system provider as unavailable with a reason', async () => {
    vi.mocked(audioApi).fetchAudioCapabilities.mockResolvedValue({
      knownProviderModes: [], implementedProviderModes: [],
      systemProviderAvailable: false, systemProviderReason: 'not on Windows',
    });
    renderPage();
    expect(await screen.findByText('System voice engine unavailable')).toBeInTheDocument();
    expect(await screen.findByText('Reason: not on Windows')).toBeInTheDocument();
  });

  it('enables the Save button once the enabled toggle is flipped, and saves', async () => {
    const user = userEvent.setup();
    vi.mocked(audioApi).updateAudioSettings.mockResolvedValue(baseSettings({ enabled: true, providerMode: 'system' }));
    renderPage();

    const toggle = await screen.findByRole('switch', { name: 'Enabled' });
    const saveButtons = await screen.findAllByRole('button', { name: 'Save' });
    expect(saveButtons[0]).toBeDisabled();

    await user.click(toggle);
    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: 'Save' })[0]).not.toBeDisabled();
    });
    await user.click(screen.getAllByRole('button', { name: 'Save' })[0]!);

    await waitFor(() => {
      expect(audioApi.updateAudioSettings).toHaveBeenCalledWith(expect.objectContaining({ enabled: true }));
    });
  });

  it('copies the Browser Source URL', async () => {
    const user = userEvent.setup();
    // userEvent.setup() installs its own navigator.clipboard stub, so
    // this must be defined after that call or it gets overwritten -
    // mirrors TwitchDeviceFlowModal.test.tsx's own identical pattern.
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    renderPage();

    const copyButton = await screen.findByRole('button', { name: 'Copy URL' });
    await user.click(copyButton);

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/overlay/audio/abc123'));
    expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument();
  });

  it('shows pending items and approves one', async () => {
    const user = userEvent.setup();
    const pending: AudioPendingItem[] = [{ id: 'auditem_1', text: 'hello there', enqueuedAt: '2026-08-13T00:00:00Z' }];
    vi.mocked(audioApi).fetchAudioPending.mockResolvedValue(pending);
    vi.mocked(audioApi).approveAudioPendingItem.mockResolvedValue(baseStatus());
    renderPage();

    expect(await screen.findByText('hello there')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Approve' }));

    await waitFor(() => {
      expect(audioApi.approveAudioPendingItem).toHaveBeenCalledWith('auditem_1');
    });
  });

  it('sends Test Speak text and clears the field on success', async () => {
    const user = userEvent.setup();
    vi.mocked(audioApi).testSpeakAudio.mockResolvedValue({
      id: 'auditem_2', text: 'hello test', enqueuedAt: '2026-08-13T00:00:00Z',
    });
    renderPage();

    const textarea = await screen.findByPlaceholderText('Type text to test the pipeline...');
    await user.type(textarea, 'hello test');
    await user.click(screen.getByRole('button', { name: 'Speak' }));

    await waitFor(() => {
      expect(audioApi.testSpeakAudio).toHaveBeenCalledWith('hello test');
    });
    await waitFor(() => {
      expect(textarea).toHaveValue('');
    });
  });

  it('skips the current item', async () => {
    const user = userEvent.setup();
    vi.mocked(audioApi).skipAudioQueueCurrent.mockResolvedValue(baseStatus());
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Skip current' }));
    await waitFor(() => expect(audioApi.skipAudioQueueCurrent).toHaveBeenCalled());
  });

  it('opens a confirmation dialog before clearing the queue', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Clear queue' }));
    expect(await screen.findByText('Clear the queue?')).toBeInTheDocument();
  });
});
