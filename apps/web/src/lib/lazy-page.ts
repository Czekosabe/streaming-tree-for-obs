import { lazy, type ComponentType } from 'react';

/**
 * `React.lazy` only accepts a module whose default export is the
 * component; every page in `src/pages/` is a named export instead (the
 * project's own consistent convention). This adapts one without
 * changing that convention just to satisfy `lazy`'s own shape.
 */
export function lazyPage<T extends ComponentType>(
  loader: () => Promise<Record<string, T>>,
  exportName: string,
) {
  return lazy(async () => {
    const module = await loader();
    const Component = module[exportName];
    if (Component === undefined) {
      throw new Error(`lazyPage: "${exportName}" is not exported by the loaded module`);
    }
    return { default: Component };
  });
}
