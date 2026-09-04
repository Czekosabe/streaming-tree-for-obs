/**
 * Real-browser regression coverage for the performance-hardening pass
 * (docs/development.md's "Typography and static assets" section's
 * sibling entry has the full contract). Deliberately structural/
 * network-count assertions, never a millisecond threshold - CI hardware
 * is variable, per the governing task's own explicit guidance. Timings
 * are still collected and logged as non-failing diagnostics.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('performance: route-level code splitting', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('a route module is not requested before its route is visited, and is requested once it is', async ({
    page,
  }) => {
    const scriptUrls: string[] = [];
    page.on('request', (request) => {
      if (request.resourceType() === 'script') scriptUrls.push(request.url());
    });

    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    // Dashboard itself stays eager (the first thing a returning operator
    // sees), but every other page's own module must not have been
    // requested yet - proves the lazy boundary actually defers loading,
    // not merely that it exists in source.
    const heavyRoutePatterns = [/AlertsPage/, /GoalsPage/, /SettingsPage/, /EngagementPage/, /AutomationPage/];
    for (const pattern of heavyRoutePatterns) {
      expect(
        scriptUrls.some((url) => pattern.test(url)),
        `${pattern} module must not be requested before its route is visited`,
      ).toBe(false);
    }

    scriptUrls.length = 0;
    await page.locator('aside').getByRole('link', { name: 'Alerts' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Alerts' })).toBeVisible();

    expect(
      scriptUrls.some((url) => /AlertsPage/.test(url)),
      'the Alerts route module must be requested once its route is visited',
    ).toBe(true);
    // Still never requested: an unrelated heavy route the user never visited.
    expect(scriptUrls.some((url) => /GoalsPage/.test(url))).toBe(false);
  });

  test('the persistent shell (sidebar, OBS panel) never remounts across a lazy route transition', async ({
    page,
  }) => {
    await page.goto('/');
    const sidebar = page.locator('aside');
    const scrollRegion = sidebar.getByTestId('sidebar-scroll-region');
    await expect(scrollRegion).toBeVisible();

    // A stable per-element marker: if ShellLayout ever remounted, this
    // handle would point at a destroyed node and the property would be
    // gone. `undefined` also afterward is exactly what "still the same
    // element" looks like, since JS properties don't survive real DOM
    // node replacement (an element removed from the DOM and replaced by
    // a new one loses any property set directly on the old node).
    await scrollRegion.evaluate((el) => {
      (el as unknown as { __e2eMarker: boolean }).__e2eMarker = true;
    });

    for (const routeName of ['Alerts', 'Goals', 'Settings', 'Dashboard']) {
      await sidebar.getByRole('link', { name: routeName, exact: true }).click();
      await expect(page.getByRole('heading', { level: 1, name: routeName })).toBeVisible();
      const markerSurvived = await scrollRegion.evaluate(
        (el) => (el as unknown as { __e2eMarker?: boolean }).__e2eMarker === true,
      );
      expect(markerSurvived, `ShellLayout must not remount when navigating to ${routeName}`).toBe(true);
    }
  });

  test('a route lazy-loading its own module shows an accessible, non-collapsing loading state, never a blank screen', async ({
    page,
  }) => {
    // Throttle the Alerts route module response so the Suspense fallback
    // has time to actually appear - deterministic, not a timing race,
    // since the throttle is only lifted after the fallback is confirmed
    // visible.
    let releaseResponse: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });
    await page.route(/AlertsPage/, async (route) => {
      await held;
      await route.continue();
    });

    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    await page.locator('aside').getByRole('link', { name: 'Alerts' }).click();

    const fallback = page.getByRole('status');
    await expect(fallback).toBeVisible();
    const box = await fallback.boundingBox();
    expect(box).not.toBeNull();
    if (box !== null) {
      expect(box.height, 'the loading state must not collapse the content area to zero height').toBeGreaterThan(20);
    }
    // The persistent shell must still be visible alongside the fallback -
    // never a full blank screen.
    await expect(page.locator('aside').getByTestId('sidebar-scroll-region')).toBeVisible();

    releaseResponse?.();
    await expect(page.getByRole('heading', { level: 1, name: 'Alerts' })).toBeVisible();
    await expect(fallback).toHaveCount(0);
  });
});

test.describe('performance: startup request hygiene', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('the branch-runtime endpoint is not fetched twice on a fresh Dashboard load', async ({ page }) => {
    // Real, fixed defect (docs/progress.md has the full diagnosis):
    // SystemStatusRail's cards mount before platformsQuery resolves,
    // PlatformGrid's cards only after - both call the same
    // useBranchRuntimeQuery, and a `staleTime: 0` let the later wave of
    // observers immediately re-trigger a fetch for data that was only
    // milliseconds old. Proven with real, isolated before/after evidence
    // against a real production build (docs/progress.md): 2 requests
    // unfixed, 1 fixed, repeatably. This suite runs against the Vite
    // *dev* server, where React's `<StrictMode>` (main.tsx) deliberately
    // double-invokes the whole tree's mount once, on its own, in
    // development only - a second, unrelated, well-known source of
    // exactly one extra request that even the fixed code cannot (and
    // should not) suppress. The bound below is 2, not 1, for that
    // reason; it still catches a real regression, since the specific bug
    // this test guards would add further requests on top of - not
    // instead of - StrictMode's own fixed one.
    const branchRequests: string[] = [];
    page.on('request', (request) => {
      if (request.method() === 'GET' && /\/api\/runtime\/branches$/.test(new URL(request.url()).pathname)) {
        branchRequests.push(request.url());
      }
    });

    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();
    // Let the platforms fetch (and the later wave of PlatformCard
    // mounts it triggers) fully resolve before counting.
    await expect(page.getByRole('article', { name: 'Twitch', exact: true })).toBeVisible();
    await page.waitForTimeout(500);

    expect(
      branchRequests.length,
      `expected at most 2 requests (StrictMode's own dev-only double-mount), got: ${branchRequests.join(', ')}`,
    ).toBeLessThanOrEqual(2);
  });

  test('the update-status endpoint is not re-fetched on every route navigation', async ({ page }) => {
    // Real, fixed defect: `UpdateBanner` mounts fresh inside every
    // page's own <AppShell> (unlike ShellLayout, never persistent
    // across routes) - `staleTime: 0` meant every single route change
    // re-triggered this fetch. Proven with real, isolated before/after
    // evidence against a real production build (docs/progress.md): 4
    // requests unfixed across 4 route mounts, 1 fixed. A forced 200
    // removes TanStack Query's own error-retry backoff as a confound
    // (this suite's hermetic testserver legitimately never wires the
    // updater subsystem, so an unmocked check here would 404 and retry
    // regardless of this fix - a separate, already-understood,
    // intentional testserver gap, not what this test is about). The
    // bound below is 2, not 1, for the same dev-only `<StrictMode>`
    // reason the branches test above documents - it still catches a
    // real regression, since the per-navigation bug this guards against
    // would add one further request per extra route visited, not one
    // fixed extra total.
    await page.route('**/api/updates/status', (route) =>
      route.fulfill({
        json: {
          enabled: true,
          releaseBuild: true,
          currentVersion: '1.0.0',
          autoCheck: true,
          state: 'up_to_date',
          updateAvailable: false,
          installBlocked: false,
        },
      }),
    );

    const updateRequests: string[] = [];
    page.on('request', (request) => {
      if (/\/api\/updates\/status$/.test(new URL(request.url()).pathname)) {
        updateRequests.push(request.url());
      }
    });

    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    const sidebar = page.locator('aside');
    for (const routeName of ['Platforms', 'Settings', 'Dashboard']) {
      await sidebar.getByRole('link', { name: routeName, exact: true }).click();
      await expect(page.getByRole('heading', { level: 1, name: routeName })).toBeVisible();
    }

    expect(
      updateRequests.length,
      `expected at most 2 requests (StrictMode's own dev-only double-mount) across 4 route mounts, got: ${updateRequests.join(', ')}`,
    ).toBeLessThanOrEqual(2);
  });
});

test.describe('performance: bounded navigation-churn smoke', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('repeated route navigation leaves the app interactive with no unbounded DOM growth', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    const sidebar = page.locator('aside');
    const countNodes = () => page.evaluate(() => document.querySelectorAll('*').length);

    const initialNodeCount = await countNodes();

    const routes = ['Platforms', 'Streams', 'Alerts', 'Dashboard'];
    for (let cycle = 0; cycle < 3; cycle += 1) {
      for (const routeName of routes) {
        await sidebar.getByRole('link', { name: routeName, exact: true }).click();
        await expect(page.getByRole('heading', { level: 1, name: routeName })).toBeVisible();
      }
    }

    // The app must still be genuinely interactive after the churn - a
    // real click reaching a real element, not just "no exception was
    // thrown".
    await sidebar.getByRole('link', { name: 'Settings' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible();

    const finalNodeCount = await countNodes();
    // Generous, structural bound (never a route's own real DOM should
    // accumulate copies of itself across repeated visits) - not a tight
    // pixel/byte-level threshold.
    expect(
      finalNodeCount,
      `DOM node count grew from ${initialNodeCount} to ${finalNodeCount} after repeated navigation - possible remount/leak`,
    ).toBeLessThan(initialNodeCount * 3);
  });
});
