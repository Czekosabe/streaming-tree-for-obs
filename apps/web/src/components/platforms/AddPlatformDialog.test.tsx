import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as platformsApi from '@/api/platforms';
import type { ConfiguredPlatform, ProviderDefinition } from '@/api/platform-schemas';
import { renderWithProviders } from '@/test/render';

import { AddPlatformDialog } from './AddPlatformDialog';

vi.mock('@/api/platforms');

/**
 * Stage 20E regression coverage for a real physical/manual Windows
 * finding: opening Add Platform for the first time in a session showed
 * "Twitch" visibly selected in the Platform field, but submitting with
 * only a Display name typed in still failed with "Select a platform." -
 * the visible selection and the actual form state had gone out of sync.
 *
 * Root cause: providerId used to be seeded from `definitions[0]?.id` via
 * useState's initializer, which runs exactly once at mount - before
 * usePlatformDefinitionsQuery's async result had ever arrived on a real
 * first open, so it captured '' permanently. Once real definitions
 * arrived afterward, the <select>'s stale '' value matched none of its
 * <option> elements, and a native <select> visually falls back to
 * showing its first option in that situation - a real, observable
 * divergence between what the browser paints and what React actually
 * holds. These tests reproduce that exact timing (render with an empty
 * definitions list, exactly like a query that has not resolved yet, then
 * rerender with the real list once it "arrives") rather than only
 * testing the already-loaded case, which would never have caught this.
 */
function providerDefinition(id: string, brandName: string): ProviderDefinition {
  return {
    id,
    brandName,
    shortLabel: brandName,
    categoryFieldType: 'freetext',
    categoryRequiresRemoteId: false,
    capabilities: {
      title: true,
      description: true,
      category: true,
      tags: true,
      language: true,
      visibility: true,
      matureContent: true,
      dvr: true,
      latencyMode: true,
    },
    limits: { titleMaxLength: 140, descriptionMaxLength: 1000, maxTags: 10, tagMaxLength: 25 },
    visibilityOptions: ['public'],
    latencyOptions: ['normal'],
    languageOptions: ['en'],
  };
}

function configuredPlatform(overrides: Partial<ConfiguredPlatform> = {}): ConfiguredPlatform {
  return {
    id: 'pf_1',
    providerId: 'twitch',
    displayName: 'Main Twitch channel',
    enabled: false,
    sortOrder: 0,
    createdAt: '2026-08-27T00:00:00Z',
    updatedAt: '2026-08-27T00:00:00Z',
    metadata: {
      title: '', description: '', category: '', categoryId: '', tags: [],
      language: '', visibility: '', matureContent: false, dvr: false, latencyMode: '',
      updatedAt: '2026-08-27T00:00:00Z',
    },
    ...overrides,
  };
}

const twitch = providerDefinition('twitch', 'Twitch');
const youtube = providerDefinition('youtube', 'YouTube');

beforeEach(() => {
  vi.clearAllMocks();
});

describe('AddPlatformDialog - controlled platform selection', () => {
  it('shows the placeholder, not a real platform, before definitions have loaded', () => {
    renderWithProviders(<AddPlatformDialog open onClose={vi.fn()} definitions={[]} />);

    const select = screen.getByLabelText('Platform') as HTMLSelectElement;
    expect(select.value).toBe('');
    expect(within(select).getByText('Select a platform…')).toBeInTheDocument();
  });

  it('reproduces the real first-open sequence: definitions arrive after mount, but the placeholder stays selected until the operator chooses', async () => {
    const { rerender } = renderWithProviders(
      <AddPlatformDialog open onClose={vi.fn()} definitions={[]} />,
    );

    // The provider-definitions query "resolves" after this component has
    // already mounted - the exact real-world timing the bug depended on.
    rerender(<AddPlatformDialog open onClose={vi.fn()} definitions={[twitch, youtube]} />);

    const select = screen.getByLabelText('Platform') as HTMLSelectElement;
    expect(select.value).toBe('');

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Display name'), 'Main Twitch channel');
    await user.click(screen.getByRole('button', { name: 'Add platform' }));

    expect(await screen.findByText('Select a platform.')).toBeInTheDocument();
    expect(platformsApi.createPlatform).not.toHaveBeenCalled();
    // The select must still show the placeholder, not "Twitch" - the
    // real bug had the browser painting the first option while state
    // (and therefore validation) disagreed.
    expect(select.value).toBe('');
  });

  it('submits the explicitly chosen platform once the operator actually selects one', async () => {
    vi.mocked(platformsApi).createPlatform.mockResolvedValue(configuredPlatform());
    const onClose = vi.fn();
    renderWithProviders(<AddPlatformDialog open onClose={onClose} definitions={[twitch, youtube]} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('Platform'), 'Twitch');
    await user.type(screen.getByLabelText('Display name'), 'Main Twitch channel');
    await user.click(screen.getByRole('button', { name: 'Add platform' }));

    await waitFor(() =>
      expect(platformsApi.createPlatform).toHaveBeenCalledWith({
        providerId: 'twitch',
        displayName: 'Main Twitch channel',
        enabled: false,
      }),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('starts empty again on every subsequent open - it must not inherit the previous choice', async () => {
    vi.mocked(platformsApi).createPlatform.mockResolvedValue(configuredPlatform());
    const onClose = vi.fn();
    const { rerender } = renderWithProviders(
      <AddPlatformDialog open onClose={onClose} definitions={[twitch, youtube]} />,
    );

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('Platform'), 'Twitch');
    await user.type(screen.getByLabelText('Display name'), 'Main Twitch channel');
    await user.click(screen.getByRole('button', { name: 'Add platform' }));
    await waitFor(() => expect(platformsApi.createPlatform).toHaveBeenCalledTimes(1));

    // Close, then open a fresh Add Platform - the same instance the real
    // app keeps mounted the whole session, exactly like DashboardPage's
    // own always-mounted AddPlatformDialog.
    rerender(<AddPlatformDialog open={false} onClose={onClose} definitions={[twitch, youtube]} />);
    rerender(<AddPlatformDialog open onClose={onClose} definitions={[twitch, youtube]} />);

    const select = screen.getByLabelText('Platform') as HTMLSelectElement;
    expect(select.value).toBe('');
    expect(screen.getByLabelText('Display name')).toHaveValue('');
  });
});

describe('AddPlatformDialog - preserves other form behavior', () => {
  it('keeps focus in the Display name field while typing, unaffected by the Platform fix', async () => {
    renderWithProviders(<AddPlatformDialog open onClose={vi.fn()} definitions={[twitch]} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText('Display name');
    await user.click(input);
    await user.type(input, 'Streaming Tree');

    expect(input).toHaveValue('Streaming Tree');
    expect(input).toHaveFocus();
  });

  it('changing the Platform selection does not move focus or clear Display name', async () => {
    renderWithProviders(<AddPlatformDialog open onClose={vi.fn()} definitions={[twitch, youtube]} />);

    const user = userEvent.setup();
    const nameInput = screen.getByLabelText('Display name');
    await user.type(nameInput, 'Main channel');

    const select = screen.getByLabelText('Platform');
    await user.selectOptions(select, 'YouTube');

    expect(nameInput).toHaveValue('Main channel');
    expect(select).toHaveFocus();
  });

  it('Cancel resets the form, including the platform selection, back to the placeholder', async () => {
    const onClose = vi.fn();
    renderWithProviders(<AddPlatformDialog open onClose={onClose} definitions={[twitch]} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('Platform'), 'Twitch');
    await user.type(screen.getByLabelText('Display name'), 'Draft');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onClose).toHaveBeenCalled();
  });
});
