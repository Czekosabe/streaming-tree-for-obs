import type { ChatOverlayEditableFields, PublicChatOverlayConfig } from '@/api/chat-overlay-schemas';

/**
 * Projects the full editable profile (what the management form edits)
 * onto the smaller public renderer config - the exact same subset
 * `internal/httpapi`'s own `toPublicChatOverlayConfigResponse` keeps,
 * kept here in one place so the Overlays page's own preview (fed from
 * the in-progress draft, never the saved profile - see Part 19's "preview
 * updates immediately from draft") renders with the identical settings
 * the real public config endpoint would serve after a save.
 */
export function toPreviewConfig(fields: ChatOverlayEditableFields): PublicChatOverlayConfig {
  return {
    schemaVersion: 1,
    layoutMode: fields.layoutMode,
    stackDirection: fields.stackDirection,
    horizontalAlignment: fields.horizontalAlignment,
    showPlatformIcon: fields.showPlatformIcon,
    showPlatformName: fields.showPlatformName,
    showTimestamp: fields.showTimestamp,
    maxVisibleItems: fields.maxVisibleItems,
    messageLifetimeSeconds: fields.messageLifetimeSeconds,
    fontFamily: fields.fontFamily,
    fontSize: fields.fontSize,
    fontWeight: fields.fontWeight,
    lineHeight: fields.lineHeight,
    textColor: fields.textColor,
    usernameColorMode: fields.usernameColorMode,
    bubbleColor: fields.bubbleColor,
    bubbleOpacity: fields.bubbleOpacity,
    borderRadius: fields.borderRadius,
    itemSpacing: fields.itemSpacing,
    textOutline: fields.textOutline,
    textShadow: fields.textShadow,
    entryAnimation: fields.entryAnimation,
    exitAnimation: fields.exitAnimation,
    animationDurationMs: fields.animationDurationMs,
    highlightBroadcaster: fields.highlightBroadcaster,
    highlightModerators: fields.highlightModerators,
    highlightSubscribers: fields.highlightSubscribers,
    highlightVips: fields.highlightVips,
    language: fields.language,
    // This local draft-fields preview never reflects a saved visual
    // design (that lives in a separate DB row/route entirely) - always
    // "legacy" is the honest value here, never a fabricated
    // "visual_design" the panel could not actually render correctly.
    renderingMode: 'legacy',
  };
}

/** Strips a ChatOverlayProfile down to its editable fields - the shape a
 * PUT request body and a fresh draft both need. */
export function toEditableFields<T extends ChatOverlayEditableFields>(profile: T): ChatOverlayEditableFields {
  const {
    name,
    enabled,
    layoutMode,
    stackDirection,
    horizontalAlignment,
    showPlatformIcon,
    showPlatformName,
    showAccountLabel,
    showAvatar,
    showBadges,
    showTimestamp,
    showActivityEvents,
    showDeletedPlaceholder,
    hideCommands,
    hideBots,
    maxVisibleItems,
    messageLifetimeSeconds,
    fontFamily,
    fontSize,
    fontWeight,
    lineHeight,
    textColor,
    usernameColorMode,
    bubbleColor,
    bubbleOpacity,
    borderRadius,
    itemSpacing,
    textOutline,
    textShadow,
    entryAnimation,
    exitAnimation,
    animationDurationMs,
    highlightBroadcaster,
    highlightModerators,
    highlightSubscribers,
    highlightVips,
    language,
  } = profile;
  return {
    name,
    enabled,
    layoutMode,
    stackDirection,
    horizontalAlignment,
    showPlatformIcon,
    showPlatformName,
    showAccountLabel,
    showAvatar,
    showBadges,
    showTimestamp,
    showActivityEvents,
    showDeletedPlaceholder,
    hideCommands,
    hideBots,
    maxVisibleItems,
    messageLifetimeSeconds,
    fontFamily,
    fontSize,
    fontWeight,
    lineHeight,
    textColor,
    usernameColorMode,
    bubbleColor,
    bubbleOpacity,
    borderRadius,
    itemSpacing,
    textOutline,
    textShadow,
    entryAnimation,
    exitAnimation,
    animationDurationMs,
    highlightBroadcaster,
    highlightModerators,
    highlightSubscribers,
    highlightVips,
    language,
  };
}

export function editableFieldsEqual(a: ChatOverlayEditableFields, b: ChatOverlayEditableFields): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
