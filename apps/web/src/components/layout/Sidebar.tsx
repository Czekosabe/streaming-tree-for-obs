import { X } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/cn';

import { BrandMark } from './BrandMark';
import { SidebarFooter } from './SidebarFooter';
import { SidebarNav } from './SidebarNav';

function SidebarContent({ onNavigate }: { onNavigate?: (() => void) | undefined }) {
  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-line p-4">
        <BrandMark />
      </div>
      <div className="flex-1 overflow-y-auto py-4">
        <SidebarNav onNavigate={onNavigate} />
      </div>
      <SidebarFooter />
    </div>
  );
}

/** Static sidebar, visible from the `lg` breakpoint upwards. */
export function DesktopSidebar() {
  return (
    <aside className="hidden w-64 shrink-0 border-r border-line bg-surface lg:block">
      <div className="sticky top-0 h-dvh">
        <SidebarContent />
      </div>
    </aside>
  );
}

type MobileSidebarProps = {
  open: boolean;
  onClose: () => void;
};

/**
 * Off-canvas drawer used below `lg`.
 *
 * Closes on Escape and returns focus to the element that opened it; the panel
 * itself is focused on open so keyboard users land inside the dialog.
 */
export function MobileSidebar({ open, onClose }: MobileSidebarProps) {
  const { t } = useTranslation(['navigation', 'common']);
  const panelRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;

    previouslyFocused.current = document.activeElement as HTMLElement | null;
    panelRef.current?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };

    document.addEventListener('keydown', handleKeyDown);
    document.body.style.overflow = 'hidden';

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = '';
      previouslyFocused.current?.focus();
    };
  }, [open, onClose]);

  return (
    <div
      className={cn('fixed inset-0 z-50 lg:hidden', open ? '' : 'pointer-events-none')}
      aria-hidden={!open}
    >
      <div
        role="presentation"
        onClick={onClose}
        className={cn(
          'absolute inset-0 bg-canvas/80 backdrop-blur-sm transition-opacity duration-200',
          open ? 'opacity-100' : 'opacity-0',
        )}
      />
      <div
        ref={panelRef}
        role="dialog"
        aria-modal={open}
        aria-label={t('navigation:mainMenu')}
        tabIndex={-1}
        className={cn(
          'absolute inset-y-0 left-0 flex w-72 max-w-[85vw] flex-col border-r border-line bg-surface',
          'shadow-raised transition-transform duration-200 ease-out',
          open ? 'translate-x-0' : '-translate-x-full',
        )}
      >
        <button
          type="button"
          onClick={onClose}
          aria-label={t('common:actions.closeMenu')}
          className="absolute top-3 right-3 inline-flex size-8 items-center justify-center rounded-lg border border-line text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink"
        >
          <X aria-hidden="true" className="size-4" />
        </button>
        <SidebarContent onNavigate={onClose} />
      </div>
    </div>
  );
}
