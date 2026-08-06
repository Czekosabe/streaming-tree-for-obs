import { useTranslation } from 'react-i18next';

import type { OperatorChatItem, OperatorChatPreferences } from '@/api/operator-chat-schemas';
import { PlatformGlyph } from '@/components/platforms/PlatformGlyph';
import { cn } from '@/lib/cn';
import { providerGlyphClass } from '@/models/provider-labels';

/**
 * Platform-presentation block shown on a chat item: an app-owned glyph
 * (never a hotlinked brand logo - see PlatformGlyph's own doc comment),
 * optional provider name, and an account label shown only when there is
 * more than one connected account contributing to the timeline (Part 8).
 */
export function ChatSourceLabel({
  item,
  preferences,
  accountLabel,
}: {
  item: OperatorChatItem;
  preferences: OperatorChatPreferences;
  accountLabel: string | null;
}) {
  useTranslation('chat');
  const showAccount = preferences.showAccountLabel && accountLabel !== null;
  if (!preferences.showPlatformIcon && !preferences.showPlatformName && !showAccount) {
    return null;
  }

  return (
    <span className="inline-flex shrink-0 items-center gap-1.5">
      {preferences.showPlatformIcon && (
        <PlatformGlyph
          className={cn(providerGlyphClass(item.providerId), 'size-5 rounded text-[9px]')}
          label={item.providerId.slice(0, 2).toUpperCase()}
        />
      )}
      {preferences.showPlatformName && (
        <span className="text-[10px] font-medium uppercase tracking-wide text-ink-faint">
          {item.providerId}
        </span>
      )}
      {showAccount && <span className="text-[10px] text-ink-faint">{accountLabel}</span>}
    </span>
  );
}
