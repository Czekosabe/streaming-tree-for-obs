import { CheckCircle2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/**
 * Final step: closes the assistant itself.
 *
 * Deliberately generic in this substage - the real per-category
 * readiness summary (application/OBS ingest/destinations/connected
 * accounts, docs/onboarding.md §7.2) lands in 21D, reusing the real
 * runtime/platform/account state already available elsewhere in the
 * app. This step never claims a specific readiness state it has not
 * actually checked.
 */
export function SummaryStep() {
  const { t } = useTranslation('onboarding');

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <span
          aria-hidden="true"
          className="flex size-10 shrink-0 items-center justify-center rounded-full bg-status-live/15 text-status-live"
        >
          <CheckCircle2 className="size-5" />
        </span>
        <h2 className="text-lg font-semibold tracking-tight text-ink">{t('summary.heading')}</h2>
      </div>
      <p className="text-sm leading-relaxed text-ink-muted">{t('summary.body')}</p>
    </div>
  );
}
