import { Sparkles } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';

/**
 * Bottom-right rail panel: genuinely planned, already-documented roadmap
 * items - not invented marketing copy. Sourced from the
 * `pages:platforms.planned.*`/`pages:metadata.planned.*` translation keys -
 * specific sub-features of the now-real `/platforms` and `/metadata` pages
 * that are not built yet (per-branch encoding profiles, title/category
 * history), distinct from those pages' own top-level `title`/`description`
 * copy. A feature that ships loses its entry here, the same way the two
 * pages themselves lost their placeholder state and `nav-items.ts`'s
 * `planned` flag once they became real.
 *
 * Deliberately no rocket illustration: the reference concept's rocket
 * asset is not available to this codebase, and this stage does not
 * approximate it with a different graphic - see docs/provider-branding.md's
 * sibling honesty policy for vendored assets.
 */
export function UpcomingFeaturesCard() {
  const { t } = useTranslation(['dashboard', 'pages']);

  const items = [
    t('pages:platforms.planned.encoding'),
    t('pages:metadata.planned.history'),
  ];

  return (
    <Panel>
      <PanelHeader
        title={t('dashboard:upcoming.heading')}
        description={t('dashboard:upcoming.description')}
        icon={<Sparkles className="size-4" />}
        headingLevel={3}
      />
      <PanelBody>
        <ul className="space-y-2">
          {items.map((item) => (
            <li key={item} className="flex items-start gap-2 text-xs text-ink-muted">
              <span aria-hidden="true" className="mt-1.5 size-1 shrink-0 rounded-full bg-accent" />
              {item}
            </li>
          ))}
        </ul>
      </PanelBody>
    </Panel>
  );
}
