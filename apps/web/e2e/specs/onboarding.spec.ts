/**
 * Real-browser coverage of the first-run onboarding flow, using the real
 * backend (`PUT /api/onboarding`) to deterministically reset status
 * before each test - never relying on a genuinely fresh database, since
 * other specs in this suite may already have completed onboarding
 * against the same shared backend.
 *
 * Covers both the success path (real completion write, real navigation,
 * the Dashboard banner disappearing) and the failure path (an
 * intercepted, deterministic 500 - never a real network failure), per
 * the governing task's explicit requirement that both be deterministic.
 */
import { setOnboardingStatus } from '../helpers/backend-api';
import { expect, test } from '../fixtures';

const STEP_COUNT = 7;

test.describe('onboarding flow', () => {
  test.beforeEach(async () => {
    // 'dismissed', not 'pending': OnboardingAutoRedirect
    // (apps/web/src/components/onboarding/OnboardingAutoRedirect.tsx)
    // immediately bounces a fresh page load away from any route to
    // `/onboarding` whenever status is 'pending', which would make a
    // direct `page.goto('/')` below never actually reach Dashboard.
    // 'dismissed' is a real, common starting state (an operator who
    // clicked "Skip setup" earlier) that is not auto-redirected, so the
    // banner and the completion flow can both be exercised directly.
    await setOnboardingStatus('dismissed');
  });

  test('completing the flow navigates to Dashboard and clears the setup-incomplete banner', async ({ page }) => {
    // Before completion: the Dashboard shows the "Setup incomplete" banner.
    await page.goto('/');
    await expect(page.getByText('Setup incomplete')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Continue setup' })).toBeVisible();

    await page.goto('/onboarding');
    await expect(page.getByRole('heading', { level: 2 })).toBeVisible();
    await expect(page.getByText('Step 1 of 7')).toBeVisible();

    for (let step = 1; step < STEP_COUNT; step += 1) {
      await page.getByRole('button', { name: 'Continue' }).click();
      await expect(page.getByText(`Step ${step + 1} of ${STEP_COUNT}`)).toBeVisible();
    }

    // Deterministic, seeded-but-not-configured destination summary: 4
    // seeded platforms, none credentialed/enabled/active - never falsely
    // counted as "configured" merely for existing.
    await expect(page.getByText('4 destinations, 0 configured, 0 enabled, 0 active')).toBeVisible();

    await page.getByRole('button', { name: 'Go to Dashboard' }).click();

    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

    // The banner must be gone now that status is genuinely 'completed'.
    await expect(page.getByText('Setup incomplete')).toHaveCount(0);
  });

  test('a failed completion write stays on the page and shows a retryable error', async ({
    page,
    expectFailedResource,
  }) => {
    // This test deliberately provokes a real 500 below - Chrome logs a
    // console.error for the failed fetch regardless of the app handling
    // it correctly (asserted below). Scoped to this test only.
    expectFailedResource(/\/api\/onboarding$/);

    await page.goto('/onboarding');

    for (let step = 1; step < STEP_COUNT; step += 1) {
      await page.getByRole('button', { name: 'Continue' }).click();
    }
    await expect(page.getByText(`Step ${STEP_COUNT} of ${STEP_COUNT}`)).toBeVisible();

    // A deterministic, forced failure - never a real network outage.
    await page.route('**/api/onboarding', async (route) => {
      if (route.request().method() === 'PUT') {
        await route.fulfill({ status: 500, json: { error: 'internal_error', message: 'forced by e2e test' } });
        return;
      }
      await route.continue();
    });

    await page.getByRole('button', { name: 'Go to Dashboard' }).click();

    await expect(page.getByRole('alert')).toHaveText('Something went wrong saving your progress. Try again.');
    // Must NOT have navigated away on a failed persist.
    await expect(page).toHaveURL(/\/onboarding$/);
  });

  test('a genuinely fresh (pending) install auto-opens the assistant on first load', async ({ page }) => {
    await setOnboardingStatus('pending');
    await page.goto('/');
    await expect(page).toHaveURL(/\/onboarding$/);
    await expect(page.getByRole('heading', { level: 2, name: 'Welcome to Streaming Tree for OBS' })).toBeVisible();
  });
});
