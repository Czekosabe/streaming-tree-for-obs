import type { TFunction } from 'i18next';

import type { GoalInput, GoalKind, WidgetProfileInput } from '@/api/goals-schemas';
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

export function isValidWidgetProfileFields(input: WidgetProfileInput): boolean {
  return (
    isValidGoalName(input.name) &&
    (input.titleOverride === undefined || [...input.titleOverride].length <= MAX_NAME_CODE_POINTS) &&
    isValidHexColor(input.backgroundColor) &&
    isValidHexColor(input.foregroundColor) &&
    isValidHexColor(input.fillColor) &&
    isValidHexColor(input.borderColor) &&
    input.borderRadiusPx >= 0 &&
    input.borderRadiusPx <= MAX_WIDGET_BORDER_RADIUS_PX &&
    input.opacity >= 0 &&
    input.opacity <= 1
  );
}

export function defaultWidgetProfileDraft(goalId: string): WidgetProfileInput {
  return {
    goalId, name: '', enabled: true, showCurrent: true, showTarget: true, showPercent: true,
    orientation: 'horizontal', textAlign: 'center', fontFamily: 'sans_serif',
    backgroundColor: '#00000080', foregroundColor: '#ffffff', fillColor: '#7c3aed', borderColor: '#ffffff33',
    borderRadiusPx: DEFAULT_WIDGET_BORDER_RADIUS_PX, opacity: 1.0,
  };
}
