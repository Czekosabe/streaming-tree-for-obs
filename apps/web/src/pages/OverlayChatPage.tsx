import { useParams } from 'react-router-dom';

import { ChatOverlayRenderer } from '@/components/chat-overlay/ChatOverlayRenderer';
import { useChatOverlayStream } from '@/hooks/use-chat-overlay-stream';
import { usePublicChatOverlayConfigQuery } from '@/hooks/use-chat-overlay';

/**
 * The public OBS Browser Source route (`/overlay/chat/:publicSlug`) - a
 * standalone page with no application shell, sidebar, top bar, operator
 * controls, or settings UI (see App.tsx: this route is registered
 * outside every layout wrapper other pages use). Transparent by design,
 * responsive to whatever viewport OBS gives it.
 *
 * Loads the public config once, then connects the public SSE stream for
 * the current item set and every live update - see
 * hooks/use-chat-overlay-stream.ts's own doc comment on why a separate
 * snapshot fetch is unnecessary (the stream's own first event is already
 * a complete reset). An unknown or disabled overlay's config request
 * fails; the page then simply renders nothing, exactly like the backend
 * stream itself renders empty rather than erroring loudly on a live
 * broadcast (see internal/httpapi/chatoverlay.go's own doc comment).
 */
export function OverlayChatPage() {
  const { publicSlug } = useParams<{ publicSlug: string }>();
  const configQuery = usePublicChatOverlayConfigQuery(publicSlug);
  const stream = useChatOverlayStream(publicSlug);

  if (configQuery.data === undefined) {
    // No visible loading UI - an OBS Browser Source with a transparent
    // background should show nothing while connecting, never a spinner
    // or error text that would appear on the live broadcast.
    return <div className="h-screen w-screen" data-testid="chat-overlay-empty" />;
  }

  return <ChatOverlayRenderer config={configQuery.data} items={stream.items} />;
}
