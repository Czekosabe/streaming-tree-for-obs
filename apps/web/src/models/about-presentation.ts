import type { TFunction } from 'i18next';

import type { AboutResponse } from './about';

/**
 * Human-readable build/version line shared by every surface that shows the
 * application's own identity (the About page, the sidebar footer).
 *
 * `GET /api/about` (`internal/buildinfo` on the backend) is the single
 * source of truth - nothing in this codebase hardcodes a version string.
 * A non-release build (every manual/test/CI artifact) shows its real
 * development-build identity, including the commit it was built from when
 * the backend reported one, rather than a version number that would be
 * misleading outside an actual tagged release.
 */
export function aboutVersionLine(t: TFunction<'about'>, data: AboutResponse): string {
  if (!data.isReleaseBuild) {
    if (data.commit === undefined) return t('product.developmentBuild');
    const commit =
      data.commitDirty === true
        ? t('product.commitDirty', { commit: data.commit })
        : t('product.commit', { commit: data.commit });
    return `${t('product.developmentBuild')} · ${commit}`;
  }

  return `${t('product.versionLabel')} ${data.version}`;
}
