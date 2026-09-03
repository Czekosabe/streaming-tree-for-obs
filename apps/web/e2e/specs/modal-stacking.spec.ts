/**
 * Real-browser regression for a genuine Stage 20E manual-test defect: an
 * unrelated `animate-fade-rise` entrance animation on the Platforms page's
 * destination cards created its own CSS stacking context that painted
 * over a non-portaled modal. The fix (`apps/web/src/components/ui/
 * Modal.tsx`) renders the modal through a `createPortal` directly under
 * `<body>` and centralizes z-index in `lib/z-layers.ts`. jsdom cannot
 * prove stacking-context/paint-order behavior at all; this spec does real
 * browser hit-testing instead of trusting DOM order.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('modal stacking over animated platform cards', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('the modal/backdrop intercepts pointer events over an underlying animated card', async ({ page }) => {
    await page.goto('/platforms');

    const twitchButton = page.getByRole('button', { name: 'Open settings for Twitch' });
    const kickButton = page.getByRole('button', { name: 'Open settings for Kick' });

    // Real coordinates of a *different* card's own control, still
    // underneath the modal once it opens - the actual hit-testing target.
    const kickBox = await kickButton.boundingBox();
    expect(kickBox).not.toBeNull();
    if (kickBox === null) return;

    await twitchButton.click();
    const dialog = page.getByRole('dialog', { name: 'Platform settings' });
    await expect(dialog).toBeVisible();

    // A real mouse click at the underlying card's own screen position.
    // If stacking were broken (card painted above the modal), this would
    // reach and activate the Kick card's own button instead of the
    // modal's backdrop.
    await page.mouse.click(kickBox.x + kickBox.width / 2, kickBox.y + kickBox.height / 2);

    // Correct behavior: the click landed on the modal's backdrop (which
    // covers the full viewport) and closed the Twitch dialog - it never
    // reached the Kick card underneath.
    await expect(dialog).toBeHidden();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('focus is trapped inside the dialog and restores to the opener on close', async ({ page }) => {
    await page.goto('/platforms');

    const opener = page.getByRole('button', { name: 'Open settings for Twitch' });
    await opener.click();

    const dialog = page.getByRole('dialog', { name: 'Platform settings' });
    await expect(dialog).toBeVisible();

    // Tab several times past the last focusable element - focus must
    // wrap back inside the dialog, never escape to the page behind it.
    for (let i = 0; i < 15; i += 1) {
      await page.keyboard.press('Tab');
      const stillInsideDialog = await dialog.evaluate((el) => el.contains(document.activeElement));
      expect(stillInsideDialog, `focus escaped the dialog after ${i + 1} Tab presses`).toBe(true);
    }

    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await expect(opener).toBeFocused();
  });
});
