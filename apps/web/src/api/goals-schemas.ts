import { z } from 'zod';

/**
 * Zod contracts for the Stage 18A goals/widget-profile API
 * (`internal/httpapi/goals.go`, `internal/httpapi/public_widgets.go`).
 *
 * A goal's `current` means "events observed since its own baseline" -
 * never a provider-canonical total (docs/goals-widgets.md §1). No field
 * here ever carries a `providerEventId`, a dedupe-ledger entry, or a
 * raw event payload - the backend response shapes never carry one.
 */

export const goalKindSchema = z.enum(['followers', 'subscriptions', 'donations', 'bits']);
export type GoalKind = z.infer<typeof goalKindSchema>;

export const goalProviderSchema = z.enum(['twitch', 'youtube', 'streamelements']);
export type GoalProvider = z.infer<typeof goalProviderSchema>;

export const widgetOrientationSchema = z.enum(['horizontal', 'vertical']);
export type WidgetOrientation = z.infer<typeof widgetOrientationSchema>;

export const widgetTextAlignSchema = z.enum(['left', 'center', 'right']);
export type WidgetTextAlign = z.infer<typeof widgetTextAlignSchema>;

export const widgetFontFamilySchema = z.enum(['sans_serif', 'serif', 'monospace', 'rounded']);
export type WidgetFontFamily = z.infer<typeof widgetFontFamilySchema>;

export const goalSchema = z.object({
  id: z.string(),
  name: z.string(),
  kind: goalKindSchema,
  enabled: z.boolean(),
  target: z.number(),
  current: z.number(),
  baseline: z.number(),
  currency: z.string().optional(),
  providers: z.array(z.string()),
  accounts: z.array(z.string()),
  progressBasisPoints: z.number(),
  completed: z.boolean(),
  createdAt: z.string(),
  updatedAt: z.string(),
  startedAt: z.string(),
  configRevision: z.number(),
});
export type Goal = z.infer<typeof goalSchema>;

/** Request body for creating/updating a goal (POST/PUT). configRevision
 * is only meaningful (and only checked) on a PUT - see
 * internal/domain/goals.Service.UpdateGoal's own optimistic-concurrency
 * rule (docs/goals-widgets.md §8.1). */
export type GoalInput = {
  name: string;
  kind: GoalKind;
  enabled: boolean;
  target: number;
  baseline: number;
  currency?: string | undefined;
  providers: GoalProvider[];
  accounts: string[];
  configRevision: number;
};

export const widgetProfileSchema = z.object({
  id: z.string(),
  goalId: z.string(),
  name: z.string(),
  enabled: z.boolean(),
  publicSlug: z.string(),
  titleOverride: z.string().optional(),
  showCurrent: z.boolean(),
  showTarget: z.boolean(),
  showPercent: z.boolean(),
  orientation: widgetOrientationSchema,
  textAlign: widgetTextAlignSchema,
  fontFamily: widgetFontFamilySchema,
  backgroundColor: z.string(),
  foregroundColor: z.string(),
  fillColor: z.string(),
  borderColor: z.string(),
  borderRadiusPx: z.number(),
  opacity: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type WidgetProfile = z.infer<typeof widgetProfileSchema>;

export type WidgetProfileInput = {
  goalId: string;
  name: string;
  enabled: boolean;
  titleOverride?: string | undefined;
  showCurrent: boolean;
  showTarget: boolean;
  showPercent: boolean;
  orientation: WidgetOrientation;
  textAlign: WidgetTextAlign;
  fontFamily: WidgetFontFamily;
  backgroundColor: string;
  foregroundColor: string;
  fillColor: string;
  borderColor: string;
  borderRadiusPx: number;
  opacity: number;
};

/** The public, unauthenticated snapshot a Browser Source widget (and
 * this app's own public preview route) reads - see
 * internal/httpapi/public_widgets.go's publicWidgetSnapshotResponse.
 * Deliberately carries no id, no account/source id, and no provider
 * identity of any kind. */
export const publicWidgetPresentationSchema = z.object({
  showCurrent: z.boolean(),
  showTarget: z.boolean(),
  showPercent: z.boolean(),
  orientation: widgetOrientationSchema,
  textAlign: widgetTextAlignSchema,
  fontFamily: widgetFontFamilySchema,
  backgroundColor: z.string(),
  foregroundColor: z.string(),
  fillColor: z.string(),
  borderColor: z.string(),
  borderRadiusPx: z.number(),
  opacity: z.number(),
});
export type PublicWidgetPresentation = z.infer<typeof publicWidgetPresentationSchema>;

export const publicWidgetSnapshotSchema = z.object({
  revision: z.number(),
  kind: z.literal('goal'),
  goalKind: goalKindSchema,
  title: z.string(),
  currency: z.string().optional(),
  current: z.number(),
  target: z.number(),
  progressBasisPoints: z.number(),
  completed: z.boolean(),
  presentation: publicWidgetPresentationSchema,
});
export type PublicWidgetSnapshot = z.infer<typeof publicWidgetSnapshotSchema>;
