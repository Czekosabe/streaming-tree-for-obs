import { Loader2, Radio, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { SelectInput } from '@/components/ui/SelectInput';
import {
  usePlatformAccountLinkQuery,
  useRemoteTargetQuery,
  useYouTubeBroadcastsQuery,
  useSetRemoteTargetMutation,
  useDeleteRemoteTargetMutation,
} from '@/hooks/use-accounts';

type BroadcastSelectSectionProps = {
  platform: ConfiguredPlatform;
};

/**
 * Selected live-broadcast target for a YouTube destination, embedded in the
 * platform settings dialog.
 *
 * Deliberately its own section, separate from AccountLinkSection (the OAuth
 * account) and StreamKeySection (the RTMP credential): a connected account,
 * a selected broadcast, and a stream key are three independent facts. This
 * component never claims the selected broadcast is bound to the stream key
 * below - see docs/provider-integrations/youtube.md's "Remote broadcast
 * target" section.
 */
export function BroadcastSelectSection({ platform }: BroadcastSelectSectionProps) {
  const { t } = useTranslation(['accounts', 'common']);

  const linkQuery = usePlatformAccountLinkQuery(platform.id);
  const accountId = linkQuery.data?.accountId ?? null;

  const targetQuery = useRemoteTargetQuery(platform.id);
  const broadcastsQuery = useYouTubeBroadcastsQuery(accountId);
  const setTarget = useSetRemoteTargetMutation();
  const deleteTarget = useDeleteRemoteTargetMutation();

  const [selected, setSelected] = useState('');

  // Hooks above run unconditionally on every render (Rules of Hooks); only
  // the render output branches on provider, matching PublishPanel's own
  // established pattern in this codebase.
  if (platform.providerId !== 'youtube') return null;

  const busy = setTarget.isPending || deleteTarget.isPending;
  const target = targetQuery.data ?? null;
  const broadcasts = broadcastsQuery.data ?? [];
  const stale = target !== null && !broadcasts.some((b) => b.id === target.resourceId);

  if (accountId === null) {
    return (
      <div className="space-y-2 rounded-lg border border-line bg-surface-sunken p-3">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t('accounts:youtube.broadcast.heading')}
        </p>
        <p className="text-[11px] text-ink-faint">{t('accounts:youtube.broadcast.notLinked')}</p>
      </div>
    );
  }

  const handleSelect = () => {
    if (selected === '') return;
    setTarget.mutate({ platformId: platform.id, resourceId: selected }, { onSuccess: () => setSelected('') });
  };

  const handleClear = () => {
    deleteTarget.mutate(platform.id);
  };

  return (
    <div className="space-y-3 rounded-lg border border-line bg-surface-sunken p-3">
      <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {t('accounts:youtube.broadcast.heading')}
      </p>
      <p className="text-[11px] text-ink-faint">{t('accounts:youtube.broadcast.description')}</p>

      {target !== null && (
        <div className="flex items-center justify-between gap-2 rounded-md border border-line bg-surface px-2.5 py-2">
          <p className="min-w-0 truncate text-xs font-medium text-ink">
            {t('accounts:youtube.broadcast.selectedLabel', { title: target.displayName })}
          </p>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={busy}
            icon={<X className="size-3.5" />}
            onClick={handleClear}
          >
            {t('accounts:youtube.broadcast.clearButton')}
          </Button>
        </div>
      )}
      {target === null && (
        <p className="text-[11px] text-ink-faint">{t('accounts:youtube.broadcast.notSelected')}</p>
      )}
      {stale && (
        <p className="text-[11px] text-status-warning">{t('accounts:youtube.broadcast.staleNote')}</p>
      )}

      {broadcastsQuery.isLoading && (
        <p className="flex items-center gap-1.5 text-xs text-ink-muted">
          <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
        </p>
      )}

      {!broadcastsQuery.isLoading && broadcasts.length === 0 && (
        <div className="space-y-1">
          <p className="text-[11px] text-ink-faint">{t('accounts:youtube.broadcast.empty')}</p>
          <p className="text-[11px] text-ink-faint">{t('accounts:youtube.broadcast.creationNote')}</p>
        </div>
      )}

      {broadcasts.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <SelectInput
            aria-label={t('accounts:youtube.broadcast.chooseBroadcast')}
            className="max-w-[260px]"
            disabled={busy}
            value={selected}
            onChange={(event) => setSelected(event.target.value)}
            options={[
              { value: '', label: t('accounts:youtube.broadcast.chooseBroadcast') },
              ...broadcasts.map((b) => ({
                value: b.id,
                label: `${b.title} (${b.lifeCycleStatus === 'live' ? t('accounts:youtube.broadcast.statusActive') : t('accounts:youtube.broadcast.statusUpcoming')})`,
              })),
            ]}
          />
          <Button
            type="button"
            variant="secondary"
            size="sm"
            disabled={busy || selected === ''}
            icon={<Radio className="size-3.5" />}
            onClick={handleSelect}
          >
            {t('accounts:youtube.broadcast.selectButton')}
          </Button>
        </div>
      )}
    </div>
  );
}
