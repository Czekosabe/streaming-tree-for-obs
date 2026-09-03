/**
 * Real-browser smoke coverage for the /metadata route (Stage 20E
 * "complete Platforms/Metadata" - no more "Soon" placeholder). Relies on
 * the same four seeded platforms as platforms.spec.ts.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('/metadata', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('loads with no placeholder and renders the current metadata surface', async ({ page }) => {
    await page.goto('/metadata');

    await expect(page.getByRole('heading', { level: 1, name: 'Metadata' })).toBeVisible();
    await expect(page.getByText('This section is not implemented yet')).toHaveCount(0);
    await expect(page.getByText('Soon', { exact: true })).toHaveCount(0);

    // One tab per seeded destination.
    const tabs = page.getByRole('tab');
    await expect(tabs).toHaveCount(4);
    for (const name of ['Twitch', 'YouTube Live', 'Kick', 'TikTok Live']) {
      await expect(page.getByRole('tab', { name: new RegExp(name) })).toBeVisible();
    }

    // Preset controls are discoverable.
    await expect(page.getByRole('button', { name: 'Presets' })).toBeVisible();
  });

  test('supports switching between destination tabs', async ({ page }) => {
    await page.goto('/metadata');

    const twitchTab = page.getByRole('tab', { name: /Twitch/ });
    const youtubeTab = page.getByRole('tab', { name: /YouTube Live/ });

    await twitchTab.click();
    await expect(twitchTab).toHaveAttribute('aria-selected', 'true');
    const twitchPanel = page.getByRole('tabpanel');
    await expect(twitchPanel).toBeVisible();

    await youtubeTab.click();
    await expect(youtubeTab).toHaveAttribute('aria-selected', 'true');
    await expect(twitchTab).toHaveAttribute('aria-selected', 'false');
    await expect(page.getByRole('tabpanel')).toBeVisible();
  });

  test('preselects the destination handed off from Platforms “Edit metadata”', async ({ page }) => {
    await page.goto('/platforms');
    await page.getByRole('button', { name: 'Edit metadata for Kick' }).click();

    await expect(page).toHaveURL(/\/metadata$/);
    await expect(page.getByRole('tab', { name: /Kick/, selected: true })).toBeVisible();
  });
});
