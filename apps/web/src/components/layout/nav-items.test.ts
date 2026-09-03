import { describe, expect, it } from 'vitest';

import { NAV_ITEMS } from './nav-items';

/**
 * Locks Stage 20E "complete Platforms/Metadata": their nav "Soon" badge
 * (`SidebarNav` renders one only when `item.planned` is true) must be gone
 * now that both routes are real pages, not placeholders.
 */
describe('NAV_ITEMS', () => {
  it('no longer marks Platforms as planned', () => {
    expect(NAV_ITEMS.find((item) => item.to === '/platforms')?.planned).toBe(false);
  });

  it('no longer marks Metadata as planned', () => {
    expect(NAV_ITEMS.find((item) => item.to === '/metadata')?.planned).toBe(false);
  });
});
