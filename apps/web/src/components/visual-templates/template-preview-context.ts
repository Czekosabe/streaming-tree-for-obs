import type { TFunction } from 'i18next';

import { chatItemDataContext } from '@/components/chat-overlay/chat-item-data-context';
import { chatPreviewScenarioItem } from '@/components/chat-overlay-designer/preview-scenarios';
import { platformDisplayName } from '@/components/visual-design/text-binding';
import type { VisualDesignDataContext } from '@/components/visual-design/VisualDesignRenderer';
import type { VisualTemplateTarget } from '@/api/visualtemplate-schemas';

/**
 * One deterministic, synthetic preview data context per target - used
 * only by the template gallery's own small preview thumbnails (Stage
 * 14A task Part 33). Never touches the Event Bus, a real queue, or
 * public SSE; never saves anything. Reuses the exact same
 * `chatItemDataContext`/`chatPreviewScenarioItem` pair the Chat Overlay
 * Designer's own 21 preview scenarios already use, so there is exactly
 * one "chat item -> renderer input" mapping in the whole application
 * (Stage 13B's own established rule) - the alert side has no saved-rule
 * template to render `alert_rendered_text` against here, so it uses a
 * small, self-contained static fixture instead, mirroring
 * preview-scenarios.ts's own "TestViewer" convention.
 */
export function templatePreviewDataContext(target: VisualTemplateTarget, t: TFunction<'overlays'>): VisualDesignDataContext {
  if (target === 'chat') {
    return chatItemDataContext(chatPreviewScenarioItem('message'), t);
  }
  return {
    providerId: 'twitch',
    avatarUrl: null,
    bindings: {
      renderedText: 'TestViewer just followed!',
      username: 'TestViewer',
      platform: platformDisplayName('twitch'),
      eventType: 'Follow',
      message: 'This is a test alert message.',
      quantity: 1,
      groupCount: 1,
      timestamp: null,
      accountLabel: null,
    },
  };
}
