/**
 * Real-browser regression for the brand/logo → Dashboard link: a real,
 * keyboard-focusable `<a>` (via react-router `Link`), not a clickable
 * `div`, and a client-side route change (never a full page reload) that
 * works regardless of the sidebar's current scroll position.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('brand/home navigation', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('is a real semantic, keyboard-focusable link with the expected accessible name', async ({ page }) => {
    await page.goto('/settings');

    const brandLink = page.locator('aside').getByRole('link', { name: 'Streaming Tree for OBS — Dashboard' });
    await expect(brandLink).toBeVisible();
    await expect(brandLink).toHaveAttribute('href', '/');

    await brandLink.focus();
    await expect(brandLink).toBeFocused();
  });

  test('navigates from a non-Dashboard route to Dashboard via client-side routing, even while scrolled', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 640 });
    await page.goto('/history');
    await expect(page.getByRole('heading', { level: 1, name: 'History' })).toBeVisible();

    const sidebar = page.locator('aside');
    const scrollRegion = sidebar.getByTestId('sidebar-scroll-region');
    await scrollRegion.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    const scrolledTo = await scrollRegion.evaluate((el) => el.scrollTop);
    expect(scrolledTo).toBeGreaterThan(0);

    // A real client-side navigation never triggers a full document
    // reload - proven by tagging `window` before the click and checking
    // the tag survived after.
    await page.evaluate(() => {
      (window as unknown as { __e2eMarker: boolean }).__e2eMarker = true;
    });

    await sidebar.getByRole('link', { name: 'Streaming Tree for OBS — Dashboard' }).click();

    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    const markerSurvived = await page.evaluate(
      () => (window as unknown as { __e2eMarker?: boolean }).__e2eMarker === true,
    );
    expect(markerSurvived, 'brand navigation must be a client-side route change, not a full page reload').toBe(true);

    // ShellLayout state (the sidebar itself) must still be intact - not
    // broken/unmounted by the navigation.
    await expect(sidebar.getByTestId('sidebar-scroll-region')).toBeVisible();
  });
});
