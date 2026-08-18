import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import * as aboutApi from '@/api/about';
import { i18n } from '@/i18n';
import { renderWithProviders } from '@/test/render';

import { AboutLegalPage } from './AboutLegalPage';

vi.mock('@/api/about');

const ABOUT_RESPONSE = {
  productName: 'Streaming Tree for OBS',
  version: '0.1.0',
  isReleaseBuild: false,
  creatorName: 'Czekosabe',
  repositoryUrl: 'https://github.com/Czekosabe/streaming-tree-for-obs',
  creatorUrl: 'https://github.com/Czekosabe',
  supportUrl: 'https://streamelements.com/czekosabe/tip',
  applicationLicenseSpdx: 'GPL-3.0-or-later',
  applicationLicenseName: 'GNU General Public License v3 or later',
};

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <AboutLegalPage />
    </MemoryRouter>,
  );
}

describe('AboutLegalPage', () => {
  afterEach(() => {
    void i18n.changeLanguage('en');
  });

  it('shows the product name, creator, and development-build version state', async () => {
    vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
    renderPage();

    expect(await screen.findByText('Streaming Tree for OBS')).toBeInTheDocument();
    expect(screen.getByText('Czekosabe')).toBeInTheDocument();
    expect(screen.getByText(/development build/i)).toBeInTheDocument();
  });

  it('never renders a real name, email, or other personal/local identifier', async () => {
    vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
    const { container } = renderPage();
    await screen.findByText('Czekosabe');

    const text = container.textContent ?? '';
    expect(text).not.toMatch(/@/);
    expect(text.toLowerCase()).not.toContain('kacper');
    expect(text.toLowerCase()).not.toContain('tlen.pl');
    expect(screen.queryByText(/email/i)).not.toBeInTheDocument();
  });

  it('links to the canonical repository, creator profile, and support URLs with safe target/rel', async () => {
    vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
    renderPage();

    const repoLink = await screen.findByRole('link', { name: /source code/i });
    expect(repoLink).toHaveAttribute('href', 'https://github.com/Czekosabe/streaming-tree-for-obs');
    expect(repoLink).toHaveAttribute('target', '_blank');
    expect(repoLink).toHaveAttribute('rel', expect.stringContaining('noopener'));
    expect(repoLink).toHaveAttribute('rel', expect.stringContaining('noreferrer'));

    const creatorLink = screen.getByRole('link', { name: /creator on github/i });
    expect(creatorLink).toHaveAttribute('href', 'https://github.com/Czekosabe');
    expect(creatorLink).toHaveAttribute('target', '_blank');

    const supportLink = screen.getByRole('link', { name: /support the creator/i });
    expect(supportLink).toHaveAttribute('href', 'https://streamelements.com/czekosabe/tip');
    expect(supportLink).toHaveAttribute('target', '_blank');
    expect(supportLink).toHaveAttribute('rel', expect.stringContaining('noopener'));
    expect(supportLink).toHaveAttribute('rel', expect.stringContaining('noreferrer'));
  });

  it('discloses the external support destination and states support is voluntary with no feature unlock', async () => {
    vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
    renderPage();

    await screen.findByRole('link', { name: /support the creator/i });
    expect(screen.getByText(/opens streamelements in your browser/i)).toBeInTheDocument();
    expect(screen.getByText(/voluntary/i)).toBeInTheDocument();
    expect(screen.getByText(/does not unlock features/i)).toBeInTheDocument();
    expect(screen.getByText(/does not process payment data/i)).toBeInTheDocument();
  });

  it('shows Privacy, Third-party notices, and Disclaimer sections, and the unresolved application-licence state', async () => {
    vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
    renderPage();

    expect(await screen.findByText('Application licence')).toBeInTheDocument();
    expect(screen.getByText(/GNU General Public License v3 or later/)).toBeInTheDocument();
    expect(screen.getByText('SPDX: GPL-3.0-or-later')).toBeInTheDocument();
    expect(screen.queryByText(/has not been selected/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/unselected/i)).not.toBeInTheDocument();
    expect(screen.getByText('Privacy')).toBeInTheDocument();
    expect(screen.getByText('Third-party notices')).toBeInTheDocument();
    expect(screen.getByText('Disclaimer')).toBeInTheDocument();
    expect(screen.getByText(/independent project/i)).toBeInTheDocument();
  });

  it('renders no donation/payment form, embedded checkout, or amount field', async () => {
    vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
    const { container } = renderPage();
    await screen.findByRole('link', { name: /support the creator/i });

    expect(container.querySelector('form')).not.toBeInTheDocument();
    expect(container.querySelector('iframe')).not.toBeInTheDocument();
    expect(container.querySelector('input')).not.toBeInTheDocument();
  });

  it('renders representative Polish copy for the support card', async () => {
    vi.mocked(aboutApi).fetchAbout.mockResolvedValue(ABOUT_RESPONSE);
    await i18n.changeLanguage('pl');
    renderPage();

    expect(await screen.findByRole('link', { name: 'Wesprzyj twórcę' })).toBeInTheDocument();
    expect(screen.getByText(/otwiera streamelements w przeglądarce/i)).toBeInTheDocument();
    expect(screen.getByText(/dobrowolne/i)).toBeInTheDocument();
  });
});
