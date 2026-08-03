import { Check, Copy, Radio } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { APP_INFO } from '@/data/app-info';
import { DEMO_OBS_CONNECTION } from '@/data/demo-system';
import { cn } from '@/lib/cn';

import { DemoBadge } from '../ui/DemoBadge';

/**
 * Bottom block of the sidebar: OBS connection state, the local ingest address
 * OBS will point at, and the application version.
 *
 * The OBS state is a DEMO constant - nothing is listening on the RTMP port in
 * this stage, so the panel always reports "waiting".
 */
export function SidebarFooter() {
  const { t } = useTranslation('navigation');
  const [copied, setCopied] = useState(false);

  const copyIngestUrl = async () => {
    try {
      await navigator.clipboard.writeText(APP_INFO.localIngestUrl);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1_500);
    } catch {
      // Clipboard access can be denied; the address stays selectable as text.
      setCopied(false);
    }
  };

  return (
    <div className="mt-auto space-y-3 border-t border-line p-3">
      <section
        aria-label={t('obs.sectionLabel')}
        className="rounded-lg border border-line bg-surface-sunken p-3"
      >
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            <Radio aria-hidden="true" className="size-3" />
            {t('obs.heading')}
          </span>
          <DemoBadge title={t('obs.demoTooltip')} />
        </div>

        <p className="mt-1.5 flex items-center gap-2 text-xs font-medium text-ink">
          <span
            aria-hidden="true"
            className={cn('size-2 rounded-full bg-status-offline animate-pulse-ring')}
          />
          {t(DEMO_OBS_CONNECTION.labelKey)}
        </p>
        <p className="mt-0.5 text-[11px] text-ink-faint">{t(DEMO_OBS_CONNECTION.detailKey)}</p>

        <div className="mt-2.5">
          <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('obs.ingestLabel')}
          </p>
          <div className="mt-1 flex items-center gap-1">
            {/* The address itself is a URL - never translated. */}
            <code className="min-w-0 flex-1 truncate rounded border border-line bg-canvas px-1.5 py-1 font-mono text-[11px] text-ink-muted">
              {APP_INFO.localIngestUrl}
            </code>
            <button
              type="button"
              onClick={() => void copyIngestUrl()}
              aria-label={t('obs.copyIngest')}
              title={t('obs.copyIngest')}
              className="inline-flex size-6 shrink-0 items-center justify-center rounded border border-line text-ink-faint transition-colors hover:border-line-strong hover:text-ink"
            >
              {copied ? (
                <Check aria-hidden="true" className="size-3 text-status-live" />
              ) : (
                <Copy aria-hidden="true" className="size-3" />
              )}
            </button>
          </div>
          <p className="mt-1 text-[10px] text-ink-faint">{t('obs.ingestHint')}</p>
        </div>
      </section>

      <p className="px-1 text-[10px] text-ink-faint">
        {t('version', { version: APP_INFO.version })}
      </p>
    </div>
  );
}
