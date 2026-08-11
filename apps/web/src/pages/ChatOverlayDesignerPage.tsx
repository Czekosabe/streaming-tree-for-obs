import { useParams } from 'react-router-dom';

import { ChatOverlayDesignerWorkspace } from '@/components/chat-overlay-designer/ChatOverlayDesignerWorkspace';
import { useChatOverlayQuery } from '@/hooks/use-chat-overlay';
import { useVisualDesignQuery } from '@/hooks/use-visual-design';

/**
 * `/overlays/{overlayId}/designer` - the Chat Overlay Designer (Stage
 * 13B), a structural mirror of AlertDesignerPage.tsx. Deliberately
 * **not** wrapped in `AppShell`: like the Alert Designer and the public
 * Browser Source routes, this page wants full control of the viewport
 * for its own canvas/panel layout and top bar. Only resolves loading/
 * error state; all editing state lives in `ChatOverlayDesignerWorkspace`,
 * which never mounts until both dependencies (the overlay profile, the
 * current design) have actually loaded.
 */
export function ChatOverlayDesignerPage() {
  const { overlayId } = useParams<{ overlayId: string }>();
  const id = overlayId ?? null;

  const overlayQuery = useChatOverlayQuery(id ?? undefined);
  const designQuery = useVisualDesignQuery('chat-overlays', id);

  if (id === null) {
    return <div className="flex h-dvh items-center justify-center text-sm text-ink-muted" data-testid="chat-overlay-designer-error">Missing overlay id.</div>;
  }

  const isLoading = overlayQuery.isLoading || designQuery.isLoading;
  const isError = overlayQuery.isError || designQuery.isError;

  if (isLoading) {
    return <div className="flex h-dvh items-center justify-center text-sm text-ink-muted" data-testid="chat-overlay-designer-loading">Loading…</div>;
  }
  if (isError || overlayQuery.data === undefined || designQuery.data === undefined) {
    return <div className="flex h-dvh items-center justify-center text-sm text-status-error" data-testid="chat-overlay-designer-error">Could not load this overlay's design.</div>;
  }

  return <ChatOverlayDesignerWorkspace overlay={overlayQuery.data} initialResponse={designQuery.data} />;
}
