import { Link2, Unlink } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { SelectInput } from '@/components/ui/SelectInput';
import {
  useAccountsQuery,
  usePlatformAccountLinkQuery,
  useSetPlatformAccountLinkMutation,
  useDeletePlatformAccountLinkMutation,
} from '@/hooks/use-accounts';
import { accountStatusKey, accountStatusTone } from '@/models/account-presentation';
import { cn } from '@/lib/cn';

type AccountLinkSectionProps = {
  platform: ConfiguredPlatform;
};

const TONE_CLASSES: Record<'live' | 'error' | 'offline' | 'starting', string> = {
  live: 'border-status-live/40 bg-status-live/12 text-status-live',
  error: 'border-status-error/40 bg-status-error/12 text-status-error',
  offline: 'border-line text-ink-faint',
  starting: 'border-status-starting/40 bg-status-starting/12 text-status-starting',
};

/**
 * Connected-account link for one configured destination, embedded in the
 * platform settings dialog.
 *
 * Deliberately its own section, never merged with StreamKeySection: a
 * connected account (OAuth) and a destination stream key are different
 * credentials serving different purposes, and conflating them in one UI
 * block would blur that distinction the rest of this application is careful
 * to keep explicit (see docs/project-overview.md).
 */
export function AccountLinkSection({ platform }: AccountLinkSectionProps) {
  const { t } = useTranslation(['accounts', 'common']);

  const linkQuery = usePlatformAccountLinkQuery(platform.id);
  const accountsQuery = useAccountsQuery();
  const setLink = useSetPlatformAccountLinkMutation();
  const deleteLink = useDeletePlatformAccountLinkMutation();

  const [selected, setSelected] = useState('');

  // Twitch and YouTube both have a connected-account integration; every
  // other provider shows an honest "not implemented" state instead of a
  // fake selector.
  if (platform.providerId !== 'twitch' && platform.providerId !== 'youtube') {
    return (
      <div className="space-y-2 rounded-lg bg-surface-sunken/70 p-3">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t('accounts:link.heading')}
        </p>
        <p className="text-[11px] text-ink-faint">{t('accounts:link.notImplementedNote')}</p>
      </div>
    );
  }

  // Twitch and YouTube each have their own English/Polish wording (see
  // i18n/resources/*/accounts.json's top-level "link" vs "youtube.link"),
  // since "Twitch account" and "YouTube channel" are not interchangeable
  // nouns - Twitch's own keys are untouched by this generalization.
  const prefix = platform.providerId === 'youtube' ? 'accounts:youtube.link' : 'accounts:link';

  const busy = setLink.isPending || deleteLink.isPending;
  const accounts = (accountsQuery.data ?? []).filter((a) => a.providerId === platform.providerId);
  const link = linkQuery.data ?? null;
  const linkedAccount = link === null ? null : accounts.find((a) => a.id === link.accountId);

  const handleLink = () => {
    if (selected === '') return;
    setLink.mutate({ platformId: platform.id, accountId: selected });
  };

  const handleUnlink = () => {
    deleteLink.mutate(platform.id);
  };

  return (
    <div className="space-y-3 rounded-lg bg-surface-sunken/70 p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t(`${prefix}.heading`)}
        </p>
      </div>
      <p className="text-[11px] text-ink-faint">{t(`${prefix}.description`)}</p>

      {link !== null && (
        <div className="flex items-center justify-between gap-2 rounded-md border border-line bg-surface px-2.5 py-2">
          <div className="min-w-0">
            <p className="truncate text-xs font-medium text-ink">
              {t(`${prefix}.linkedTo`, { login: linkedAccount?.login ?? link.accountId })}
            </p>
            {linkedAccount !== undefined && linkedAccount !== null && (
              <span
                className={cn(
                  'mt-1 inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-semibold',
                  TONE_CLASSES[accountStatusTone(linkedAccount.status)],
                )}
              >
                {t(accountStatusKey(linkedAccount.status))}
              </span>
            )}
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={busy}
            icon={<Unlink className="size-3.5" />}
            onClick={handleUnlink}
          >
            {t(`${prefix}.unlinkButton`)}
          </Button>
        </div>
      )}

      {linkedAccount?.status === 'reconnect_required' && (
        <p className="text-[11px] text-status-warning">{t('accounts:link.reconnectRequiredNote')}</p>
      )}

      {accounts.length === 0 ? (
        <p className="text-[11px] text-ink-faint">{t(`${prefix}.noAccountsAvailable`)}</p>
      ) : (
        <div className="flex flex-wrap items-center gap-2">
          <SelectInput
            aria-label={t(`${prefix}.chooseAccount`)}
            className="max-w-[220px]"
            disabled={busy}
            value={selected}
            onChange={(event) => setSelected(event.target.value)}
            options={[
              { value: '', label: t(`${prefix}.chooseAccount`) },
              ...accounts
                .filter((a) => a.id !== link?.accountId)
                .map((a) => ({ value: a.id, label: a.login })),
            ]}
          />
          <Button
            type="button"
            variant="secondary"
            size="sm"
            disabled={busy || selected === ''}
            icon={<Link2 className="size-3.5" />}
            onClick={handleLink}
          >
            {t(`${prefix}.linkButton`)}
          </Button>
        </div>
      )}
      {link !== null && <p className="text-[11px] text-ink-faint">{t(`${prefix}.replaceNote`)}</p>}
    </div>
  );
}
