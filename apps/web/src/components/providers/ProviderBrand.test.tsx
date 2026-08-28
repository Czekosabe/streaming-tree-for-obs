import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { KNOWN_PROVIDER_BRAND_IDS, ProviderBrand } from './ProviderBrand';

const ASSETS_DIR = join(dirname(fileURLToPath(import.meta.url)), '../../assets/providers');

describe('ProviderBrand', () => {
  it('resolves every real destination provider to a known brand mark', () => {
    // The four providers this application actually supports (see
    // provider-labels.ts's own accent map) must each have a real vendored
    // mark - a Dashboard showing "TW"/"YT"/"KI"/"TT" letter tiles again
    // would be the exact regression this stage fixed.
    expect(KNOWN_PROVIDER_BRAND_IDS.sort()).toEqual(['kick', 'tiktok', 'twitch', 'youtube']);
  });

  it('renders a decorative, aria-hidden tile for a known provider', () => {
    const { container } = render(<ProviderBrand providerId="twitch" fallbackLabel="TW" />);
    const tile = container.querySelector('[aria-hidden="true"]');
    expect(tile).not.toBeNull();
    // The mark itself is never the accessible identifier - callers always
    // render visible provider/brand name text alongside this component.
    expect(screen.queryByRole('img')).toBeNull();
  });

  it('falls back to the neutral text tile for an unrecognised provider id', () => {
    render(<ProviderBrand providerId="some-future-provider" fallbackLabel="SF" />);
    expect(screen.getByText('SF')).toBeInTheDocument();
  });

  it('never throws for an empty or malformed provider id', () => {
    expect(() => render(<ProviderBrand providerId="" fallbackLabel="?" />)).not.toThrow();
  });

  for (const id of KNOWN_PROVIDER_BRAND_IDS) {
    it(`vendored ${id}.svg contains no scripts, event handlers, or external references`, () => {
      const raw = readFileSync(join(ASSETS_DIR, `${id}.svg`), 'utf8');

      // Governing requirement: reject any vendored SVG containing scripts,
      // external references, event handlers, or an unexpected
      // foreignObject before it is trusted as a static local asset.
      expect(raw).not.toMatch(/<script/i);
      expect(raw).not.toMatch(/\son[a-z]+\s*=/i);
      expect(raw).not.toMatch(/<foreignObject/i);
      // The SVG namespace declaration (xmlns="http://www.w3.org/2000/svg")
      // is expected and benign - only an actual href/src reference to an
      // external resource is a real concern.
      expect(raw).not.toMatch(/\b(?:href|src)\s*=\s*["']https?:/i);
      expect(raw).not.toMatch(/xlink:href/i);
      // Sanity: still a real, non-empty SVG with actual path geometry.
      expect(raw).toMatch(/<svg[\s\S]*<path/i);
    });
  }

  it('vendors exactly the four supported provider marks - no unused catalog files', () => {
    const files = readdirSync(ASSETS_DIR).filter((name) => name.endsWith('.svg'));
    expect(files.sort()).toEqual(['kick.svg', 'tiktok.svg', 'twitch.svg', 'youtube.svg']);
  });
});
