import { useState } from 'react';

import type { PublicChatOverlayFragment } from '@/api/chat-overlay-schemas';
import { isSafeTwitchAssetUrl } from '@/models/operator-chat-presentation';

/**
 * Renders one ordered message fragment - plain text, an emote image (with
 * a same-host-verified URL, falling back to its text on load failure or
 * an unrecognized host), or a highlighted mention. Never
 * `dangerouslySetInnerHTML`; every value is typed, validated data.
 */
export function OverlayFragment({ fragment }: { fragment: PublicChatOverlayFragment }) {
  const [failed, setFailed] = useState(false);

  switch (fragment.type) {
    case 'emote': {
      const url = fragment.emoteImageUrl;
      if (url === undefined || url === '' || !isSafeTwitchAssetUrl(url) || failed) {
        return <span>{fragment.text}</span>;
      }
      return (
        <img
          src={url}
          alt={fragment.text}
          loading="lazy"
          decoding="async"
          referrerPolicy="no-referrer"
          className="inline-block h-[1.6em] w-auto max-w-[4em] align-text-bottom"
          onError={() => setFailed(true)}
        />
      );
    }
    case 'mention':
      return <span className="font-semibold opacity-90">{fragment.text}</span>;
    default:
      return <span>{fragment.text}</span>;
  }
}
