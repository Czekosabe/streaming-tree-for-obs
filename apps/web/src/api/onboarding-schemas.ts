import { z } from 'zod';

/**
 * Zod contract for `GET`/`PUT /api/onboarding` (docs/onboarding.md §4.4).
 *
 * The payload is versioned, matching every other backend contract in this
 * codebase - a backend that changes the shape in an incompatible way is
 * rejected outright rather than rendering half a screen.
 */

/** Schema version this build understands. */
export const ONBOARDING_SCHEMA_VERSION = 1;

export const ONBOARDING_STATUSES = ['pending', 'completed', 'dismissed'] as const;

export type OnboardingStatus = (typeof ONBOARDING_STATUSES)[number];

export const onboardingStateSchema = z.object({
  version: z.number().int(),
  status: z.enum(ONBOARDING_STATUSES),
  schemaVersion: z.number().int(),
});

export type OnboardingState = z.infer<typeof onboardingStateSchema>;
