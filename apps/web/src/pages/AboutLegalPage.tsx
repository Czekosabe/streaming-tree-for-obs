import { ExternalLink, Heart, Scale, ScrollText } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { useAboutQuery } from '@/hooks/use-about-query';
import type { AboutResponse } from '@/models/about';

const EXTERNAL_LINK_CLASSES =
  'inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface-raised px-3 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-surface-hover';

function ExternalAction({ href, children }: { href: string; children: ReactNode }) {
  return (
    <a href={href} target="_blank" rel="noreferrer noopener" className={EXTERNAL_LINK_CLASSES}>
      {children}
      <ExternalLink aria-hidden="true" className="size-3.5" />
    </a>
  );
}

/** One row inside the "Legal & Privacy" panel: a short summary and, when the
 * canonical document exists in the repository, a link to view it in full. */
function LegalEntry({
  heading,
  body,
  detail,
  href,
  linkLabel,
}: {
  heading: string;
  body: string;
  /** Optional short technical line, e.g. an SPDX identifier - shown in monospace. */
  detail?: string;
  href?: string;
  linkLabel?: string;
}) {
  return (
    <div className="space-y-1.5 border-t border-line pt-3 first:border-t-0 first:pt-0">
      <h3 className="text-sm font-semibold text-ink">{heading}</h3>
      <p className="text-xs leading-relaxed text-ink-muted">{body}</p>
      {detail !== undefined && (
        <p className="font-mono text-[11px] text-ink-faint">{detail}</p>
      )}
      {href !== undefined && linkLabel !== undefined && (
        <a
          href={href}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-1 text-xs font-medium text-accent-soft hover:underline"
        >
          {linkLabel}
          <ExternalLink aria-hidden="true" className="size-3" />
        </a>
      )}
    </div>
  );
}

function VersionLine({ data }: { data: AboutResponse }) {
  const { t } = useTranslation('about');

  if (!data.isReleaseBuild) {
    const commitSuffix =
      data.commit === undefined
        ? ''
        : ` · ${
            data.commitDirty === true
              ? t('product.commitDirty', { commit: data.commit })
              : t('product.commit', { commit: data.commit })
          }`;
    return (
      <p className="text-xs text-ink-faint">
        {t('product.developmentBuild')}
        {commitSuffix}
      </p>
    );
  }

  return (
    <p className="text-xs text-ink-faint">
      {t('product.versionLabel')} {data.version}
    </p>
  );
}

/**
 * About & Legal: product identity, the voluntary creator-support action, and
 * pointers to the canonical PRIVACY.md/LEGAL.md/THIRD_PARTY_NOTICES.md
 * documents. Reached from Settings, not the primary sidebar.
 *
 * Every product-identity value here (name, creator, repository/creator/
 * support URLs) comes from GET /api/about - internal/buildinfo is the one
 * place these are defined, so this component never hardcodes them.
 */
export function AboutLegalPage() {
  const { t } = useTranslation(['about', 'common']);
  const { data, isPending, isError } = useAboutQuery();

  return (
    <AppShell title={t('about:meta.title')} description={t('about:meta.description')}>
      <div className="mx-auto max-w-2xl space-y-4">
        {isPending && <p className="text-sm text-ink-muted">{t('about:loading')}</p>}
        {isError && <p className="text-sm text-status-error">{t('about:error')}</p>}

        {data !== undefined && (
          <>
            <Panel>
              <PanelHeader
                title={data.productName}
                description={t('about:product.description')}
                icon={<ScrollText className="size-4" />}
              />
              <PanelBody className="space-y-3">
                <VersionLine data={data} />
                <p className="text-sm text-ink">
                  <span className="text-ink-muted">{t('about:product.createdByLabel')}: </span>
                  <span className="font-medium">{data.creatorName}</span>
                </p>
                <div className="flex flex-wrap gap-2 pt-1">
                  <ExternalAction href={data.repositoryUrl}>{t('about:links.sourceCode')}</ExternalAction>
                  <ExternalAction href={data.creatorUrl}>{t('about:links.creatorGithub')}</ExternalAction>
                </div>
              </PanelBody>
            </Panel>

            <Panel>
              <PanelHeader title={t('about:support.heading')} icon={<Heart className="size-4" />} />
              <PanelBody className="space-y-2.5">
                <p className="text-sm leading-relaxed text-ink">{t('about:support.body')}</p>
                <div>
                  <a
                    href={data.supportUrl}
                    target="_blank"
                    rel="noreferrer noopener"
                    className={EXTERNAL_LINK_CLASSES}
                  >
                    <Heart aria-hidden="true" className="size-3.5" />
                    {t('about:support.button')}
                    <ExternalLink aria-hidden="true" className="size-3.5" />
                  </a>
                </div>
                <p className="text-[11px] text-ink-faint">{t('about:support.disclosure')}</p>
                <p className="text-[11px] text-ink-faint">{t('about:support.voluntary')}</p>
                <p className="text-[11px] text-ink-faint">{t('about:support.noPaymentHandling')}</p>
              </PanelBody>
            </Panel>

            <Panel>
              <PanelHeader title={t('about:legal.heading')} icon={<Scale className="size-4" />} />
              <PanelBody className="space-y-3">
                <LegalEntry
                  heading={t('about:legal.licence.heading')}
                  body={t('about:legal.licence.summary', { name: data.applicationLicenseName })}
                  detail={`SPDX: ${data.applicationLicenseSpdx}`}
                  href={`${data.repositoryUrl}/blob/main/LICENSE`}
                  linkLabel={t('about:legal.licence.viewFull')}
                />
                <LegalEntry
                  heading={t('about:legal.privacy.heading')}
                  body={t('about:legal.privacy.summary')}
                  href={`${data.repositoryUrl}/blob/main/PRIVACY.md`}
                  linkLabel={t('about:legal.privacy.viewFull')}
                />
                <LegalEntry
                  heading={t('about:legal.thirdPartyNotices.heading')}
                  body={t('about:legal.thirdPartyNotices.summary')}
                  href={`${data.repositoryUrl}/blob/main/THIRD_PARTY_NOTICES.md`}
                  linkLabel={t('about:legal.thirdPartyNotices.viewFull')}
                />
                <LegalEntry
                  heading={t('about:legal.disclaimer.heading')}
                  body={t('about:legal.disclaimer.summary')}
                  href={`${data.repositoryUrl}/blob/main/LEGAL.md`}
                  linkLabel={t('about:legal.disclaimer.viewFull')}
                />
              </PanelBody>
            </Panel>
          </>
        )}
      </div>
    </AppShell>
  );
}
