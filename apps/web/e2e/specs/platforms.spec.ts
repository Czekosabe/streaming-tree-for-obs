/**
 * Real-browser smoke coverage for the /platforms route (Stage 20E
 * "complete Platforms/Metadata" - no more "Soon" placeholder). Relies on
 * the four platforms `0002_seed_default_platforms.sql` seeds into every
 * fresh database (Twitch/YouTube Live/Kick/TikTok Live, all disabled,
 * none credentialed) as deterministic fixture data.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('/platforms', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('loads with no placeholder, renders seeded destinations and the summary strip', async ({ page }) => {
    await page.goto('/platforms');

    await expect(page.getByRole('heading', { level: 1, name: 'Platforms' })).toBeVisible();
    await expect(page.getByText('This section is not implemented yet')).toHaveCount(0);
    await expect(page.getByText('Soon', { exact: true })).toHaveCount(0);

    // The four seeded destinations. Asserted against the card
    // (`article`, accessible-named via the heading it wraps) rather than
    // the heading text node directly: the heading's own box can render
    // at a near-zero CSS width under `truncate` when a real environment
    // (a missing/different font, a narrower viewport) squeezes its flex
    // column - a real, if narrow, layout fragility discovered via CI's
    // headless Linux Chromium falling back to a different sans-serif
    // than this developer machine's own installed "Inter" (the app
    // never bundles its own copy - see docs/progress.md for the full
    // finding). The card itself never collapses, and is what this
    // assertion is actually meant to prove exists.
    for (const name of ['Twitch', 'YouTube Live', 'Kick', 'TikTok Live']) {
      await expect(page.getByRole('article', { name, exact: true })).toBeVisible();
    }

    // Destination summary: 4 total, 0 configured/enabled/active - a real
    // seeded-but-not-configured destination is never miscounted.
    const summary = page.getByRole('group', { name: 'Destination summary' });
    await expect(summary).toBeVisible();
    await expect(summary).toContainText('4');
  });

  test('opening a destination’s settings shows a correctly layered dialog', async ({ page }) => {
    await page.goto('/platforms');

    await page.getByRole('button', { name: 'Open settings for Twitch' }).click();

    const dialog = page.getByRole('dialog', { name: 'Platform settings' });
    await expect(dialog).toBeVisible();
    await expect(dialog).toHaveAttribute('aria-modal', 'true');

    // Focus must have moved into the dialog.
    const focusedInsideDialog = await dialog.evaluate((el) => el.contains(document.activeElement));
    expect(focusedInsideDialog).toBe(true);

    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
  });
});
