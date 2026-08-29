import { describe, expect, it } from 'vitest';

import { columnClassesFor } from './platform-grid-columns';

/**
 * Regression coverage for a real physical Windows finding: exactly four
 * configured destinations rendered as three cards in one row plus a
 * fourth stranded alone underneath, at a container width that fit three
 * ~280px-minimum cards but not a fourth. Pure-function coverage of the
 * breakpoint rules themselves - not a full component render - since the
 * defect is entirely in which Tailwind container-query classes get
 * chosen for a given destination count, not in how a card itself renders.
 */
describe('columnClassesFor', () => {
  it('never offers a 3-column breakpoint for exactly four destinations - the exact orphan-row regression', () => {
    const classes = columnClassesFor(4);
    expect(classes).not.toMatch(/grid-cols-3\b/);
    expect(classes).not.toMatch(/@\w+:grid-cols-3\b/);
  });

  it('jumps from 2 to 4 columns for exactly four destinations', () => {
    const classes = columnClassesFor(4);
    expect(classes).toMatch(/grid-cols-2\b/);
    expect(classes).toMatch(/grid-cols-4\b/);
  });

  it('allows a real 3-column layout when there genuinely are three destinations', () => {
    const classes = columnClassesFor(3);
    expect(classes).toMatch(/grid-cols-3\b/);
  });

  it('never exceeds the real destination count for one or two destinations', () => {
    expect(columnClassesFor(1)).toBe('grid-cols-1');
    expect(columnClassesFor(2)).not.toMatch(/grid-cols-3\b|grid-cols-4\b/);
  });

  it('falls back to a fluid, container-width-based rule for five or more destinations', () => {
    const classes = columnClassesFor(5);
    expect(classes).toContain('auto-fit');
  });

  it('is a pure function of the count alone - the same count always yields the same classes', () => {
    expect(columnClassesFor(4)).toBe(columnClassesFor(4));
  });
});
