import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { KNOWN_PROVIDER_BRAND_IDS, PROVIDER_MARK_PATHS, ProviderBrand } from './ProviderBrand';

const ASSETS_DIR = join(dirname(fileURLToPath(import.meta.url)), '../../assets/providers');

/** Official (or, for TikTok, contrast-corrected) fill colour each provider
 * mark must render with - see ProviderBrand.tsx's own doc comment for why
 * TikTok's is a light neutral tone rather than its official near-black. */
const EXPECTED_HEX: Record<string, string> = {
  twitch: '#9146FF',
  youtube: '#FF0000',
  kick: '#53FC19',
  tiktok: '#f4f6fb',
};

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

  /**
   * Renders inline SVG rather than a CSS `mask-image` referencing an
   * external/bundler-resolved asset URL, specifically so this proves real
   * rendered vector geometry with the correct fill colour - not merely
   * `backgroundColor === brandColor` on an otherwise-empty element, which
   * would pass even if the actual visible result were a flat coloured
   * square (the exact physical regression this test guards against).
   */
  for (const id of KNOWN_PROVIDER_BRAND_IDS) {
    it(`${id} renders a real <svg><path> with the correct fill - not a flat coloured square`, () => {
      const { container } = render(<ProviderBrand providerId={id} fallbackLabel="??" />);

      const svg = container.querySelector('svg');
      expect(svg).not.toBeNull();
      expect(svg?.getAttribute('viewBox')).toBe('0 0 24 24');

      const path = container.querySelector('svg path');
      expect(path).not.toBeNull();
      // Real geometry, not an empty/placeholder path.
      expect(path?.getAttribute('d')?.length ?? 0).toBeGreaterThan(20);
      expect(path?.getAttribute('fill')).toBe(EXPECTED_HEX[id]);
    });
  }

  for (const id of KNOWN_PROVIDER_BRAND_IDS) {
    it(`${id}'s rendered path data matches the vendored ${id}.svg file exactly`, () => {
      // Keeps the component's own hardcoded geometry (used for reliable
      // inline rendering, with no dependency on how the bundler resolves
      // an external asset URL) from silently drifting away from the
      // vendored file that documents its real provenance.
      const raw = readFileSync(join(ASSETS_DIR, `${id}.svg`), 'utf8');
      const match = /<path d="([^"]+)"/.exec(raw);
      expect(match).not.toBeNull();
      expect(PROVIDER_MARK_PATHS[id]).toBe(match?.[1]);
    });
  }

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
