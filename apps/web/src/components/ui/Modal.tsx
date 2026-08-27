import { X } from 'lucide-react';
import { useEffect, useId, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';

import { acquireBodyScrollLock } from '@/lib/body-scroll-lock';
import { cn } from '@/lib/cn';
import { APP_LAYER_Z } from '@/lib/z-layers';

type ModalProps = {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string | undefined;
  children: ReactNode;
  footer?: ReactNode;
  /**
   * When false, Escape and the backdrop do not close the dialog. Used while a
   * request is in flight so a half-finished action cannot be abandoned by a
   * stray keypress.
   */
  dismissible?: boolean;
  size?: 'sm' | 'md';
};

/**
 * Accessible modal dialog.
 *
 * Behaviour: focus moves into the panel on open and returns to whatever opened
 * it on close; Escape closes when dismissible; Tab is trapped inside the panel;
 * background scrolling is locked. Built on plain elements rather than a
 * dependency, since this is the only overlay the application needs.
 */
export function Modal({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  dismissible = true,
  size = 'md',
}: ModalProps) {
  const { t } = useTranslation('common');
  const panelRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);
  const titleId = useId();
  const descriptionId = useId();

  // A caller's onClose is almost never a stable reference (typically an
  // inline arrow or an unmemoized handler recreated on every render), and
  // dismissible is frequently derived from in-flight state (`!busy`).
  // Reading both through a ref that's kept current on every render - rather
  // than putting them in the lifecycle effect's own dependency array below -
  // is what lets that effect depend on `open` alone. Without this, typing
  // into any controlled input inside the modal re-renders the caller with a
  // new onClose identity, re-runs the whole effect, and redoes "move focus
  // to the panel's first focusable element" on every keystroke - a real
  // Stage 20E manual-test regression (focus jumping to the header's Close
  // button after each character typed in Add Platform's Display name
  // field). This keeps Escape/Tab-trap handling reading live values without
  // that.
  const onCloseRef = useRef(onClose);
  const dismissibleRef = useRef(dismissible);
  useEffect(() => {
    onCloseRef.current = onClose;
    dismissibleRef.current = dismissible;
  });

  useEffect(() => {
    if (!open) return;

    previouslyFocused.current = document.activeElement as HTMLElement | null;

    // Prefer the first real control so keyboard users start where they act.
    const focusable = panelRef.current?.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    );
    if (focusable !== undefined && focusable.length > 0) {
      focusable[0]?.focus();
    } else {
      panelRef.current?.focus();
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && dismissibleRef.current) {
        event.preventDefault();
        onCloseRef.current();
        return;
      }

      if (event.key !== 'Tab') return;

      const items = panelRef.current?.querySelectorAll<HTMLElement>(
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      );
      if (items === undefined || items.length === 0) return;

      const first = items[0];
      const last = items[items.length - 1];
      if (first === undefined || last === undefined) return;

      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    const releaseScrollLock = acquireBodyScrollLock();

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      releaseScrollLock();
      previouslyFocused.current?.focus();
    };
  }, [open]);

  if (!open) return null;

  // Rendered through a portal directly under <body>, deliberately never
  // as a descendant of any page content. A modal that renders inline
  // depends on every ancestor between it and <body> staying free of
  // anything that creates its own CSS stacking context (transform,
  // filter, opacity < 1, isolation, will-change, contain) - a
  // dependency nothing enforces, and a real Stage 20E manual test
  // found broken in practice (an unrelated entrance animation
  // elsewhere on the page ended up outranking this dialog). A portal
  // makes the question moot instead of relying on the rest of the
  // app never regressing it: see `lib/z-layers.ts`'s `APP_LAYER_Z`
  // for the one shared z-index scale every fixed/sticky overlay in
  // the app now draws from.
  return createPortal(
    <div className={cn('fixed inset-0 flex items-center justify-center p-4', APP_LAYER_Z.modal)}>
      <div
        role="presentation"
        onClick={dismissible ? onClose : undefined}
        className="absolute inset-0 bg-canvas/80 backdrop-blur-sm"
      />

      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description === undefined ? undefined : descriptionId}
        tabIndex={-1}
        className={cn(
          'animate-fade-rise relative flex max-h-[90vh] w-full flex-col overflow-hidden',
          'rounded-xl border border-line bg-surface shadow-raised',
          size === 'sm' ? 'max-w-md' : 'max-w-lg',
        )}
      >
        <header className="flex items-start justify-between gap-3 border-b border-line px-5 py-4">
          <div className="min-w-0">
            <h2 id={titleId} className="text-sm font-semibold tracking-tight text-ink">
              {title}
            </h2>
            {description !== undefined && (
              <p id={descriptionId} className="mt-1 text-xs text-ink-muted">
                {description}
              </p>
            )}
          </div>

          {dismissible && (
            <button
              type="button"
              onClick={onClose}
              aria-label={t('actions.close')}
              className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg border border-line text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink"
            >
              <X aria-hidden="true" className="size-4" />
            </button>
          )}
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>

        {footer !== undefined && (
          <footer className="flex flex-wrap items-center justify-end gap-2 border-t border-line px-5 py-4">
            {footer}
          </footer>
        )}
      </div>
    </div>,
    document.body,
  );
}
