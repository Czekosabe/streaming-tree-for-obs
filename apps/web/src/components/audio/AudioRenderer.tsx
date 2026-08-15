import { useEffect, useRef } from 'react';

import { ackPublicAudio } from '@/api/audio';
import type { PublicAudioCurrentPayload } from '@/api/audio-schemas';

type AudioRendererProps = {
  publicSlug: string;
  current: PublicAudioCurrentPayload | null;
  rendererToken: string;
};

/**
 * Plays the current item's already-generated audio and reports
 * playback-started/ended/failed acknowledgements. One `<audio>`
 * element at a time; never displays queue/message/user information
 * (docs/audio-tts.md §53) - this component renders no visible content,
 * only a hidden audio element.
 *
 * Whether the browser's first programmatic play is actually accepted
 * by OBS's own CEF autoplay policy has not been manually verified
 * (docs/audio-tts.md §18/§33) - a rejected `play()` is reported
 * honestly as `playback_failed` rather than silently ignored.
 */
export function AudioRenderer({ publicSlug, current, rendererToken }: AudioRendererProps) {
  const audioRef = useRef<HTMLAudioElement>(null);

  useEffect(() => {
    const audio = audioRef.current;
    if (audio === null) return;
    if (current === null || rendererToken === '') {
      audio.removeAttribute('src');
      audio.load();
      return;
    }

    audio.src = current.bytesUrl;
    audio.volume = Math.min(1, Math.max(0, current.volume));
    // play() always returns a Promise per spec, but jsdom's test
    // environment does not implement it (returns undefined) - guard
    // defensively rather than assuming a thenable.
    const playResult: unknown = audio.play();
    if (playResult instanceof Promise) {
      void playResult.catch(() => {
        void ackPublicAudio(publicSlug, rendererToken, current.itemId, 'playback_failed');
      });
    }
    // Re-runs only when the current item's own identity or bytes URL
    // changes - not on every object identity change from a new SSE
    // payload that happens to describe the same item.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.itemId, current?.bytesUrl, rendererToken]);

  if (current === null || rendererToken === '') {
    return <audio ref={audioRef} data-testid="audio-renderer-element" className="hidden" />;
  }

  return (
    <audio
      ref={audioRef}
      data-testid="audio-renderer-element"
      className="hidden"
      onPlay={() => void ackPublicAudio(publicSlug, rendererToken, current.itemId, 'playback_started')}
      onEnded={() => void ackPublicAudio(publicSlug, rendererToken, current.itemId, 'playback_ended')}
      onError={() => void ackPublicAudio(publicSlug, rendererToken, current.itemId, 'playback_failed')}
    />
  );
}
