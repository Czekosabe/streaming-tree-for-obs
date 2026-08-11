import type { VisualDesignTextProps } from '@/api/visualdesign-schemas';

/**
 * Resolves a text layer's closed binding against one item's own safe,
 * already-resolved data (Stage 13A task Part 21; Stage 13B extends this
 * to chat items - docs/visual-designs.md §20) - never an arbitrary
 * object path, never a fabricated value for missing data (Part 12: "do
 * not fabricate values"). Deliberately pure/framework-free so it is
 * trivially unit-testable; every caller (AlertRenderer.tsx,
 * ChatOverlayRenderer.tsx, preview-scenarios.ts) resolves
 * `platform`/`eventType` labels via i18n *before* building this context
 * - this module never imports react-i18next itself, and
 * VisualDesignRenderer.tsx no longer does either (Stage 13B task Part
 * 16: the shared renderer takes an already-normalized data context, not
 * an alert object and not a Twitch object).
 */
export type VisualBindingContext = {
  renderedText: string | null;
  username: string | null;
  platform: string;
  eventType: string | null;
  message: string | null;
  quantity: number | null;
  groupCount: number;
  timestamp: string | null;
  accountLabel: string | null;
};

/** Returns the resolved text, or null when the bound value is genuinely
 * absent for this item (anonymous actor, no message, an alert event
 * type with no quantity, a chat message item with no account label,
 * etc.) - the caller decides hide-vs-placeholder from this null, never
 * from a fabricated fallback string. */
export function resolveTextBindingValue(text: VisualDesignTextProps, ctx: VisualBindingContext): string | null {
  switch (text.binding) {
    case 'static':
      return text.staticText !== undefined && text.staticText !== '' ? text.staticText : null;
    case 'alert_rendered_text':
      return ctx.renderedText;
    case 'username':
      return ctx.username;
    case 'platform':
      return ctx.platform;
    case 'event_type':
      return ctx.eventType;
    case 'message':
      return ctx.message;
    case 'quantity':
      return ctx.quantity !== null ? String(ctx.quantity) : null;
    case 'group_count':
      return String(ctx.groupCount);
    case 'timestamp':
      return ctx.timestamp;
    case 'account_label':
      return ctx.accountLabel;
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
