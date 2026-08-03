import type { ReactNode } from 'react';

import { cn } from '@/lib/cn';

type PanelProps = {
  children: ReactNode;
  className?: string;
  /** Renders the slightly lighter surface used for nested blocks. */
  raised?: boolean;
  as?: 'section' | 'article' | 'aside' | 'div';
};

/** Base surface: dark panel, subtle border, restrained shadow. */
export function Panel({ children, className, raised = false, as = 'section' }: PanelProps) {
  const Tag = as;
  return (
    <Tag
      className={cn(
        'rounded-xl border border-line shadow-panel',
        raised ? 'bg-surface-raised' : 'bg-surface',
        className,
      )}
    >
      {children}
    </Tag>
  );
}

type PanelHeaderProps = {
  title: string;
  description?: string;
  icon?: ReactNode;
  actions?: ReactNode;
  className?: string;
  /** Heading level, so pages keep a correct document outline. */
  headingLevel?: 2 | 3;
};

export function PanelHeader({
  title,
  description,
  icon,
  actions,
  className,
  headingLevel = 2,
}: PanelHeaderProps) {
  const Heading = headingLevel === 2 ? 'h2' : 'h3';

  return (
    <header
      className={cn(
        'flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-3 sm:px-5',
        className,
      )}
    >
      <div className="flex min-w-0 items-start gap-3">
        {icon !== undefined && (
          <span
            aria-hidden="true"
            className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-raised text-accent-soft"
          >
            {icon}
          </span>
        )}
        <div className="min-w-0">
          <Heading className="truncate text-sm font-semibold tracking-tight text-ink">
            {title}
          </Heading>
          {description !== undefined && (
            <p className="mt-0.5 text-xs text-ink-muted">{description}</p>
          )}
        </div>
      </div>
      {actions !== undefined && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}

export function PanelBody({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('p-4 sm:p-5', className)}>{children}</div>;
}
