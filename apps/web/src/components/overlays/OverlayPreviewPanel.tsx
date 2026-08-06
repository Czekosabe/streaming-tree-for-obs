import { useCallback, useEffect, useReducer } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChatOverlayEditableFields } from '@/api/chat-overlay-schemas';
import { ChatOverlayRenderer } from '@/components/chat-overlay/ChatOverlayRenderer';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { Button } from '@/components/ui/Button';
import {
  chatOverlayItemsInOrder,
  chatOverlayLeavingItemsInOrder,
  chatOverlayReducer,
  createChatOverlayState,
} from '@/models/chat-overlay-reducer';
import { toPreviewConfig } from '@/models/chat-overlay-config';
import { overlayPreviewFixtures } from '@/models/overlay-preview-fixtures';

const COSMETIC_TARGET_ID = 'preview_ordinary';
const IMMEDIATE_TARGET_ID = 'preview_badges';

function initPreviewState() {
  let state = createChatOverlayState();
  state = chatOverlayReducer(state, { type: 'reset', items: overlayPreviewFixtures() });
  return state;
}

/**
 * A local preview of the current draft, using deterministic synthetic
 * fixtures - never published to Twitch, the operator chat projection, or
 * a public SSE stream (Part 19). Updates immediately as the draft
 * changes; the real public overlay only updates after Save.
 *
 * Runs the same pure reducer (Part 20/21) as the real overlay so the
 * cosmetic-vs-immediate removal split can be demonstrated honestly: the
 * "simulate expiry" button plays the draft's own configured exit
 * animation, while "simulate moderation removal" always applies on the
 * same tick, with no animation, exactly like a real moderation deletion.
 */
export function OverlayPreviewPanel({ draft }: { draft: ChatOverlayEditableFields }) {
  const { t } = useTranslation('overlays');
  const [state, dispatch] = useReducer(chatOverlayReducer, undefined, initPreviewState);

  // The draft's fixture set never changes shape at runtime, but if the
  // panel remounts for a different overlay this still starts fresh.
  useEffect(() => {
    dispatch({ type: 'reset', items: overlayPreviewFixtures() });
  }, []);

  const items = chatOverlayItemsInOrder(state);
  const leaving = chatOverlayLeavingItemsInOrder(state);

  const onLeavingComplete = useCallback((id: string) => {
    dispatch({ type: 'completeLeaving', id });
  }, []);

  const simulateCosmeticRemoval = useCallback(() => {
    if (!(COSMETIC_TARGET_ID in state.itemsById)) return;
    dispatch({ type: 'remove', id: COSMETIC_TARGET_ID, reason: 'expired' });
  }, [state.itemsById]);

  const simulateImmediateRemoval = useCallback(() => {
    if (!(IMMEDIATE_TARGET_ID in state.itemsById)) return;
    dispatch({ type: 'remove', id: IMMEDIATE_TARGET_ID, reason: 'message_deleted' });
  }, [state.itemsById]);

  const resetPreview = useCallback(() => {
    dispatch({ type: 'reset', items: overlayPreviewFixtures() });
  }, []);

  return (
    <Panel>
      <PanelHeader
        title={t('preview.title')}
        description={t('preview.description')}
        actions={
          <span className="rounded border border-line px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-ink-faint">
            {t('preview.syntheticBadge')}
          </span>
        }
      />
      <PanelBody>
        <div className="mb-3 flex flex-wrap gap-2">
          <Button
            type="button"
            variant="secondary"
            title={t('preview.simulateCosmeticRemovalHint')}
            disabled={!(COSMETIC_TARGET_ID in state.itemsById)}
            onClick={simulateCosmeticRemoval}
          >
            {t('preview.simulateCosmeticRemoval')}
          </Button>
          <Button
            type="button"
            variant="secondary"
            title={t('preview.simulateImmediateRemovalHint')}
            disabled={!(IMMEDIATE_TARGET_ID in state.itemsById)}
            onClick={simulateImmediateRemoval}
          >
            {t('preview.simulateImmediateRemoval')}
          </Button>
          <Button type="button" variant="ghost" onClick={resetPreview}>
            {t('preview.resetPreview')}
          </Button>
        </div>
        <div
          className="relative h-[420px] w-full overflow-hidden rounded-lg border border-line bg-[repeating-conic-gradient(#1d2740_0%_25%,transparent_0%_50%)] bg-[length:16px_16px]"
          data-testid="overlay-preview-surface"
        >
          <ChatOverlayRenderer
            config={toPreviewConfig(draft)}
            items={items}
            leaving={leaving}
            onLeavingComplete={onLeavingComplete}
          />
        </div>
      </PanelBody>
    </Panel>
  );
}
