import { Search, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { TextInput } from '@/components/ui/TextInput';
import { useCategorySearchQuery, usePlatformAccountLinkQuery } from '@/hooks/use-accounts';
import { cn } from '@/lib/cn';

type CategoryPickerProps = {
  platformId: string;
  value: string;
  categoryId: string;
  disabled: boolean;
  invalid: boolean;
  onChange: (category: string, categoryId: string) => void;
};

/**
 * Category field for a provider that requires a remote category ID to
 * publish (Twitch) - a search box backed by the linked account's category
 * search, instead of free text. Selecting a result stores both the display
 * name and the provider's own stable ID; typing without selecting leaves a
 * stale ID that the publish preview reports as a blocker rather than
 * guessing which remote category the text meant.
 */
export function CategoryPicker({
  platformId,
  value,
  categoryId,
  disabled,
  invalid,
  onChange,
}: CategoryPickerProps) {
  const { t } = useTranslation('accounts');
  const linkQuery = usePlatformAccountLinkQuery(platformId);
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);

  const accountId = linkQuery.data?.accountId ?? null;
  const searchQuery = useCategorySearchQuery(open ? accountId : null, query);

  if (accountId === null) {
    return (
      <div className="space-y-1.5">
        <TextInput value={value} disabled className="opacity-70" />
        <p className="text-[11px] text-ink-faint">{t('category.linkAccountNote')}</p>
      </div>
    );
  }

  return (
    <div className="relative space-y-1.5">
      <div className="relative">
        <Search
          aria-hidden="true"
          className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint"
        />
        <TextInput
          value={open ? query : value}
          disabled={disabled}
          aria-invalid={invalid}
          placeholder={t('category.searchPlaceholder')}
          className="pl-8"
          onFocus={() => {
            setOpen(true);
            setQuery('');
          }}
          onChange={(event) => setQuery(event.target.value)}
          onBlur={() => {
            // Delay so a click on a result registers before the list closes.
            window.setTimeout(() => setOpen(false), 150);
          }}
        />
        {value !== '' && !open && (
          <button
            type="button"
            aria-label={t('category.clear')}
            disabled={disabled}
            onClick={() => onChange('', '')}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-ink-faint hover:text-ink"
          >
            <X aria-hidden="true" className="size-3.5" />
          </button>
        )}
      </div>

      {open && (
        <div className="absolute z-10 mt-1 max-h-56 w-full overflow-y-auto rounded-lg border border-line bg-surface shadow-panel">
          {searchQuery.isFetching && (
            <p className="px-3 py-2 text-xs text-ink-muted">{t('category.searching')}</p>
          )}
          {!searchQuery.isFetching && query.trim().length >= 2 && (searchQuery.data ?? []).length === 0 && (
            <p className="px-3 py-2 text-xs text-ink-muted">{t('category.noResults')}</p>
          )}
          {(searchQuery.data ?? []).map((item) => (
            <button
              key={item.id}
              type="button"
              // onMouseDown fires before the input's onBlur, so the click
              // registers before the list closes.
              onMouseDown={(event) => {
                event.preventDefault();
                onChange(item.name, item.id);
                setOpen(false);
              }}
              className={cn(
                'flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-ink hover:bg-surface-hover',
                item.id === categoryId && 'bg-accent/10',
              )}
            >
              {item.boxArtUrl !== undefined && item.boxArtUrl !== '' && (
                <img src={item.boxArtUrl} alt="" aria-hidden="true" className="h-8 w-6 rounded object-cover" />
              )}
              {item.name}
            </button>
          ))}
        </div>
      )}

      {value !== '' && categoryId === '' && (
        <p className="text-[11px] text-status-warning">{t('category.staleNote')}</p>
      )}
    </div>
  );
}
