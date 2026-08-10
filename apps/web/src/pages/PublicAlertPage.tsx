import { useParams } from 'react-router-dom';

import { AlertRenderer } from '@/components/alerts/AlertRenderer';
import { useAlertStream } from '@/hooks/use-alert-stream';
import { usePublicAlertProfileConfigQuery } from '@/hooks/use-alerts';

/**
 * The public OBS Browser Source alert route
 * (`/overlay/alerts/:publicSlug`) - a standalone page with no
 * application shell, sidebar, top bar, operator controls, queue
 * contents, or settings UI (see App.tsx: this route is registered
 * outside every layout wrapper other pages use, exactly like
 * pages/OverlayChatPage.tsx). Transparent by design, responsive to
 * whatever viewport OBS gives it, and needs no OBS permissions (no
 * camera/microphone/clipboard/filesystem access).
 *
 * Loads the public config once, then connects the public SSE stream
 * for the current alert and every live show/hide/paused/gap update -
 * see hooks/use-alert-stream.ts's own doc comment on why a separate
 * snapshot fetch is unnecessary (the stream's own first event is
 * already a complete reset). An unknown or disabled profile's config
 * request fails; the page then simply renders nothing, exactly like
 * the backend stream itself renders an empty reset rather than erroring
 * loudly on a live broadcast (see internal/httpapi/alerts.go's own doc
 * comment on handlePublicAlertStream).
 */
export function PublicAlertPage() {
  const { publicSlug } = useParams<{ publicSlug: string }>();
  const configQuery = usePublicAlertProfileConfigQuery(publicSlug);
  const stream = useAlertStream(publicSlug);

  if (configQuery.data === undefined) {
    // No visible loading UI - a transparent Browser Source should show
    // nothing while connecting, never a spinner or error text that
    // would appear on the live broadcast.
    return <div className="h-screen w-screen" data-testid="alert-page-empty" />;
  }

  return (
    <div className="h-screen w-screen overflow-hidden">
      <AlertRenderer config={configQuery.data} current={stream.current} />
    </div>
  );
}
