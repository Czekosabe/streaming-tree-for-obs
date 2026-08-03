import { FlaskConical } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/cn';

/**
 * Marks any value or control that is not backed by real functionality yet.
 *
 * Used consistently so a reviewer can tell at a glance which parts of the
 * dashboard are placeholders and which reflect real backend data.
 */
export function DemoBadge({ className, title }: { className?: string; title?: string }) {
  const { t } = useTranslation('common');

  return (
    <span
      title={title ?? t('demo.defaultTooltip')}
      className={cn(
        'inline-flex items-center gap-1 rounded-md border border-status-warning/35 bg-status-warning/10',
        'px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-status-warning',
        className,
      )}
    >
      <FlaskConical aria-hidden="true" className="size-3" />
      {t('demo.badge')}
    </span>
  );
}
