import { useTranslation } from 'react-i18next';

import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';

const STEP_KEYS = [
  'step1',
  'step2',
  'step3',
  'step4',
  'step5',
  'step6',
  'step7',
  'step8',
  'step9',
  'step10',
  'step11',
  'step12',
] as const;

/** The 12-step Browser Source setup walkthrough (Part 20) - research-
 * backed recommendations, not a claim that one shutdown/refresh choice
 * is universal; see docs/obs-browser-source.md for the full trade-off
 * discussion this summarizes. */
export function OverlaySetupPanel() {
  const { t } = useTranslation('overlays');
  return (
    <Panel>
      <PanelHeader title={t('setup.title')} />
      <PanelBody>
        <ol className="list-decimal space-y-1.5 pl-5 text-sm text-ink-muted">
          {STEP_KEYS.map((key) => (
            <li key={key}>{t(`setup.${key}`)}</li>
          ))}
        </ol>
      </PanelBody>
    </Panel>
  );
}
