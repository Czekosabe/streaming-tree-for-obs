/**
 * Bounded smoke pass across every current, user-reachable route inside
 * `ShellLayout` - built directly from the real route table
 * (`NAV_ITEMS`/`navigation.json`), not a hand-maintained list, so this
 * test can never silently drift out of sync with the actual navigation.
 *
 * Deliberately excluded (documented, not merely omitted):
 * - `/overlay/chat|alerts|audio/widgets/:publicSlug` - standalone OBS
 *   Browser Source pages with no ShellLayout chrome, requiring a real
 *   seeded public overlay slug and (for chat) a live SSE/WS source to
 *   render non-empty content; out of scope for a bounded app-navigation
 *   smoke pass (docs/obs-browser-source.md is the real contract for
 *   these routes).
 * - `/alerts/rules/:ruleId/designer` and `/overlays/:overlayId/designer`
 *   - standalone full-viewport editor pages that require a real,
 *   already-selected rule/overlay id; not reachable from a fresh smoke
 *   pass without first creating one, which is a deeper flow than this
 *   bounded matrix is meant to cover.
 * - `/settings/about` is included below (reachable via a real in-app
 *   link from Settings) but, being outside the primary nav list, is
 *   checked only for "loads, no crash, no placeholder" rather than an
 *   exact heading match.
 */
import navigationEn from '../../src/i18n/resources/en/navigation.json' with { type: 'json' };
import { NAV_ITEMS } from '../../src/components/layout/nav-items';
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

type NavItemLabels = Record<string, string>;
const ITEM_LABELS = (navigationEn as { items: NavItemLabels }).items;

test.describe('route smoke matrix', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('every current primary route loads via sidebar navigation with the expected heading', async ({ page }) => {
    await page.goto('/');

    for (const item of NAV_ITEMS) {
      const key = item.labelKey.replace(/^items\./, '');
      const label = ITEM_LABELS[key];
      expect(label, `no navigation.json label for nav item "${item.labelKey}"`).toBeTruthy();

      await page.locator('aside').getByRole('link', { name: label, exact: true }).click();
      await expect(page).toHaveURL(item.to === '/' ? /\/$/ : new RegExp(`${item.to}$`));
      await expect(page.getByRole('heading', { level: 1, name: label, exact: true })).toBeVisible();
      await expect(page.getByText('This section is not implemented yet')).toHaveCount(0);
    }
  });

  test('/settings/about loads without a crash or placeholder', async ({ page }) => {
    await page.goto('/settings');
    await page.getByRole('link', { name: 'About & Legal' }).click();
    await expect(page).toHaveURL(/\/settings\/about$/);
    await expect(page.getByRole('heading', { level: 1, name: 'About & Legal' })).toBeVisible();
    await expect(page.getByText('This section is not implemented yet')).toHaveCount(0);
  });

  test('an unknown route renders NotFoundPage rather than a blank page or crash', async ({ page }) => {
    await page.goto('/this-route-does-not-exist');
    await expect(page.getByRole('heading', { level: 1, name: 'Not found' })).toBeVisible();
    await expect(page.getByRole('link', { name: /back to dashboard/i })).toBeVisible();
  });
});
