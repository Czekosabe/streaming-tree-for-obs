import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as metadataPresetsApi from '@/api/metadata-presets';
import type { MetadataPreset } from '@/api/metadata-preset-schemas';
import { renderWithProviders } from '@/test/render';

import { ManagePresetsDialog } from './ManagePresetsDialog';

vi.mock('@/api/metadata-presets');

function preset(overrides: Partial<MetadataPreset> = {}): MetadataPreset {
  return {
    id: 'mp_1',
    name: 'Ranked night',
    note: 'Used every Tuesday',
    title: 'Ranked climb',
    description: 'Grinding to Diamond',
    tags: ['ranked'],
    language: 'en',
    visibility: 'public',
    matureContent: false,
    dvr: true,
    latencyMode: 'normal',
    providers: { twitch: { category: 'League of Legends', categoryId: '21779' } },
    createdAt: '2026-09-01T00:00:00Z',
    updatedAt: '2026-09-01T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ManagePresetsDialog', () => {
  it('shows a creator-oriented empty state when there are no presets', async () => {
    vi.mocked(metadataPresetsApi).fetchMetadataPresets.mockResolvedValue([]);
    renderWithProviders(<ManagePresetsDialog open onClose={vi.fn()} />);

    expect(await screen.findByText('No presets yet')).toBeInTheDocument();
  });

  it('lists an existing preset with its name, note and provider scope', async () => {
    vi.mocked(metadataPresetsApi).fetchMetadataPresets.mockResolvedValue([preset()]);
    renderWithProviders(<ManagePresetsDialog open onClose={vi.fn()} />);

    expect(await screen.findByText('Ranked night')).toBeInTheDocument();
    expect(screen.getByText('Used every Tuesday')).toBeInTheDocument();
  });

  it('renames a preset while preserving its captured metadata', async () => {
    const existing = preset();
    vi.mocked(metadataPresetsApi).fetchMetadataPresets.mockResolvedValue([existing]);
    vi.mocked(metadataPresetsApi).updateMetadataPreset.mockResolvedValue({
      ...existing,
      name: 'Renamed night',
    });
    renderWithProviders(<ManagePresetsDialog open onClose={vi.fn()} />);

    await screen.findByText('Ranked night');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Rename' }));

    const input = screen.getByLabelText('Preset name');
    await user.clear(input);
    await user.type(input, 'Renamed night');
    await user.click(screen.getByRole('button', { name: 'Save name' }));

    await waitFor(() =>
      expect(metadataPresetsApi.updateMetadataPreset).toHaveBeenCalledWith('mp_1', {
        name: 'Renamed night',
        note: existing.note,
        title: existing.title,
        description: existing.description,
        tags: existing.tags,
        language: existing.language,
        visibility: existing.visibility,
        matureContent: existing.matureContent,
        dvr: existing.dvr,
        latencyMode: existing.latencyMode,
        providers: existing.providers,
      }),
    );
  });

  it('deletes a preset only after the confirmation dialog is accepted', async () => {
    vi.mocked(metadataPresetsApi).fetchMetadataPresets.mockResolvedValue([preset()]);
    vi.mocked(metadataPresetsApi).deleteMetadataPreset.mockResolvedValue(undefined);
    renderWithProviders(<ManagePresetsDialog open onClose={vi.fn()} />);

    await screen.findByText('Ranked night');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Delete preset' }));

    expect(metadataPresetsApi.deleteMetadataPreset).not.toHaveBeenCalled();

    const dialog = await screen.findByText('Delete this preset?');
    const confirmDialog = dialog.closest('[role="dialog"]') ?? document.body;
    await user.click(within(confirmDialog as HTMLElement).getByRole('button', { name: 'Delete preset' }));

    await waitFor(() => expect(metadataPresetsApi.deleteMetadataPreset).toHaveBeenCalledWith('mp_1'));
  });
});
