import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as donationSourcesApi from '@/api/donationsources';
import type { DonationConnector, DonationSource } from '@/api/donationsource-schemas';
import { renderWithProviders } from '@/test/render';

import { StreamElementsConnectorCard } from './StreamElementsConnectorCard';

vi.mock('@/api/donationsources');

const api = vi.mocked(donationSourcesApi);

beforeEach(() => {
  vi.clearAllMocks();
});

function source(overrides: Partial<DonationSource> = {}): DonationSource {
  return {
    id: 'donsrc_1',
    providerId: 'streamelements',
    label: 'Main channel',
    enabled: true,
    remoteChannelId: 'chan_1',
    credentialConfigured: true,
    createdAt: '2026-08-12T00:00:00Z',
    updatedAt: '2026-08-12T00:00:00Z',
    ...overrides,
  };
}

function connector(overrides: Partial<DonationConnector> = {}): DonationConnector {
  return {
    sourceId: 'donsrc_1',
    enabled: true,
    state: 'connected',
    reconnectCount: 0,
    possibleGapCount: 0,
    ...overrides,
  };
}

describe('StreamElementsConnectorCard list', () => {
  it('renders an existing source and its connection state', async () => {
    api.fetchDonationSources.mockResolvedValue([source()]);
    api.fetchDonationSourceEngagement.mockResolvedValue(connector());

    renderWithProviders(<StreamElementsConnectorCard />);

    await screen.findByText('Main channel');
    await screen.findByText(/connected/i);
  });

  it('never renders the stored credential value or a raw JWT-shaped field name', async () => {
    api.fetchDonationSources.mockResolvedValue([source()]);
    api.fetchDonationSourceEngagement.mockResolvedValue(connector());

    renderWithProviders(<StreamElementsConnectorCard />);
    await screen.findByText('Main channel');

    const rendered = document.body.textContent ?? '';
    expect(rendered).not.toMatch(/eyj[a-z0-9_-]{10,}/i); // a JWT-shaped string
  });
});

describe('StreamElementsConnectorCard add source', () => {
  it('submits label, remoteChannelId and token, then clears the form on success', async () => {
    const user = userEvent.setup();
    api.fetchDonationSources.mockResolvedValue([]);
    api.createDonationSource.mockResolvedValue(source({ label: 'New source', remoteChannelId: 'chan_9' }));

    renderWithProviders(<StreamElementsConnectorCard />);
    await screen.findByText(/add a streamelements donation source/i);

    await user.type(screen.getByLabelText(/^label$/i), 'New source');
    await user.type(screen.getByLabelText(/streamelements account id/i), 'chan_9');
    await user.type(screen.getByLabelText(/jwt token/i), 'super-secret-jwt-value');
    await user.click(screen.getByRole('button', { name: /add donation source/i }));

    await waitFor(() =>
      expect(api.createDonationSource).toHaveBeenCalledWith({
        providerId: 'streamelements',
        label: 'New source',
        remoteChannelId: 'chan_9',
        token: 'super-secret-jwt-value',
      }),
    );

    await waitFor(() => expect(screen.getByLabelText(/jwt token/i)).toHaveValue(''));
  });

  it('disables the submit button until label, room, and token are all filled in', async () => {
    api.fetchDonationSources.mockResolvedValue([]);

    renderWithProviders(<StreamElementsConnectorCard />);
    await screen.findByText(/add a streamelements donation source/i);

    expect(screen.getByRole('button', { name: /add donation source/i })).toBeDisabled();
  });
});

describe('StreamElementsConnectorCard enable/disable', () => {
  it('toggles engagement on', async () => {
    const user = userEvent.setup();
    api.fetchDonationSources.mockResolvedValue([source({ enabled: false })]);
    api.fetchDonationSourceEngagement.mockResolvedValue(connector({ enabled: false, state: 'disabled' }));
    api.setDonationSourceEngagement.mockResolvedValue(connector());

    renderWithProviders(<StreamElementsConnectorCard />);
    const toggle = await screen.findByRole('switch', { name: /enable this donation source/i });
    await user.click(toggle);

    await waitFor(() =>
      expect(api.setDonationSourceEngagement).toHaveBeenCalledWith('donsrc_1', { enabled: true }),
    );
  });
});

describe('StreamElementsConnectorCard delete', () => {
  it('requires confirmation before deleting', async () => {
    const user = userEvent.setup();
    api.fetchDonationSources.mockResolvedValue([source()]);
    api.fetchDonationSourceEngagement.mockResolvedValue(connector());
    api.deleteDonationSource.mockResolvedValue(undefined);

    renderWithProviders(<StreamElementsConnectorCard />);
    await screen.findByText('Main channel');

    await user.click(screen.getByText(/manage/i));
    await user.click(screen.getByRole('button', { name: /delete donation source/i }));

    const dialog = await screen.findByRole('dialog');
    expect(api.deleteDonationSource).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: /delete donation source/i }));

    await waitFor(() => expect(api.deleteDonationSource).toHaveBeenCalledWith('donsrc_1'));
  });
});
