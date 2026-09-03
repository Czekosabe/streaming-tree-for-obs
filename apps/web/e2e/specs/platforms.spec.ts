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

    // The four seeded destinations.
    for (const name of ['Twitch', 'YouTube Live', 'Kick', 'TikTok Live']) {
      await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
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
