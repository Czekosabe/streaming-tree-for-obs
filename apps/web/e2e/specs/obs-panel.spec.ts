/**
 * Real-browser regression for the collapsible OBS connection panel
 * (`SidebarFooter`, Stage 20E defect B): collapsed by default, a real
 * error always stays visible regardless of collapse state, and expansion
 * state survives route navigation because `ShellLayout` never remounts it.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

const FORCED_ERROR_MESSAGE = 'Forced error for the E2E disclosure test.';

/** A schema-valid GET /api/runtime payload with a forced mediaMtx error. */
const ERROR_RUNTIME_SNAPSHOT = {
  version: 1,
  mediaMtx: {
    supportedVersion: '1.0.0',
    installedVersion: '1.0.0',
    source: 'managed',
    state: 'error',
    autoStart: true,
    autoRestart: true,
    restartCount: 0,
    lastError: { code: 'e2e_forced_error', message: FORCED_ERROR_MESSAGE },
  },
  ingest: {
    state: 'unavailable',
    path: 'live',
    trackCount: null,
    tracks: [],
  },
  connection: {
    serverUrl: 'rtmp://127.0.0.1:1935/live',
    streamKey: 'e2e-test',
    publishUrl: 'rtmp://127.0.0.1:1935/live/e2e-test',
  },
};

test.describe('OBS connection disclosure', () => {
  test.beforeEach(async () => {
    await setOnboardingStatus('completed');
  });

  test('collapsed by default, status visible, expands and collapses correctly', async ({ page }) => {
    await page.goto('/');

    const sidebar = page.locator('aside');
    const toggle = sidebar.getByRole('button', { name: /connection details/i });
    const footer = sidebar.getByTestId('sidebar-footer');

    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    // The compact status summary (heading + live status dot/text) is a
    // sibling of the collapsible `details` region, so it stays visible
    // even while collapsed.
    const footerTextWhileCollapsed = (await footer.innerText()).trim();
    expect(footerTextWhileCollapsed.length, 'a compact status summary must be visible while collapsed').toBeGreaterThan(
      0,
    );

    const detailsId = await toggle.getAttribute('aria-controls');
    expect(detailsId).not.toBeNull();
    const details = page.locator(`#${detailsId}`);
    await expect(details).toBeHidden();

    // Mouse activation expands it.
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(toggle).toHaveAccessibleName(/hide connection details/i);
    await expect(details).toBeVisible();

    // Collapsing hides only the detail controls - the compact status line
    // (asserted above) is a sibling outside `details`, so it is never
    // touched by `hidden`.
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect(details).toBeHidden();
    const footerTextAfterCollapse = (await footer.innerText()).trim();
    expect(footerTextAfterCollapse.length, 'status summary must remain visible after collapsing again').toBeGreaterThan(
      0,
    );
  });

  test('keyboard activation toggles the disclosure', async ({ page }) => {
    await page.goto('/');

    const toggle = page.locator('aside').getByRole('button', { name: /connection details/i });
    await toggle.focus();
    await expect(toggle).toBeFocused();

    await page.keyboard.press('Enter');
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');

    await page.keyboard.press('Space');
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  });

  test('a real error stays visible even while collapsed, and survives route navigation', async ({ page }) => {
    await page.route('**/api/runtime', async (route) => {
      await route.fulfill({ json: ERROR_RUNTIME_SNAPSHOT });
    });

    await page.goto('/');

    const sidebar = page.locator('aside');
    const toggle = sidebar.getByRole('button', { name: /connection details/i });
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');

    // The error text is always visible, even collapsed.
    await expect(sidebar.getByText(FORCED_ERROR_MESSAGE)).toBeVisible();

    // Expand, then navigate - expansion state is component state inside
    // ShellLayout, which never remounts across a route change.
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');

    await sidebar.getByRole('link', { name: 'Settings' }).click();
    await expect(page).toHaveURL(/\/settings$/);

    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(sidebar.getByText(FORCED_ERROR_MESSAGE)).toBeVisible();
  });
});
