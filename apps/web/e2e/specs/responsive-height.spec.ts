/**
 * Real-browser layout checks at the exact viewport heights a prior Stage
 * 20E manual pass could only reason about by reading source
 * (900px/768px/600px) - now backed by real browser geometry instead.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

const HEIGHTS = [900, 768, 600] as const;
const DESKTOP_WIDTH = 1280;

test.describe('sidebar layout at representative viewport heights', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  for (const height of HEIGHTS) {
    test(`navigation stays reachable and non-overlapping at ${DESKTOP_WIDTH}x${height}`, async ({ page }) => {
      await page.setViewportSize({ width: DESKTOP_WIDTH, height });
      await page.goto('/');

      const sidebar = page.locator('aside');
      const scrollRegion = sidebar.getByTestId('sidebar-scroll-region');
      const footer = sidebar.getByTestId('sidebar-footer');

      await expect(scrollRegion).toBeVisible();
      await expect(footer).toBeVisible();

      // No overlap: the scrollable nav region's bottom edge must never sit
      // below the footer's top edge, at any of these heights.
      const [regionBox, footerBox] = await Promise.all([scrollRegion.boundingBox(), footer.boundingBox()]);
      expect(regionBox).not.toBeNull();
      expect(footerBox).not.toBeNull();
      if (regionBox !== null && footerBox !== null) {
        expect(regionBox.y + regionBox.height, 'nav region must not overlap the OBS panel/footer below it').toBeLessThanOrEqual(
          footerBox.y + 1,
        );
      }

      // Every nav item must be reachable - either directly visible, or
      // reachable by scrolling the nav region (never permanently
      // inaccessible, never requiring the whole page/body to scroll).
      const links = sidebar.getByRole('navigation', { name: 'Primary' }).getByRole('link');
      const count = await links.count();
      expect(count).toBe(14);
      for (let i = 0; i < count; i += 1) {
        await links.nth(i).scrollIntoViewIfNeeded();
        await expect(links.nth(i)).toBeVisible();
      }

      // No horizontal overflow at any of these heights (a common
      // consequence of a broken height budget).
      const hasHorizontalOverflow = await page.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
      );
      expect(hasHorizontalOverflow).toBe(false);
    });
  }

  test('collapsing the OBS panel materially increases navigation space', async ({ page }) => {
    await page.setViewportSize({ width: DESKTOP_WIDTH, height: 700 });
    await page.goto('/');

    const sidebar = page.locator('aside');
    const scrollRegion = sidebar.getByTestId('sidebar-scroll-region');
    const toggle = sidebar.getByRole('button', { name: /connection details/i });

    // Collapsed by default (Stage 20E defect B contract).
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    const collapsedHeight = await scrollRegion.evaluate((el) => el.clientHeight);

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    const expandedHeight = await scrollRegion.evaluate((el) => el.clientHeight);

    expect(
      collapsedHeight,
      'collapsing the OBS connection panel must materially increase the space available for navigation',
    ).toBeGreaterThan(expandedHeight + 20);
  });
});
