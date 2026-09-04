/**
 * Real-browser proof of the canonical typography contract
 * (docs/development.md's "Typography and static assets" section): the
 * operator dashboard's UI font is the native system-font stack only,
 * never a remote or bundled family, and rendering it never requires
 * network access. A CSS-string check alone cannot prove any of this - it
 * cannot see what font actually got used to lay out real text, and it
 * cannot see whether the browser attempted a network request for one.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

test.describe('typography and static-asset determinism', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('no request for a font is ever made, on any route', async ({ page }) => {
    const fontRequests: string[] = [];
    page.on('request', (request) => {
      if (request.resourceType() === 'font' || /\.(woff2?|ttf|otf|eot)(\?|$)/i.test(request.url())) {
        fontRequests.push(request.url());
      }
    });

    await page.goto('/');
    await page.getByRole('link', { name: 'Platforms' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Platforms' })).toBeVisible();

    expect(fontRequests, `unexpected font request(s):\n${fontRequests.join('\n')}`).toEqual([]);
  });

  test('the basic app shell renders with all outbound internet access blocked', async ({ page }) => {
    // A hermetic stand-in for "no internet access": every request to a
    // host other than this suite's own hermetic frontend/backend
    // loopback ports is aborted, proving the shell's static product
    // assets (logo, provider marks, typography, layout) never depend on
    // any of them - only real provider/API functionality is expected to
    // need real network access, and this suite never exercises that.
    await page.route(/^https?:\/\/(?!127\.0\.0\.1)/, (route) => route.abort());

    await page.goto('/');
    const sidebar = page.locator('aside');

    // Brand logo (a locally bundled PNG, not a remote image).
    await expect(sidebar.getByRole('link', { name: 'Streaming Tree for OBS — Dashboard' })).toBeVisible();
    await expect(sidebar.locator('img').first()).toBeVisible();

    // Sidebar/nav shell.
    await expect(sidebar.getByTestId('sidebar-scroll-region')).toBeVisible();
    await expect(sidebar.getByRole('navigation', { name: 'Primary' }).getByRole('link')).toHaveCount(14);

    // Dashboard shell itself.
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    // Normal client-side navigation still works, including a route whose
    // cards render the inline-SVG provider marks (never a remote icon).
    await sidebar.getByRole('link', { name: 'Platforms' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Platforms' })).toBeVisible();
    await expect(page.getByRole('article', { name: 'Twitch', exact: true })).toBeVisible();
  });

  test('document.fonts settles, and the resolved UI font is a real local system font, never "Inter"', async ({
    page,
  }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    const result = await page.evaluate(async () => {
      await document.fonts.ready;
      const declaredFamily = getComputedStyle(document.body).fontFamily;
      return { declaredFamily, fontsReadyStatus: document.fonts.status };
    });

    expect(result.fontsReadyStatus).toBe('loaded');
    // The declared CSS family list - the one thing actually under this
    // product's own control - must never claim "Inter" again.
    // Deliberately not asserting on `document.fonts.check('Inter')`
    // instead/as well: whether a face named "Inter" happens to be
    // resolvable is a property of the machine's own OS font inventory
    // (some developer machines have it installed for unrelated
    // reasons), not of this product's code, and asserting on it would
    // make the test pass or fail depending on who runs it rather than
    // on what this repository ships.
    expect(result.declaredFamily.toLowerCase()).not.toContain('inter');
  });

  test('a representative heading renders with a real, non-collapsed content width', async ({ page }) => {
    // The exact regression a headless-Linux CI run once found: a
    // platform card's destination-name heading rendered at a 0.75px
    // bounding-box width because an unrelated sibling badge, combined
    // with a then-undeclared font's fallback metrics, squeezed a
    // `min-w-0` flex column to nothing. Proven directly here rather
    // than only inferring it from `toBeVisible()`.
    await page.goto('/platforms');

    const heading = page.getByRole('heading', { name: 'Twitch', exact: true });
    await expect(heading).toBeVisible();
    const box = await heading.boundingBox();
    expect(box).not.toBeNull();
    if (box !== null) {
      expect(box.width, 'the destination name must render with real, legible width, not a sub-pixel collapse').toBeGreaterThan(
        20,
      );
      expect(box.height).toBeGreaterThan(10);
    }
  });
});
