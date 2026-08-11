import { useEffect, useState } from 'react';

/**
 * Loads a Stage 14B managed WOFF2 font asset via the browser `FontFace`
 * API and registers it under an app-generated internal family name -
 * never trusts a font's own internal family name as CSS input (docs/
 * visual-template-packages.md §22/§41: "the application assigns an
 * internal deterministic renderer family name"). Returns `null` while
 * loading, on failure, or when no font is requested - callers fall back
 * to the safe system font in every one of those cases, never blocking
 * on a font load and never leaving text invisible.
 *
 * One in-memory cache per asset id for the lifetime of the page - a
 * font already loaded once (e.g. by another layer, or a previous
 * mount) resolves immediately without a second network request.
 */

const cache = new Map<string, Promise<string>>();

function internalFamilyName(assetId: string): string {
  return `managed-font-${assetId.replace(/[^A-Za-z0-9_-]/g, '')}`;
}

export function useManagedFont(assetId: string | undefined, url: string | undefined): string | null {
  const [family, setFamily] = useState<string | null>(null);

  useEffect(() => {
    if (assetId === undefined || url === undefined) {
      setFamily(null);
      return;
    }

    let cancelled = false;
    let loadPromise = cache.get(assetId);
    if (loadPromise === undefined) {
      const name = internalFamilyName(assetId);
      loadPromise = new FontFace(name, `url(${JSON.stringify(url)})`)
        .load()
        .then((loaded) => {
          (document.fonts as FontFaceSet).add(loaded);
          return name;
        });
      cache.set(assetId, loadPromise);
    }

    loadPromise
      .then((name) => {
        if (!cancelled) setFamily(name);
      })
      .catch(() => {
        if (!cancelled) setFamily(null);
      });

    return () => {
      cancelled = true;
    };
  }, [assetId, url]);

  return family;
}
