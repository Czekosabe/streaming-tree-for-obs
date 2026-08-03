import { ChevronDown, Languages } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { useLanguage } from '@/i18n/use-language';
import { cn } from '@/lib/cn';
import { isSupportedLanguage } from '@/i18n';

type LanguageSwitcherProps = {
  className?: string | undefined;
  /** Id of an external visible label. Falls back to an `aria-label`. */
  labelledBy?: string | undefined;
  id?: string | undefined;
};

/**
 * Interface language control.
 *
 * A native `<select>` is used on purpose: it is keyboard accessible and picks
 * up the platform's own picker on mobile, which no custom dropdown matches. The
 * options carry endonyms ("English", "Polski") rather than flags, because a
 * flag identifies a country, not a language.
 *
 * Switching is immediate - i18next re-renders subscribed components, so no page
 * reload happens.
 */
export function LanguageSwitcher({ className, labelledBy, id }: LanguageSwitcherProps) {
  const { t } = useTranslation('common');
  const { language, options, setLanguage } = useLanguage();

  return (
    <div className={cn('relative inline-flex items-center', className)}>
      <Languages
        aria-hidden="true"
        className="pointer-events-none absolute left-2.5 size-3.5 text-ink-faint"
      />
      <select
        id={id}
        value={language}
        aria-label={labelledBy === undefined ? t('language.switcherLabel') : undefined}
        aria-labelledby={labelledBy}
        onChange={(event) => {
          // Guarded rather than cast: the DOM value is untrusted input.
          if (isSupportedLanguage(event.target.value)) {
            setLanguage(event.target.value);
          }
        }}
        className={cn(
          'h-8 w-full appearance-none rounded-lg border border-line bg-surface',
          'py-0 pr-7 pl-8 text-xs font-medium text-ink',
          'transition-colors duration-150 hover:border-line-strong hover:bg-surface-hover',
          'focus:border-accent focus:outline-none',
        )}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value} className="bg-surface text-ink">
            {option.label}
          </option>
        ))}
      </select>
      <ChevronDown
        aria-hidden="true"
        className="pointer-events-none absolute right-2 size-3.5 text-ink-faint"
      />
    </div>
  );
}
