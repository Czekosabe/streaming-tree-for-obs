import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as metadataPresetsApi from '@/api/metadata-presets';
import type { ApplyDestinationPreview } from '@/api/metadata-preset-apply-schemas';
import type { MetadataPreset } from '@/api/metadata-preset-schemas';
import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { renderWithProviders } from '@/test/render';

import { ApplyPresetDialog } from './ApplyPresetDialog';

vi.mock('@/api/metadata-presets');

const preset: MetadataPreset = {
  id: 'mp_1',
  name: 'Ranked night',
  note: '',
  title: 'Ranked climb',
  description: '',
  tags: ['ranked'],
  language: 'en',
  visibility: '',
  matureContent: false,
  dvr: false,
  latencyMode: '',
  providers: {},
  createdAt: '2026-09-01T00:00:00Z',
  updatedAt: '2026-09-01T00:00:00Z',
};

function platform(overrides: Partial<ConfiguredPlatform> = {}): ConfiguredPlatform {
  return {
    id: 'pf_1',
    providerId: 'twitch',
    displayName: 'Main Twitch',
    enabled: true,
    sortOrder: 0,
    createdAt: '2026-08-01T00:00:00Z',
    updatedAt: '2026-08-01T00:00:00Z',
    metadata: {
      title: '', description: '', category: '', categoryId: '', tags: [],
      language: '', visibility: '', matureContent: false, dvr: false, latencyMode: '',
      updatedAt: '2026-08-01T00:00:00Z',
    },
    ...overrides,
  };
}

function validPreview(platformId: string): ApplyDestinationPreview {
  return {
    platformId,
    providerId: 'twitch',
    valid: true,
    fields: [{ field: 'title', status: 'will_change' }],
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ApplyPresetDialog', () => {
  it('shows a message when there are no configured destinations', () => {
    renderWithProviders(
      <ApplyPresetDialog
        open onClose={vi.fn()} preset={preset} platforms={[]}
        activeId={null} activeDirty={false} onApplied={vi.fn()}
      />,
    );
    expect(screen.getByText("Add a destination first to apply a preset to it.")).toBeInTheDocument();
  });

  it('disables Apply until a destination is selected, then fetches and shows its preview', async () => {
    vi.mocked(metadataPresetsApi).fetchApplyPreview.mockResolvedValue([validPreview('pf_1')]);
    renderWithProviders(
      <ApplyPresetDialog
        open onClose={vi.fn()} preset={preset} platforms={[platform()]}
        activeId={null} activeDirty={false} onApplied={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();

    const user = userEvent.setup();
    await user.click(screen.getByRole('checkbox'));

    await waitFor(() =>
      expect(metadataPresetsApi.fetchApplyPreview).toHaveBeenCalledWith('mp_1', ['pf_1'], expect.anything()),
    );
    expect(await screen.findByText(/Title: will change/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled();
  });

  it('disables Apply when the selected destination is incompatible', async () => {
    vi.mocked(metadataPresetsApi).fetchApplyPreview.mockResolvedValue([
      { platformId: 'pf_1', providerId: 'twitch', valid: false, fields: [], errors: { title: 'Too long.' } },
    ]);
    renderWithProviders(
      <ApplyPresetDialog
        open onClose={vi.fn()} preset={preset} platforms={[platform()]}
        activeId={null} activeDirty={false} onApplied={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole('checkbox'));

    expect(await screen.findByText("This preset can't be applied to this destination as-is.")).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('applies directly and reports the applied ids when there is no unsaved-edit conflict', async () => {
    vi.mocked(metadataPresetsApi).fetchApplyPreview.mockResolvedValue([validPreview('pf_1')]);
    vi.mocked(metadataPresetsApi).applyMetadataPreset.mockResolvedValue({ platforms: {} });
    const onApplied = vi.fn();
    renderWithProviders(
      <ApplyPresetDialog
        open onClose={vi.fn()} preset={preset} platforms={[platform()]}
        activeId={null} activeDirty={false} onApplied={onApplied}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole('checkbox'));
    await screen.findByText(/Title: will change/);
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    await waitFor(() =>
      expect(metadataPresetsApi.applyMetadataPreset).toHaveBeenCalledWith('mp_1', ['pf_1']),
    );
    await waitFor(() => expect(onApplied).toHaveBeenCalledWith(['pf_1']));
  });

  it('keeps an already-checked destination\'s chips visible while a newly-checked one is still loading', async () => {
    let resolveSecond: (value: ApplyDestinationPreview[]) => void = () => {};
    vi.mocked(metadataPresetsApi).fetchApplyPreview.mockImplementation((_id, platformIds) => {
      if (platformIds.length === 1) return Promise.resolve([validPreview('pf_1')]);
      return new Promise((resolve) => {
        resolveSecond = resolve;
      });
    });

    renderWithProviders(
      <ApplyPresetDialog
        open onClose={vi.fn()} preset={preset}
        platforms={[platform(), platform({ id: 'pf_2', displayName: 'Backup YouTube', providerId: 'youtube' })]}
        activeId={null} activeDirty={false} onApplied={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    const checkboxes = screen.getAllByRole('checkbox');
    await user.click(checkboxes[0]!);
    await screen.findByText(/Title: will change/);

    await user.click(checkboxes[1]!);
    // The second destination's own preview has not resolved yet, but the
    // first destination's chip must still be on screen (kept via
    // placeholderData: keepPreviousData), not blanked out to "Checking...".
    expect(screen.getByText(/Title: will change/)).toBeInTheDocument();
    expect(screen.getAllByText('Checking compatibility...')).toHaveLength(1);

    resolveSecond([validPreview('pf_1'), validPreview('pf_2')]);
    await waitFor(() => expect(screen.queryByText('Checking compatibility...')).not.toBeInTheDocument());
  });

  it('confirms before discarding unsaved edits on the active destination', async () => {
    vi.mocked(metadataPresetsApi).fetchApplyPreview.mockResolvedValue([validPreview('pf_1')]);
    vi.mocked(metadataPresetsApi).applyMetadataPreset.mockResolvedValue({ platforms: {} });
    renderWithProviders(
      <ApplyPresetDialog
        open onClose={vi.fn()} preset={preset} platforms={[platform()]}
        activeId="pf_1" activeDirty onApplied={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole('checkbox'));
    await screen.findByText(/Title: will change/);
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(metadataPresetsApi.applyMetadataPreset).not.toHaveBeenCalled();
    expect(await screen.findByText('Discard unsaved changes?')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Discard and apply' }));
    await waitFor(() => expect(metadataPresetsApi.applyMetadataPreset).toHaveBeenCalledWith('mp_1', ['pf_1']));
  });
});
