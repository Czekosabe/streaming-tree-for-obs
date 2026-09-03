/**
 * Real-browser regression for the Stage 20E defect fixed by lifting
 * `ShellLayout` (sidebar) above the routed `<Outlet>` so it never
 * remounts across navigation (see `apps/web/src/components/layout/
 * AppShell.tsx`'s own doc comment). A jsdom/unit test can prove the
 * sidebar's DOM node identity survives a route change; it cannot prove a
 * real browser's actual `scrollTop` survives it, because jsdom never lays
 * out or scrolls anything for real. This spec proves the real thing.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('sidebar scroll preservation', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('a scrolled sidebar keeps its scroll position across two route changes', async ({ page }) => {
    // Short enough that the fourteen-item nav genuinely overflows its
    // container - the whole point of this test is a *real* scrollable
    // region, not an assertion that would trivially pass at 0 scrollTop.
    await page.setViewportSize({ width: 1280, height: 640 });
    await page.goto('/');

    const scrollRegion = page.locator('aside').getByTestId('sidebar-scroll-region');
    await expect(scrollRegion).toBeVisible();

    const maxScroll = await scrollRegion.evaluate((el) => el.scrollHeight - el.clientHeight);
    expect(maxScroll, 'the sidebar nav region must actually overflow at this viewport height').toBeGreaterThan(20);

    await scrollRegion.evaluate((el, target) => {
      el.scrollTop = target;
    }, maxScroll);
    const scrolledTo = await scrollRegion.evaluate((el) => el.scrollTop);
    expect(scrolledTo).toBeGreaterThan(20);

    // Navigate to a route near the bottom of the nav list.
    await page.locator('aside').getByRole('link', { name: 'History' }).click();
    await expect(page).toHaveURL(/\/history$/);
    await expect(page.getByRole('heading', { level: 1, name: 'History' })).toBeVisible();

    const scrollAfterFirstNav = await scrollRegion.evaluate((el) => el.scrollTop);
    expect(
      Math.abs(scrollAfterFirstNav - scrolledTo),
      'scroll position must be preserved (within a small tolerance) across the first navigation',
    ).toBeLessThanOrEqual(5);

    // Navigate again, to a nearby item - it must still not reset to top.
    await page.locator('aside').getByRole('link', { name: 'Logs' }).click();
    await expect(page).toHaveURL(/\/logs$/);
    const scrollAfterSecondNav = await scrollRegion.evaluate((el) => el.scrollTop);
    expect(
      scrollAfterSecondNav,
      'scroll position must still not have reset to top after a second navigation',
    ).toBeGreaterThan(20);
  });
});
