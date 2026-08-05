import { Loader2, Search, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import {
  useCategorySearchQuery,
  usePlatformAccountLinkQuery,
  useYouTubeCategoriesQuery,
  useYouTubeRegionQuery,
  useSetYouTubeRegionMutation,
} from '@/hooks/use-accounts';
import { cn } from '@/lib/cn';

type CategoryPickerProps = {
  platformId: string;
  providerId: string;
  value: string;
  categoryId: string;
  disabled: boolean;
  invalid: boolean;
  onChange: (category: string, categoryId: string) => void;
};

/**
 * Category field for a provider that requires a remote category ID to
 * publish - a Twitch text search, or a YouTube region-scoped list, instead
 * of free text. Selecting a result stores both the display name and the
 * provider's own stable ID; typing (Twitch) or leaving a value unselected
 * after a region change (YouTube) without selecting leaves a stale ID that
 * the publish preview reports as a blocker rather than guessing which
 * remote category the text meant.
 */
export function CategoryPicker(props: CategoryPickerProps) {
  if (props.providerId === 'youtube') return <YouTubeCategoryPicker {...props} />;
  return <TwitchCategorySearch {...props} />;
}

function TwitchCategorySearch({ platformId, value, categoryId, disabled, invalid, onChange }: CategoryPickerProps) {
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

function YouTubeCategoryPicker({ platformId, value, categoryId, disabled, invalid, onChange }: CategoryPickerProps) {
  const { t } = useTranslation('accounts');
  const linkQuery = usePlatformAccountLinkQuery(platformId);
  const accountId = linkQuery.data?.accountId ?? null;

  const regionQuery = useYouTubeRegionQuery(accountId);
  const categoriesQuery = useYouTubeCategoriesQuery(accountId);
  const setRegion = useSetYouTubeRegionMutation();

  const [editingRegion, setEditingRegion] = useState(false);
  const [regionInput, setRegionInput] = useState('');

  if (accountId === null) {
    return (
      <div className="space-y-1.5">
        <TextInput value={value} disabled className="opacity-70" />
        <p className="text-[11px] text-ink-faint">{t('category.linkAccountNoteYouTube')}</p>
      </div>
    );
  }

  const region = regionQuery.data ?? '';
  const categories = categoriesQuery.data ?? [];

  const handleSaveRegion = () => {
    const trimmed = regionInput.trim();
    if (trimmed.length !== 2) return;
    setRegion.mutate({ accountId, region: trimmed }, { onSuccess: () => setEditingRegion(false) });
  };

  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-2">
        {editingRegion ? (
          <>
            <TextInput
              value={regionInput}
              maxLength={2}
              className="w-16 uppercase"
              placeholder="US"
              onChange={(event) => setRegionInput(event.target.value)}
            />
            <button
              type="button"
              className="text-[11px] font-medium text-accent hover:underline"
              onClick={handleSaveRegion}
            >
              {t('integration.save')}
            </button>
          </>
        ) : (
          <>
            <span className="text-[11px] text-ink-faint">
              {region === ''
                ? t('category.regionRequiredNote')
                : t('category.regionLabel') + ': ' + region}
            </span>
            <button
              type="button"
              className="text-[11px] font-medium text-accent hover:underline"
              onClick={() => {
                setRegionInput(region);
                setEditingRegion(true);
              }}
            >
              {t('category.regionChangeButton')}
            </button>
          </>
        )}
      </div>

      {region !== '' && (
        <>
          {categoriesQuery.isLoading ? (
            <p className="flex items-center gap-1.5 text-xs text-ink-muted">
              <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
              {t('category.loadingList')}
            </p>
          ) : (
            <SelectInput
              value={categoryId}
              disabled={disabled}
              aria-invalid={invalid}
              onChange={(event) => {
                const chosen = categories.find((c) => c.id === event.target.value);
                if (chosen === undefined) {
                  onChange('', '');
                } else {
                  onChange(chosen.name, chosen.id);
                }
              }}
              options={[
                { value: '', label: t('category.listPlaceholder') },
                ...categories.map((c) => ({ value: c.id, label: c.name })),
              ]}
            />
          )}
        </>
      )}

      {value !== '' && categoryId === '' && (
        <p className="text-[11px] text-status-warning">{t('category.staleNote')}</p>
      )}
    </div>
  );
}
