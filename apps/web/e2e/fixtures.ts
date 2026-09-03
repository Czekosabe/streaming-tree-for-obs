/**
 * Shared Playwright test fixture for this suite.
 *
 * Overrides the built-in `page` fixture to add an automatic console/page
 * error gate: any `pageerror` (an uncaught exception) or `console.error`
 * call during a test fails that test, unless the message matches the
 * narrow allowlist below. This is deliberately the *only* place that
 * allowlist lives, so every spec in this suite gets the same gate for
 * free (governing task requirement: "fail tests on NEW unexpected
 * application errors").
 */
import { test as base, expect, type Page } from '@playwright/test';

/**
 * Genuinely unavoidable test-environment noise only - never a broad
 * "ignore everything" rule. Each entry must name the real, known source.
 * Matched against the failing resource's own URL (`message.location().url`)
 * - a browser-generated "Failed to load resource" console entry never
 * includes the URL in its own `.text()`.
 */
const ALLOWED_FAILED_RESOURCE_URL_PATTERNS: RegExp[] = [
  // `cmd/testserver`'s own doc comment (apps/server/cmd/testserver/
  // main.go) is explicit that it deliberately never constructs the
  // remote-management or updater subsystems - real Windows-install/OS-
  // service state this integration test twin never has. The frontend
  // already treats both probes as "feature not available" correctly
  // (AuthGate falls through to unauthenticated-not-applicable,
  // UpdateBanner simply never renders) - the console.error is Chrome's
  // own automatic logging for any non-2xx fetch response, unrelated to
  // whether the application handled it. A real auth/update regression
  // would show up as a broken UI state (AuthGate stuck on "checking", an
  // update banner in a bad state), which this suite's other assertions
  // would still catch.
  /\/api\/auth\/session$/,
  /\/api\/updates\/status$/,
  // Remote ingest is the third field cmd/testserver's own doc comment
  // names as deliberately never constructed (it requires real TLS
  // cert/key files and a configured RTMPS address - docs/remote-
  // ingest.md §8 - not something this lightweight integration twin
  // provisions). The Settings page and public overlay routes both
  // already handle its absence as "feature not configured", not a
  // crash.
  /\/api\/remote-ingest\/status$/,
  /\/api\/remote-overlay\/.+\/status$/,
];

function isAllowedConsoleText(text: string): boolean {
  // Vite's dev-server HMR client logs a connection message at startup in
  // some environments; informational, not an application error.
  return /\[vite\] connect/i.test(text);
}

/** Per-test opt-in allowances, set only by `expectFailedResource` below. */
const perTestAllowlists = new WeakMap<Page, RegExp[]>();

type Fixtures = {
  page: Page;
  /**
   * Registers a URL pattern this *specific test* deliberately provokes
   * (e.g. a test that intercepts an API route to return 500, to prove
   * the UI's own failure handling) - scoped to the current test only, so
   * it can never mask a real regression on the same endpoint in a
   * different, ordinary-usage test. Prefer the module-level allowlist
   * above instead of this for anything that isn't specific to one
   * test's own deliberately provoked scenario.
   */
  expectFailedResource: (urlPattern: RegExp) => void;
};

export const test = base.extend<Fixtures>({
  page: async ({ page }, use, testInfo) => {
    const unexpected: string[] = [];
    perTestAllowlists.set(page, []);

    page.on('pageerror', (error) => {
      unexpected.push(`pageerror: ${error.message}`);
    });
    page.on('console', (message) => {
      if (message.type() !== 'error') return;
      const text = message.text();
      if (isAllowedConsoleText(text)) return;
      if (/^Failed to load resource:/.test(text)) {
        const url = message.location().url;
        if (ALLOWED_FAILED_RESOURCE_URL_PATTERNS.some((pattern) => pattern.test(url))) return;
        const perTest = perTestAllowlists.get(page) ?? [];
        if (perTest.some((pattern) => pattern.test(url))) return;
        unexpected.push(`console.error: ${text} (${url})`);
        return;
      }
      unexpected.push(`console.error: ${text}`);
    });

    await use(page);

    // Only gate on errors when the test itself otherwise behaved as
    // expected - a test that already failed for its own reason should
    // report that failure, not a secondary "also there were console
    // errors" one that just adds noise on top of the real signal.
    if (testInfo.status === testInfo.expectedStatus) {
      expect(unexpected, `Unexpected browser error(s):\n${unexpected.join('\n')}`).toEqual([]);
    }
    perTestAllowlists.delete(page);
  },

  expectFailedResource: async ({ page }, use) => {
    await use((urlPattern: RegExp) => {
      perTestAllowlists.get(page)?.push(urlPattern);
    });
  },
});

export { expect };
