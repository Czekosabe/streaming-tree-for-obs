import { useParams } from 'react-router-dom';

import { AudioRenderer } from '@/components/audio/AudioRenderer';
import { useAudioStream } from '@/hooks/use-audio-stream';

/**
 * The public OBS Browser Source audio route
 * (`/overlay/audio/:publicSlug`) - a standalone page with no
 * application shell, sidebar, top bar, operator controls, queue
 * contents, or settings UI (see App.tsx: this route is registered
 * outside every layout wrapper other pages use, exactly like
 * pages/PublicAlertPage.tsx). Renders no visible content at all, only
 * audio - transparent by design, responsive to whatever viewport OBS
 * gives it, and needs no OBS permissions.
 */
export function PublicAudioPage() {
  const { publicSlug } = useParams<{ publicSlug: string }>();
  const stream = useAudioStream(publicSlug);

  if (publicSlug === undefined) {
    return <div className="h-screen w-screen" data-testid="audio-page-empty" />;
  }

  return (
    <div className="h-screen w-screen overflow-hidden" data-testid="audio-page-empty">
      <AudioRenderer publicSlug={publicSlug} current={stream.current} rendererToken={stream.rendererToken} />
    </div>
  );
}
