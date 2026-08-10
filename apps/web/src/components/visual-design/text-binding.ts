import type { VisualDesignTextProps } from '@/api/visualdesign-schemas';

/**
 * Resolves a text layer's closed binding against one alert's own safe,
 * already-present data (Stage 13A task Part 21) - never an arbitrary
 * object path, never a fabricated value for missing data (Part 12:
 * "do not fabricate values"). Deliberately pure/framework-free so it is
 * trivially unit-testable; the caller resolves `platformLabel`/
 * `eventTypeLabel` via i18n before calling this (this module never
 * imports react-i18next itself).
 */
export type AlertBindingContext = {
  renderedText: string;
  username: string | null;
  platformLabel: string;
  eventTypeLabel: string;
  message: string | null;
  quantity: number | null;
  groupCount: number;
};

/** Returns the resolved text, or null when the bound value is genuinely
 * absent for this alert (anonymous actor, no message, event type has
 * no quantity, etc.) - the caller decides hide-vs-placeholder from
 * this null, never from a fabricated fallback string. */
export function resolveTextBindingValue(text: VisualDesignTextProps, ctx: AlertBindingContext): string | null {
  switch (text.binding) {
    case 'static':
      return text.staticText !== undefined && text.staticText !== '' ? text.staticText : null;
    case 'alert_rendered_text':
      return ctx.renderedText;
    case 'username':
      return ctx.username;
    case 'platform':
      return ctx.platformLabel;
    case 'event_type':
      return ctx.eventTypeLabel;
    case 'message':
      return ctx.message;
    case 'quantity':
      return ctx.quantity !== null ? String(ctx.quantity) : null;
    case 'group_count':
      return String(ctx.groupCount);
    default:
      return null;
  }
}

/** The fixed, never-translated provider brand name (mirrors
 * internal/alerts.PlatformDisplayName exactly - brand names are never
 * localized anywhere in this application). */
export function platformDisplayName(providerId: string): string {
  switch (providerId) {
    case 'twitch':
      return 'Twitch';
    default:
      return providerId;
  }
}
