/**
 * The one shared stacking-order scale for every fixed/sticky overlay
 * in the application.
 *
 * A Stage 20E manual test found ordinary Dashboard content rendering
 * above an open modal dialog - not because anything had an explicit,
 * competing z-index (a repo-wide audit at the time found none), but
 * because an entrance animation left a lingering CSS transform on
 * unrelated page content, which silently created a new stacking
 * context that changed how the browser resolved paint order relative
 * to a modal rendered elsewhere in the tree (see `index.css`'s own
 * `animate-fade-rise` doc comment for the exact mechanism, and
 * `components/ui/Modal.tsx`, which now also renders through a portal
 * so it no longer depends on any page-content ancestor staying free
 * of stacking-context side effects at all). This module exists so a
 * *number* is never invented again at an individual call site:
 * everything that needs to sit above ordinary page content picks a
 * named layer here, and the ordering between layers is reviewable in
 * one place.
 *
 * Ordered lowest to highest. A later entry's numeric value is always
 * greater than every entry above it - the scale itself is the
 * documentation of intended paint order, not the individual numbers.
 */
export const APP_LAYER_Z = {
  /** The sticky page header (`TopBar`). Sits above ordinary in-flow
   * page content (which never sets its own z-index) by virtue of
   * `position: sticky` alone: this z-index only matters relative to
   * *other* positioned overlays in this same scale. */
  topBar: 'z-30',
  /** The off-canvas mobile navigation drawer (`MobileSidebar`).
   * Deliberately below `modal`: a dialog opened while the drawer is
   * open must never render underneath it. */
  mobileSidebarDrawer: 'z-40',
  /** Modal dialogs (`components/ui/Modal.tsx`, and everything built on
   * it - `ConfirmDialog`, every feature-specific dialog). The highest
   * layer in ordinary use; also rendered through a React portal
   * directly under `<body>`, so no page-content ancestor can ever
   * affect its stacking order regardless of what else changes in this
   * scale or elsewhere in the app. */
  modal: 'z-50',
} as const;
