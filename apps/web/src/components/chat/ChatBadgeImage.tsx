import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { OperatorChatBadge } from '@/api/operator-chat-schemas';
import { isSafeTwitchAssetUrl } from '@/models/operator-chat-presentation';

/**
 * Renders one resolved chat badge as a small image, when a safe URL is
 * available. Unlike an emote (whose fragment text IS the message content),
 * a badge with no resolvable image is simply omitted - decoration, not
 * content, per the Stage 9 task's Part 11 fallback policy.
 */
export function ChatBadgeImage({ badge }: { badge: OperatorChatBadge }) {
  const { t } = useTranslation('chat');
  const [failed, setFailed] = useState(false);
  const url = badge.imageUrl2x ?? badge.imageUrl1x;

  if (url === undefined || url === '' || !isSafeTwitchAssetUrl(url) || failed) {
    return null;
  }

  return (
    <img
      src={url}
      alt={badge.info !== undefined && badge.info !== '' ? badge.info : t('assetFallback.badgeAlt')}
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      className="inline-block size-4"
      onError={() => setFailed(true)}
    />
  );
}
