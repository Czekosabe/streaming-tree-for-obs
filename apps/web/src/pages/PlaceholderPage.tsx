import type { ParseKeys } from 'i18next';
import { Construction, type LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { Panel, PanelBody } from '@/components/ui/Panel';

type PlaceholderPageProps = {
  titleKey: ParseKeys<'pages'>;
  descriptionKey: ParseKeys<'pages'>;
  icon: LucideIcon;
  /** Keys describing what this view will do once its stage is implemented. */
  plannedKeys: readonly ParseKeys<'pages'>[];
  /** Real, working content rendered above the placeholder card. */
  children?: ReactNode;
};

/**
 * Shared empty view for routes whose feature has not been built yet.
 *
 * It deliberately shows no fake widgets: the point is to state clearly that the
 * section is planned and what it will contain.
 */
export function PlaceholderPage({
  titleKey,
  descriptionKey,
  icon: Icon,
  plannedKeys,
  children,
}: PlaceholderPageProps) {
  const { t } = useTranslation('pages');
  const title = t(titleKey);

  return (
    <AppShell title={title} description={t(descriptionKey)}>
      <div className="mx-auto max-w-2xl space-y-4">
        {children}

        <Panel>
          <PanelBody className="flex flex-col items-center gap-5 py-10 text-center sm:py-14">
            <span
              aria-hidden="true"
              className="flex size-14 items-center justify-center rounded-xl border border-line bg-surface-raised text-accent-soft"
            >
              <Icon className="size-6" />
            </span>

            <div className="space-y-2">
              <h2 className="text-base font-semibold tracking-tight text-ink">{title}</h2>
              <p className="mx-auto max-w-md text-sm leading-relaxed text-ink-muted">
                {t('placeholder.notImplemented')}
              </p>
            </div>

            <div className="w-full max-w-md rounded-lg border border-line bg-surface-sunken p-4 text-left">
              <p className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
                <Construction aria-hidden="true" className="size-3" />
                {t('placeholder.plannedHeading')}
              </p>
              <ul className="mt-2.5 space-y-1.5">
                {plannedKeys.map((key) => (
                  <li key={key} className="flex gap-2 text-xs text-ink-muted">
                    <span
                      aria-hidden="true"
                      className="mt-1.5 size-1 shrink-0 rounded-full bg-accent"
                    />
                    {t(key)}
                  </li>
                ))}
              </ul>
            </div>
          </PanelBody>
        </Panel>
      </div>
    </AppShell>
  );
}
