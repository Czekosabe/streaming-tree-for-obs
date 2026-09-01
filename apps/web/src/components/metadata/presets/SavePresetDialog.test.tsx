import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as metadataPresetsApi from '@/api/metadata-presets';
import type { SaveMetadataInput } from '@/api/platform-schemas';
import { renderWithProviders } from '@/test/render';

import { SavePresetDialog } from './SavePresetDialog';

vi.mock('@/api/metadata-presets');

function draft(overrides: Partial<SaveMetadataInput> = {}): SaveMetadataInput {
  return {
    title: 'Ranked climb',
    description: 'Grinding to Diamond',
    category: 'League of Legends',
    categoryId: '21779',
    tags: ['ranked', 'lol'],
    language: 'en',
    visibility: 'public',
    matureContent: false,
    dvr: true,
    latencyMode: 'normal',
    ...overrides,
  };
}

const preset = {
  id: 'mp_1',
  name: 'Ranked night',
  note: '',
  title: 'Ranked climb',
  description: 'Grinding to Diamond',
  tags: ['ranked', 'lol'],
  language: 'en',
  visibility: 'public',
  matureContent: false,
  dvr: true,
  latencyMode: 'normal',
  providers: { twitch: { category: 'League of Legends', categoryId: '21779' } },
  createdAt: '2026-09-01T00:00:00Z',
  updatedAt: '2026-09-01T00:00:00Z',
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SavePresetDialog', () => {
  it('is disabled until a name is entered, and never sends an empty name', async () => {
    renderWithProviders(
      <SavePresetDialog open onClose={vi.fn()} providerId="twitch" draft={draft()} />,
    );

    expect(screen.getByRole('button', { name: 'Save preset' })).toBeDisabled();
  });

  it('creates a preset scoping the category under the current provider only', async () => {
    vi.mocked(metadataPresetsApi).createMetadataPreset.mockResolvedValue(preset);
    renderWithProviders(
      <SavePresetDialog open onClose={vi.fn()} providerId="twitch" draft={draft()} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Preset name'), 'Ranked night');
    await user.click(screen.getByRole('button', { name: 'Save preset' }));

    await waitFor(() =>
      expect(metadataPresetsApi.createMetadataPreset).toHaveBeenCalledWith({
        name: 'Ranked night',
        note: '',
        title: 'Ranked climb',
        description: 'Grinding to Diamond',
        tags: ['ranked', 'lol'],
        language: 'en',
        visibility: 'public',
        matureContent: false,
        dvr: true,
        latencyMode: 'normal',
        providers: { twitch: { category: 'League of Legends', categoryId: '21779' } },
      }),
    );
  });

  it('omits the provider entry entirely when the draft has no category', async () => {
    vi.mocked(metadataPresetsApi).createMetadataPreset.mockResolvedValue(preset);
    renderWithProviders(
      <SavePresetDialog
        open
        onClose={vi.fn()}
        providerId="kick"
        draft={draft({ category: '', categoryId: '' })}
      />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Preset name'), 'No category preset');
    await user.click(screen.getByRole('button', { name: 'Save preset' }));

    await waitFor(() =>
      expect(metadataPresetsApi.createMetadataPreset).toHaveBeenCalledWith(
        expect.objectContaining({ providers: {} }),
      ),
    );
  });

  it('shows the server error message when creation fails', async () => {
    vi.mocked(metadataPresetsApi).createMetadataPreset.mockRejectedValue(new Error('boom'));
    renderWithProviders(
      <SavePresetDialog open onClose={vi.fn()} providerId="twitch" draft={draft()} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Preset name'), 'Ranked night');
    await user.click(screen.getByRole('button', { name: 'Save preset' }));

    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });

  it('resets its local state each time it is reopened', async () => {
    vi.mocked(metadataPresetsApi).createMetadataPreset.mockResolvedValue(preset);
    const { rerender } = renderWithProviders(
      <SavePresetDialog open onClose={vi.fn()} providerId="twitch" draft={draft()} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Preset name'), 'Draft name');

    rerender(
      <SavePresetDialog open={false} onClose={vi.fn()} providerId="twitch" draft={draft()} />,
    );
    rerender(<SavePresetDialog open onClose={vi.fn()} providerId="twitch" draft={draft()} />);

    expect(screen.getByLabelText('Preset name')).toHaveValue('');
  });
});
