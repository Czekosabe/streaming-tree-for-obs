import { Menu } from 'lucide-react';
import type { ReactNode } from 'react';

import { SystemStatusPill } from '../system/SystemStatusPill';

type TopBarProps = {
  title: string;
  description: string;
  actions?: ReactNode;
  onOpenMenu: () => void;
};

/**
 * Sticky page header: menu trigger (below `lg`), page title, page actions and
 * the aggregated system status.
 */
export function TopBar({ title, description, actions, onOpenMenu }: TopBarProps) {
  return (
    <header className="sticky top-0 z-30 border-b border-line bg-canvas/85 backdrop-blur-md">
      <div className="flex flex-wrap items-center gap-3 px-4 py-3 sm:px-6">
        <button
          type="button"
          onClick={onOpenMenu}
          aria-label="Open menu"
          className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink lg:hidden"
        >
          <Menu aria-hidden="true" className="size-4" />
        </button>

        <div className="min-w-0 flex-1">
          <h1 className="truncate text-base font-semibold tracking-tight text-ink sm:text-lg">
            {title}
          </h1>
          <p className="truncate text-xs text-ink-muted">{description}</p>
        </div>

        <div className="order-last flex w-full items-center gap-2 sm:order-none sm:w-auto">
          <SystemStatusPill className="sm:order-last" />
          {actions !== undefined && (
            <div className="flex flex-1 items-center justify-end gap-2 sm:flex-none">{actions}</div>
          )}
        </div>
      </div>
    </header>
  );
}
