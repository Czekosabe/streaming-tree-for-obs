import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { isSafeTwitchAssetUrl } from '@/models/operator-chat-presentation';

/**
 * Renders one emote fragment as an image when a safe, verified-host URL is
 * available, falling back to its plain text otherwise - a broken image
 * load or an unrecognized URL scheme must never hide the fragment's text
 * (see the Stage 9 task's Part 22).
 */
export function ChatEmoteImage({ url, text }: { url: string | undefined; text: string }) {
  const { t } = useTranslation('chat');
  const [failed, setFailed] = useState(false);

  if (url === undefined || url === '' || !isSafeTwitchAssetUrl(url) || failed) {
    return <span>{text}</span>;
  }

  return (
    <img
      src={url}
      alt={t('assetFallback.emoteAlt', { text })}
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      className="inline-block h-6 w-auto max-w-[64px] align-text-bottom"
      onError={() => setFailed(true)}
    />
  );
}
