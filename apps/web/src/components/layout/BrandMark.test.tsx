import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import brandEmblem from '@/assets/brand-emblem.png';

import { BrandMark } from './BrandMark';

/**
 * Locks the Stage 20E branding remediation: the sidebar/header brand
 * mark must render the real, canonical generated emblem asset
 * (scripts/generate-branding-assets.go's own output), never a generic
 * icon-library placeholder - a real physical/manual Windows test found
 * the old placeholder still showing after the tray/exe/installer were
 * already updated to use the real logo.
 */
describe('BrandMark', () => {
  it('renders the canonical generated emblem image, not a generic icon', () => {
    const { container } = render(<BrandMark />);

    const img = container.querySelector('img');
    expect(img).not.toBeNull();
    expect(img?.getAttribute('src')).toBe(brandEmblem);
  });

  it('keeps the existing textual product-name lockup', () => {
    render(<BrandMark />);

    expect(screen.getByText('Streaming Tree')).toBeInTheDocument();
    expect(screen.getByText('for OBS')).toBeInTheDocument();
  });
});
