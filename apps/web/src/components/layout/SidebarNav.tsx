import { useTranslation } from 'react-i18next';
import { NavLink } from 'react-router-dom';

import { cn } from '@/lib/cn';

import { NAV_ITEMS } from './nav-items';

/**
 * Primary navigation. `NavLink` handles the active state and `aria-current`,
 * so the current section is exposed to assistive technology as well.
 */
export function SidebarNav({ onNavigate }: { onNavigate?: (() => void) | undefined }) {
  const { t } = useTranslation('navigation');

  return (
    <nav aria-label={t('primaryLabel')} className="px-3">
      <p className="px-2 pb-2 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {t('sectionLabel')}
      </p>
      <ul className="space-y-0.5">
        {NAV_ITEMS.map((item) => {
          const Icon = item.icon;
          return (
            <li key={item.to}>
              <NavLink
                to={item.to}
                end={item.to === '/'}
                onClick={onNavigate}
                className={({ isActive }) =>
                  cn(
                    'group flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition-colors duration-150',
                    isActive
                      ? 'bg-accent/12 font-medium text-ink ring-1 ring-accent/30 ring-inset'
                      : 'text-ink-muted hover:bg-surface-hover hover:text-ink',
                  )
                }
              >
                {({ isActive }) => (
                  <>
                    <Icon
                      aria-hidden="true"
                      className={cn(
                        'size-4 shrink-0 transition-colors',
                        isActive ? 'text-accent-soft' : 'text-ink-faint group-hover:text-ink-muted',
                      )}
                    />
                    <span className="truncate">{t(item.labelKey)}</span>
                    {item.planned && (
                      <span
                        className="ml-auto rounded border border-line px-1 text-[9px] font-semibold uppercase tracking-wide text-ink-faint"
                        title={t('plannedBadgeTooltip')}
                      >
                        {t('plannedBadge')}
                      </span>
                    )}
                  </>
                )}
              </NavLink>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
