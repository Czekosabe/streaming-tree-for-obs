/**
 * Reference-counted `document.body` scroll lock shared by every
 * overlay that needs to suspend background scrolling (the shared
 * `Modal` and the mobile navigation drawer).
 *
 * A real Stage 20E physical/manual Windows test found the page
 * permanently unscrollable (until a manual refresh) after deleting a
 * destination: `PlatformSettingsDialog` renders its own settings
 * `Modal` (always open while a platform is selected) alongside a
 * `ConfirmDialog`-wrapped `Modal` for the delete step - two modals
 * open at once. Each modal independently captured
 * `document.body.style.overflow` as its own "previous value" when it
 * opened; the confirm modal opened *after* the settings modal had
 * already locked scrolling, so it captured `'hidden'` as its own
 * "previous" value instead of the page's true original state. A
 * successful delete closes both modals in the same batched React
 * update, and whichever modal's cleanup happened to run last
 * overwrote the other's (correct) restoration with its own
 * (contaminated) one - leaving the body permanently locked whenever
 * that happened to be the confirm modal.
 *
 * Counting acquisitions instead fixes this for any nesting depth and
 * any close order: only the very first `acquire()` call captures the
 * true original value, and only the release that brings the count
 * back to zero restores it.
 */

let lockCount = 0;
let originalOverflow: string | null = null;

/** Acquires the lock, applying it immediately if this is the first
 * holder. Returns a release function - idempotent, so calling it more
 * than once (e.g. from a defensive cleanup) never double-decrements. */
export function acquireBodyScrollLock(): () => void {
  if (lockCount === 0) {
    originalOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
  }
  lockCount += 1;

  let released = false;
  return () => {
    if (released) return;
    released = true;

    lockCount -= 1;
    if (lockCount === 0) {
      document.body.style.overflow = originalOverflow ?? '';
      originalOverflow = null;
    }
  };
}
