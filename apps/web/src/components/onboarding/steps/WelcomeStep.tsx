import { useTranslation } from 'react-i18next';

/**
 * Step 1: the product model at user level (docs/onboarding.md §6.1).
 *
 * Leads with OBS -> Streaming Tree -> destinations, never with MediaMTX/
 * FFmpeg/port numbers/Go server terminology. No fake screenshots, stream
 * stats, or providers.
 */
export function WelcomeStep() {
  const { t } = useTranslation('onboarding');

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight text-ink">
        {t('welcome.heading')}
      </h2>
      <p className="text-sm leading-relaxed text-ink-muted">{t('welcome.flow')}</p>
      <p className="text-sm leading-relaxed text-ink-muted">{t('welcome.standalone')}</p>
      <p className="text-sm leading-relaxed text-ink-muted">{t('welcome.expectation')}</p>
    </div>
  );
}
