import { Check, Copy } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/cn';

type CopyableValueProps = {
  label: string;
  value: string;
  copyLabel: string;
  className?: string;
};

/**
 * A read-only value with a copy button.
 *
 * The value is rendered as selectable text as well, so it stays usable when
 * clipboard access is denied - which browsers do outside a secure context.
 * Copy feedback is announced through a live region rather than colour alone.
 */
export function CopyableValue({ label, value, copyLabel, className }: CopyableValueProps) {
  const { t } = useTranslation('runtime');
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 1_500);
    return () => window.clearTimeout(timer);
  }, [copied]);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
    } catch {
      // Denied or unavailable; the text remains selectable.
      setCopied(false);
    }
  };

  return (
    <div className={cn('min-w-0', className)}>
      <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">{label}</p>
      <div className="mt-1 flex items-center gap-1">
        {/* A URL or route name - never translated. */}
        <code className="min-w-0 flex-1 truncate rounded border border-line bg-canvas px-1.5 py-1 font-mono text-[11px] text-ink-muted">
          {value}
        </code>
        <button
          type="button"
          onClick={() => void copy()}
          aria-label={copyLabel}
          title={copyLabel}
          className="inline-flex size-6 shrink-0 items-center justify-center rounded border border-line text-ink-faint transition-colors hover:border-line-strong hover:text-ink"
        >
          {copied ? (
            <Check aria-hidden="true" className="size-3 text-status-live" />
          ) : (
            <Copy aria-hidden="true" className="size-3" />
          )}
        </button>
      </div>
      <span aria-live="polite" className="sr-only">
        {copied ? t('connection.copied') : ''}
      </span>
    </div>
  );
}
