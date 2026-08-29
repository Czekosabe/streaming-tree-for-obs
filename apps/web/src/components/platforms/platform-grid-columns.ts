/**
 * Column-count breakpoints for `PlatformGrid`'s own container width (via
 * Tailwind's `@`-prefixed container-query variants, not `sm:`/`lg:`
 * viewport-width ones - the destination grid sits in a column whose real
 * available width depends on whether the right rail is present at all,
 * which a viewport-width breakpoint cannot see).
 *
 * Deliberately *count-aware*, not just a single fluid `auto-fit` rule: a
 * physical Windows test of an earlier `auto-fit` version found exactly
 * four configured destinations still rendering as three cards in one row
 * plus a fourth stranded alone underneath, at a real container width that
 * fit three ~280px-minimum cards but not a fourth. `auto-fit`/`auto-fill`
 * cannot see the *item count*, only the minimum card width, so it has no
 * way to know that landing on 3 columns for exactly 4 items produces an
 * orphaned last row - it will always take the "fits" answer nearest the
 * container width regardless of how evenly that divides the real count.
 * A three-column layout is genuinely the right answer when there really
 * are three destinations (not banned here), so the fix is per-count rules
 * for the small counts where an orphan is actually possible, falling back
 * to the general fluid rule once there are enough destinations that no
 * single column count reads as obviously more "balanced" than another.
 *
 * Kept in its own module (not co-located with the `PlatformGrid` component
 * itself) purely so it can be unit-tested as a pure function without
 * mounting any component, and so this file's only export stays a plain
 * function - a component file mixing component and non-component exports
 * loses React Fast Refresh's guarantee.
 */
export function columnClassesFor(count: number): string {
  if (count <= 1) return 'grid-cols-1';
  if (count === 2) return 'grid-cols-1 @lg:grid-cols-2';
  if (count === 3) return 'grid-cols-1 @md:grid-cols-2 @3xl:grid-cols-3';
  if (count === 4) {
    // Jumps straight from 2 to 4 columns - a 3-column breakpoint is
    // deliberately never offered here, since that is exactly the
    // arrangement that strands the fourth card alone underneath.
    return 'grid-cols-1 @xl:grid-cols-2 @4xl:grid-cols-4';
  }
  // Five or more: no single column count is obviously "the balanced one"
  // for an arbitrary, unbounded destination count, so this falls back to
  // the general fluid rule - it still respects the real container width
  // (grid's own auto-fit/auto-fill sizing is always relative to its
  // containing block, not the viewport) and, being `auto-fit`, never
  // leaves a large dead area the way `auto-fill` would.
  return 'grid-cols-[repeat(auto-fit,minmax(280px,1fr))]';
}
