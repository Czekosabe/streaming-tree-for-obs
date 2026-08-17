import { z } from 'zod';

/**
 * Zod contracts for the Stage 18A/18B goals/widget-profile API
 * (`internal/httpapi/goals.go`, `internal/httpapi/public_widgets.go`).
 *
 * A goal's `current` means "events observed since its own baseline" -
 * never a provider-canonical total (docs/goals-widgets.md §1). Every
 * Stage 18B kind's own "latest"/"largest"/"recent"/"ticker"/"counter"
 * state is runtime-only, cleared on restart (docs/supporter-widgets.md
 * §3) - never persisted, never a source of truth here beyond one poll's
 * worth of data. No field here ever carries a `providerEventId`, a
 * dedupe-ledger entry, a raw event payload, or any internal widget-
 * profile id beyond the one it names itself.
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

// --- Stage 18B: widget-profile kinds (docs/supporter-widgets.md §5) ------

export const widgetProfileKindSchema = z.enum([
  'goal',
  'latest_follower',
  'latest_subscriber',
  'latest_donation',
  'largest_donation',
  'recent_supporters',
  'event_ticker',
  'session_counter',
  'dashboard',
]);
export type WidgetProfileKind = z.infer<typeof widgetProfileKindSchema>;

export const sessionMetricSchema = z.enum([
  'follows',
  'new_subscriptions',
  'resubscriptions',
  'gifted_subscriptions',
  'raids',
  'bits_quantity',
  'support_event_count',
  'support_amount',
]);
export type SessionMetric = z.infer<typeof sessionMetricSchema>;

export const supporterEventTypeSchema = z.enum([
  'follow',
  'subscription',
  'resubscription',
  'gifted_subscription',
  'subscription_gift_batch',
  'bits',
  'raid',
  'donation',
  'youtube_super_chat',
  'youtube_super_sticker',
  'youtube_membership',
  'youtube_membership_milestone',
]);
export type SupporterEventType = z.infer<typeof supporterEventTypeSchema>;

export const dashboardChildSchema = z.object({
  widgetProfileId: z.string(),
  column: z.number(),
  columnSpan: z.number(),
  row: z.number(),
  rowSpan: z.number(),
});
export type DashboardChild = z.infer<typeof dashboardChildSchema>;

export const widgetProfileSchema = z.object({
  id: z.string(),
  kind: widgetProfileKindSchema,
  goalId: z.string().optional(),
  name: z.string(),
  enabled: z.boolean(),
  publicSlug: z.string(),
  providers: z.array(z.string()).optional(),
  accounts: z.array(z.string()).optional(),
  titleOverride: z.string().optional(),
  showCurrent: z.boolean(),
  showTarget: z.boolean(),
  showPercent: z.boolean(),
  showProvider: z.boolean().optional(),
  showTime: z.boolean().optional(),
  showMessage: z.boolean().optional(),
  maxItems: z.number().optional(),
  currency: z.string().optional(),
  metric: sessionMetricSchema.optional(),
  eventTypes: z.array(supporterEventTypeSchema).optional(),
  columns: z.number().optional(),
  children: z.array(dashboardChildSchema).optional(),
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

/** Request body for creating/updating a widget profile. Kind and (for a
 * goal widget) goalId are immutable after creation - the backend simply
 * ignores a changed value for either on a PUT (docs/supporter-
 * widgets.md §5, §16). Only the fields relevant to `kind` are ever
 * validated as non-empty/non-zero by the backend. */
export type WidgetProfileInput = {
  kind: WidgetProfileKind;
  goalId?: string | undefined;
  name: string;
  enabled: boolean;
  providers: GoalProvider[];
  accounts: string[];
  titleOverride?: string | undefined;
  showCurrent: boolean;
  showTarget: boolean;
  showPercent: boolean;
  showProvider: boolean;
  showTime: boolean;
  showMessage: boolean;
  maxItems: number;
  currency?: string | undefined;
  metric?: SessionMetric | undefined;
  eventTypes: SupporterEventType[];
  columns: number;
  children: DashboardChild[];
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

// --- runtime status / public presentation items ---------------------------

export const supporterItemSchema = z.object({
  itemId: z.string(),
  displayName: z.string().optional(),
  provider: z.string().optional(),
  amountMicros: z.number().optional(),
  currency: z.string().optional(),
  quantity: z.number().optional(),
  message: z.string().optional(),
  observedAt: z.string(),
});
export type SupporterItem = z.infer<typeof supporterItemSchema>;

export const tickerItemSchema = supporterItemSchema.extend({ eventType: supporterEventTypeSchema });
export type TickerItem = z.infer<typeof tickerItemSchema>;

export const runtimeStatusSchema = z.object({
  kind: widgetProfileKindSchema,
  latest: supporterItemSchema.optional(),
  largest: supporterItemSchema.optional(),
  recent: z.array(supporterItemSchema).optional(),
  ticker: z.array(tickerItemSchema).optional(),
  counter: z.number().optional(),
});
export type RuntimeStatus = z.infer<typeof runtimeStatusSchema>;

/** The public, unauthenticated snapshot a Browser Source widget (and
 * this app's own public preview route) reads - see
 * internal/httpapi/public_widgets.go's publicWidgetSnapshotResponse.
 * Deliberately carries no id, no account/source id, and no provider
 * identity of any kind. A discriminated union by `kind`; every field
 * outside the goal-kind ones is optional, present only for the one kind
 * that owns it. */
export const publicWidgetPresentationSchema = z.object({
  showCurrent: z.boolean(),
  showTarget: z.boolean(),
  showPercent: z.boolean(),
  showProvider: z.boolean().optional(),
  showTime: z.boolean().optional(),
  columns: z.number().optional(),
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

export interface PublicWidgetSnapshot {
  revision: number;
  kind: WidgetProfileKind;
  title: string;
  // --- goal (Stage 18A) ---
  goalKind?: GoalKind | undefined;
  currency?: string | undefined;
  current?: number | undefined;
  target?: number | undefined;
  progressBasisPoints?: number | undefined;
  completed?: boolean | undefined;
  // --- Stage 18B ---
  latest?: SupporterItem | undefined;
  largest?: SupporterItem | undefined;
  recent?: SupporterItem[] | undefined;
  ticker?: TickerItem[] | undefined;
  counter?: number | undefined;
  dashboard?: PublicDashboardChild[] | undefined;
  presentation: PublicWidgetPresentation;
}

export interface PublicDashboardChild {
  key: string;
  column: number;
  columnSpan: number;
  row: number;
  rowSpan: number;
  snapshot: PublicWidgetSnapshot;
}

export const publicWidgetSnapshotSchema: z.ZodType<PublicWidgetSnapshot> = z.lazy(() =>
  z.object({
    revision: z.number(),
    kind: widgetProfileKindSchema,
    title: z.string(),
    goalKind: goalKindSchema.optional(),
    currency: z.string().optional(),
    current: z.number().optional(),
    target: z.number().optional(),
    progressBasisPoints: z.number().optional(),
    completed: z.boolean().optional(),
    latest: supporterItemSchema.optional(),
    largest: supporterItemSchema.optional(),
    recent: z.array(supporterItemSchema).optional(),
    ticker: z.array(tickerItemSchema).optional(),
    counter: z.number().optional(),
    dashboard: z.array(publicDashboardChildSchema).optional(),
    presentation: publicWidgetPresentationSchema,
  }),
);

const publicDashboardChildSchema: z.ZodType<PublicDashboardChild> = z.lazy(() =>
  z.object({
    key: z.string(),
    column: z.number(),
    columnSpan: z.number(),
    row: z.number(),
    rowSpan: z.number(),
    snapshot: publicWidgetSnapshotSchema,
  }),
);
