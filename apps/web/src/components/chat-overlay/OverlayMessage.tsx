import type { ParseKeys } from 'i18next';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { PublicChatOverlayConfig, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';
import { isSafeTwitchAssetUrl } from '@/models/operator-chat-presentation';

import { OverlayFragment } from './OverlayFragment';
import { OverlaySourceMarker } from './OverlaySourceMarker';

type OverlaysKey = ParseKeys<'overlays'>;

const ROLE_TAG_KEYS = {
  broadcaster: 'renderer.role.broadcaster',
  moderator: 'renderer.role.moderator',
  subscriber: 'renderer.role.subscriber',
  vip: 'renderer.role.vip',
} as const satisfies Record<string, OverlaysKey>;

type RoleTag = keyof typeof ROLE_TAG_KEYS;

/** Role tags a message may carry - text labels, never color alone (Part
 * 12: "color not the only distinction"), and only ever derived from
 * already-normalized role flags the server computed from real chat
 * badges - never inferred from a username here either. */
function roleTags(item: PublicChatOverlayItem, config: PublicChatOverlayConfig): RoleTag[] {
  const user = item.user;
  if (user === undefined) return [];
  const tags: RoleTag[] = [];
  if (config.highlightBroadcaster && user.isBroadcaster === true) tags.push('broadcaster');
  if (config.highlightModerators && user.isModerator === true) tags.push('moderator');
  if (config.highlightSubscribers && user.isSubscriber === true) tags.push('subscriber');
  if (config.highlightVips && user.isVip === true) tags.push('vip');
  return tags;
}

function AvatarImage({ url, alt }: { url: string | undefined; alt: string }) {
  const [failed, setFailed] = useState(false);
  if (url === undefined || url === '' || !isSafeTwitchAssetUrl(url) || failed) return null;
  return (
    <img
      src={url}
      alt={alt}
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      className="size-[1.8em] shrink-0 rounded-full object-cover"
      onError={() => setFailed(true)}
    />
  );
}

export function OverlayMessage({
  item,
  config,
}: {
  item: PublicChatOverlayItem;
  config: PublicChatOverlayConfig;
}) {
  const { t } = useTranslation('overlays');
  const user = item.user;
  const anonymous = user?.anonymous ?? false;
  const displayName = anonymous ? '' : (user?.displayName ?? '');
  const usernameColor =
    config.usernameColorMode === 'provider' && user?.color !== undefined && user.color !== ''
      ? user.color
      : undefined;
  const tags = roleTags(item, config);

  return (
    <div className="flex items-start gap-1.5">
      <AvatarImage url={user?.avatarUrl} alt={displayName} />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-1">
          <OverlaySourceMarker
            providerId={item.providerId}
            accountLabel={item.accountLabel}
            showIcon={config.showPlatformIcon}
            showName={config.showPlatformName}
          />
          {config.showTimestamp && (
            <time className="text-[0.7em] tabular-nums opacity-70">
              {new Date(item.occurredAt).toLocaleTimeString()}
            </time>
          )}
          {user?.badges?.map((badge, index) =>
            badge.imageUrl1x !== undefined && isSafeTwitchAssetUrl(badge.imageUrl1x) ? (
              <img
                key={`${badge.setId}-${badge.id}-${index}`}
                src={badge.imageUrl1x}
                alt={badge.setId}
                loading="lazy"
                decoding="async"
                referrerPolicy="no-referrer"
                className="size-[1.1em]"
              />
            ) : null,
          )}
          {anonymous ? (
            <span className="italic opacity-70">{t('renderer.anonymous')}</span>
          ) : (
            displayName !== '' && (
              <span className="font-semibold" style={usernameColor ? { color: usernameColor } : undefined}>
                {displayName}
              </span>
            )
          )}
          {tags.map((tag) => (
            <span
              key={tag}
              className="rounded border border-current/40 px-1 text-[0.6em] font-bold uppercase tracking-wide opacity-90"
            >
              {t(ROLE_TAG_KEYS[tag])}
            </span>
          ))}
        </div>

        {item.deleted ? (
          <p className="italic opacity-60">{t('renderer.deletedPlaceholder')}</p>
        ) : (
          item.message !== undefined && (
            <p className="wrap-break-word whitespace-pre-wrap">
              {item.message.fragments.length === 0
                ? item.message.plainText
                : item.message.fragments.map((fragment, index) => (
                    <OverlayFragment key={index} fragment={fragment} />
                  ))}
            </p>
          )
        )}
      </div>
    </div>
  );
}
