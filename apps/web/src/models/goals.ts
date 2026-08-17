import type { TFunction } from 'i18next';

import type {
  DashboardChild,
  GoalInput,
  GoalKind,
  SessionMetric,
  SupporterEventType,
  WidgetProfileInput,
  WidgetProfileKind,
} from '@/api/goals-schemas';
import { ApiError } from '@/lib/api-client';

/** Maps a Stage 18A API error onto its own translated message - kept
 * here rather than in a component file so components/goals/GoalManager.tsx
 * and WidgetProfileManager.tsx can both import it without either one
 * exporting a non-component value (react-refresh/only-export-components -
 * see docs/progress.md's Stage 17B frontend entry for the identical
 * fix applied to RuleManager.tsx's own draftFromRule). */
export function errorMessage(t: TFunction<'goals'>, error: unknown): string {
  if (error instanceof ApiError && error.code !== null) {
    const key = `errors.${error.code}` as never;
    const translated = t(key, { defaultValue: '' });
    return translated === '' ? t('errors.generic') : translated;
  }
  return t('errors.generic');
}

/**
 * Client-side mirrors of the Stage 18A bounds/validation rules in
 * `internal/domain/goals/validation.go` - the backend remains
 * authoritative; these exist only so the form can disable Save before a
 * round trip that would fail anyway (docs/goals-widgets.md §8.2).
 */

export const MAX_NAME_CODE_POINTS = 80;

/** Mirrors goals.MaxGoalCountValue - followers/subscriptions/Bits. */
export const MAX_GOAL_COUNT_VALUE = 100_000_000;

/** Mirrors goals.MaxGoalAmountMicros - donation goals, integer micros. */
export const MAX_GOAL_AMOUNT_MICROS = 100_000_000_000_000;

export const GOAL_KINDS: readonly GoalKind[] = ['followers', 'subscriptions', 'donations', 'bits'];

/** Providers a goal may filter on - mirrors internal/domain/goals's own
 * closed ProviderID set exactly (docs/goals-widgets.md §3, §14). */
export const GOAL_PROVIDERS = ['twitch', 'youtube', 'streamelements'] as const;

export function isValidGoalName(name: string): boolean {
  const n = [...name].length;
  return n >= 1 && n <= MAX_NAME_CODE_POINTS;
}

function maxValueFor(kind: GoalKind): number {
  return kind === 'donations' ? MAX_GOAL_AMOUNT_MICROS : MAX_GOAL_COUNT_VALUE;
}

export function isValidGoalTarget(kind: GoalKind, target: number): boolean {
  return Number.isInteger(target) && target > 0 && target <= maxValueFor(kind);
}

export function isValidGoalValue(kind: GoalKind, value: number): boolean {
  return Number.isInteger(value) && value >= 0 && value <= maxValueFor(kind);
}

/** A plain uppercase-ASCII currency-code shape check - mirrors
 * goals.validCurrencyCode exactly (3-8 characters, A-Z only). Never
 * validated against an external ISO-4217 registry. */
export function isValidCurrencyCode(code: string): boolean {
  return /^[A-Z]{3,8}$/.test(code);
}

export function normalizeCurrencyCode(currency: string): string {
  return currency.trim().toUpperCase();
}

export function isValidGoalCurrency(kind: GoalKind, currency: string | undefined): boolean {
  if (kind === 'donations') return currency !== undefined && isValidCurrencyCode(currency);
  return currency === undefined || currency === '';
}

export function emptyGoalDraft(kind: GoalKind = 'followers'): GoalInput {
  return {
    name: '', kind, enabled: true, target: kind === 'donations' ? 100_000_000 : 100,
    baseline: 0, currency: kind === 'donations' ? 'USD' : undefined,
    providers: [], accounts: [], configRevision: 0,
  };
}

export const WIDGET_ORIENTATIONS = ['horizontal', 'vertical'] as const;
export const WIDGET_TEXT_ALIGNS = ['left', 'center', 'right'] as const;
export const WIDGET_FONT_FAMILIES = ['sans_serif', 'serif', 'monospace', 'rounded'] as const;

export const DEFAULT_WIDGET_BORDER_RADIUS_PX = 12;
export const MAX_WIDGET_BORDER_RADIUS_PX = 32;

/** A bounded "#RRGGBB" or "#RRGGBBAA" shape check - mirrors
 * goals.validHexColor exactly. Never an arbitrary CSS color function or
 * keyword. */
export function isValidHexColor(value: string): boolean {
  return /^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(value);
}

// --- Stage 18B: widget-profile kinds (docs/supporter-widgets.md §5) ------
// Client-side mirrors of internal/domain/goals/widget_kinds.go and
// validation.go's own kind-specific rules - the backend remains
// authoritative.

export const WIDGET_PROFILE_KINDS: readonly WidgetProfileKind[] = [
  'goal',
  'latest_follower',
  'latest_subscriber',
  'latest_donation',
  'largest_donation',
  'recent_supporters',
  'event_ticker',
  'session_counter',
  'dashboard',
];

/** Kinds a management "Widgets" list shows (everything but goal, which
 * has its own per-goal management UI, and dashboard, which has its own
 * "Dashboards" list). */
export const SUPPORTER_WIDGET_KINDS: readonly WidgetProfileKind[] = [
  'latest_follower',
  'latest_subscriber',
  'latest_donation',
  'largest_donation',
  'recent_supporters',
  'event_ticker',
  'session_counter',
];

export const SESSION_METRICS: readonly SessionMetric[] = [
  'follows',
  'new_subscriptions',
  'resubscriptions',
  'gifted_subscriptions',
  'raids',
  'bits_quantity',
  'support_event_count',
  'support_amount',
];

export const SUPPORTER_EVENT_TYPES: readonly SupporterEventType[] = [
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
];

export function widgetKindRequiresGoal(kind: WidgetProfileKind): boolean {
  return kind === 'goal';
}
export function widgetKindIsDashboard(kind: WidgetProfileKind): boolean {
  return kind === 'dashboard';
}
export function widgetKindHasOwnFilters(kind: WidgetProfileKind): boolean {
  return !widgetKindRequiresGoal(kind) && !widgetKindIsDashboard(kind);
}
export function widgetKindRequiresMaxItems(kind: WidgetProfileKind): boolean {
  return kind === 'recent_supporters' || kind === 'event_ticker';
}
export function widgetKindRequiresCurrency(kind: WidgetProfileKind): boolean {
  return kind === 'largest_donation';
}

export const MIN_MAX_ITEMS = 1;
export const MAX_RECENT_SUPPORTERS = 20;
export const DEFAULT_MAX_ITEMS = 5;
export const MAX_EVENT_TICKER_ITEMS = 50;
export const DEFAULT_TICKER_MAX_ITEMS = 10;

export const MIN_DASHBOARD_COLUMNS = 1;
export const MAX_DASHBOARD_COLUMNS = 4;
export const MIN_DASHBOARD_CHILDREN = 1;
export const MAX_DASHBOARD_CHILDREN = 8;

function maxItemsBoundFor(kind: WidgetProfileKind): number {
  return kind === 'event_ticker' ? MAX_EVENT_TICKER_ITEMS : MAX_RECENT_SUPPORTERS;
}

/** Validates every bound/enum/required-field rule
 * `internal/domain/goals.ValidateWidgetProfileFields` checks, except
 * goalId/dashboard-child existence (only the backend can answer that).
 * Used to disable Save before a round trip that would fail anyway. */
export function isValidWidgetProfileFields(input: WidgetProfileInput): boolean {
  if (
    !isValidGoalName(input.name) ||
    (input.titleOverride !== undefined && [...input.titleOverride].length > MAX_NAME_CODE_POINTS) ||
    !isValidHexColor(input.backgroundColor) ||
    !isValidHexColor(input.foregroundColor) ||
    !isValidHexColor(input.fillColor) ||
    !isValidHexColor(input.borderColor) ||
    input.borderRadiusPx < 0 ||
    input.borderRadiusPx > MAX_WIDGET_BORDER_RADIUS_PX ||
    input.opacity < 0 ||
    input.opacity > 1
  ) {
    return false;
  }

  if (widgetKindRequiresGoal(input.kind)) {
    if (!input.goalId) return false;
  } else if (input.goalId) {
    return false;
  }

  if (widgetKindHasOwnFilters(input.kind)) {
    if (new Set(input.accounts).size !== input.accounts.length) return false;
  } else if (input.providers.length > 0 || input.accounts.length > 0) {
    return false;
  }

  if (widgetKindRequiresMaxItems(input.kind)) {
    const max = maxItemsBoundFor(input.kind);
    if (!Number.isInteger(input.maxItems) || input.maxItems < MIN_MAX_ITEMS || input.maxItems > max) return false;
  } else if (input.maxItems !== 0) {
    return false;
  }

  if (input.kind === 'session_counter') {
    if (input.metric === undefined) return false;
    if (input.metric === 'support_amount') {
      if (!input.currency || !isValidCurrencyCode(input.currency)) return false;
    } else if (input.currency) {
      return false;
    }
  } else {
    if (input.metric !== undefined) return false;
    if (widgetKindRequiresCurrency(input.kind)) {
      if (!input.currency || !isValidCurrencyCode(input.currency)) return false;
    } else if (input.currency) {
      return false;
    }
  }

  if (input.kind === 'event_ticker') {
    if (new Set(input.eventTypes).size !== input.eventTypes.length) return false;
  } else if (input.eventTypes.length > 0) {
    return false;
  }

  if (widgetKindIsDashboard(input.kind)) {
    if (input.columns < MIN_DASHBOARD_COLUMNS || input.columns > MAX_DASHBOARD_COLUMNS) return false;
    if (input.children.length < MIN_DASHBOARD_CHILDREN || input.children.length > MAX_DASHBOARD_CHILDREN) return false;
    const seen = new Set<string>();
    for (const c of input.children) {
      if (!c.widgetProfileId || seen.has(c.widgetProfileId)) return false;
      seen.add(c.widgetProfileId);
      if (c.column < 1 || c.columnSpan < 1 || c.column + c.columnSpan - 1 > input.columns) return false;
      if (c.row < 1 || c.rowSpan < 1) return false;
    }
  } else if (input.columns !== 0 || input.children.length > 0) {
    return false;
  }

  return true;
}

/** Default draft for a KindGoal widget profile (Stage 18A) - kept as
 * its own function for every existing call site; see
 * defaultWidgetProfileDraftOfKind for every Stage 18B kind. */
export function defaultWidgetProfileDraft(goalId: string): WidgetProfileInput {
  return defaultWidgetProfileDraftOfKind('goal', goalId);
}

export function defaultWidgetProfileDraftOfKind(kind: WidgetProfileKind, goalId?: string): WidgetProfileInput {
  return {
    kind,
    goalId: widgetKindRequiresGoal(kind) ? goalId : undefined,
    name: '',
    enabled: true,
    providers: [],
    accounts: [],
    showCurrent: true,
    showTarget: true,
    showPercent: true,
    showProvider: true,
    showTime: true,
    showMessage: false,
    maxItems: widgetKindRequiresMaxItems(kind) ? (kind === 'event_ticker' ? DEFAULT_TICKER_MAX_ITEMS : DEFAULT_MAX_ITEMS) : 0,
    currency: widgetKindRequiresCurrency(kind) ? 'USD' : undefined,
    metric: kind === 'session_counter' ? 'follows' : undefined,
    eventTypes: kind === 'event_ticker' ? ['follow'] : [],
    columns: widgetKindIsDashboard(kind) ? MIN_DASHBOARD_COLUMNS : 0,
    children: [],
    orientation: 'horizontal',
    textAlign: 'center',
    fontFamily: 'sans_serif',
    backgroundColor: '#00000080',
    foregroundColor: '#ffffff',
    fillColor: '#7c3aed',
    borderColor: '#ffffff33',
    borderRadiusPx: DEFAULT_WIDGET_BORDER_RADIUS_PX,
    opacity: 1.0,
  };
}

export function emptyDashboardChild(widgetProfileId: string): DashboardChild {
  return { widgetProfileId, column: 1, columnSpan: 1, row: 1, rowSpan: 1 };
}
