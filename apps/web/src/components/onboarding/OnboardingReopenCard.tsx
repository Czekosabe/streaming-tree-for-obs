import { ChevronRight, Compass } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

import { Panel, PanelBody } from '@/components/ui/Panel';

/**
 * Settings entry point back into the onboarding assistant
 * (docs/onboarding.md §5.4), styled exactly like SettingsPage's own
 * About & Legal card - the established "settings section that
 * navigates to a dedicated route" pattern in this codebase.
 */
export function OnboardingReopenCard() {
  const { t } = useTranslation('onboarding');

  return (
    <Panel>
      <PanelBody>
        <Link
          to="/onboarding"
          className="flex items-center justify-between gap-3 rounded-lg -m-1 p-1 transition-colors hover:bg-surface-hover"
        >
          <span className="flex items-center gap-3">
            <span
              aria-hidden="true"
              className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-raised text-accent-soft"
            >
              <Compass className="size-4" />
            </span>
            <span>
              <span className="block text-sm font-semibold text-ink">
                {t('settingsCard.heading')}
              </span>
              <span className="block text-xs text-ink-muted">
                {t('settingsCard.description')}
              </span>
            </span>
          </span>
          <ChevronRight aria-hidden="true" className="size-4 shrink-0 text-ink-faint" />
        </Link>
      </PanelBody>
    </Panel>
  );
}
