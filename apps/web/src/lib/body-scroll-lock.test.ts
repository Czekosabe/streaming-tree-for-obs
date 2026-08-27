import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { acquireBodyScrollLock } from './body-scroll-lock';

/**
 * Stage 20E regression coverage for the reference-counted body-scroll
 * lock: a real physical/manual Windows test found the page permanently
 * unscrollable after deleting a destination, because two independently
 * naive `document.body.style.overflow` locks (the settings modal and
 * its nested delete-confirm modal) captured/restored the "previous"
 * value out of order when both closed in the same batched update. See
 * the doc comment on `acquireBodyScrollLock` itself for the full
 * mechanism.
 */
describe('acquireBodyScrollLock', () => {
  beforeEach(() => {
    document.body.style.overflow = '';
  });

  afterEach(() => {
    document.body.style.overflow = '';
  });

  it('locks on acquire and restores the original value on release', () => {
    document.body.style.overflow = 'auto';

    const release = acquireBodyScrollLock();
    expect(document.body.style.overflow).toBe('hidden');

    release();
    expect(document.body.style.overflow).toBe('auto');
  });

  it('keeps the page locked while a second, nested lock is still held, and restores only once both release', () => {
    document.body.style.overflow = 'auto';

    const releaseFirst = acquireBodyScrollLock();
    expect(document.body.style.overflow).toBe('hidden');

    const releaseSecond = acquireBodyScrollLock();
    expect(document.body.style.overflow).toBe('hidden');

    releaseFirst();
    expect(document.body.style.overflow).toBe('hidden');

    releaseSecond();
    expect(document.body.style.overflow).toBe('auto');
  });

  it('restores the true original value regardless of which holder releases last', () => {
    document.body.style.overflow = 'auto';

    const releaseFirst = acquireBodyScrollLock();
    const releaseSecond = acquireBodyScrollLock();

    // The second holder releases first this time - the exact ordering
    // the real bug depended on. The true original value ('auto') must
    // still win once the last holder releases, not the second holder's
    // own (contaminated, already-'hidden') captured value.
    releaseSecond();
    expect(document.body.style.overflow).toBe('hidden');

    releaseFirst();
    expect(document.body.style.overflow).toBe('auto');
  });

  it('release is idempotent - calling it twice never over-decrements the count', () => {
    document.body.style.overflow = 'auto';

    const releaseFirst = acquireBodyScrollLock();
    const releaseSecond = acquireBodyScrollLock();

    releaseFirst();
    releaseFirst(); // a stray second call, e.g. from a defensive cleanup
    expect(document.body.style.overflow).toBe('hidden');

    releaseSecond();
    expect(document.body.style.overflow).toBe('auto');
  });
});
