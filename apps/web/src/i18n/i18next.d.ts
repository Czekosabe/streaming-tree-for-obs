import type { DEFAULT_NAMESPACE } from './config';
import type { AppResources } from './resources';

/**
 * Teaches i18next the exact shape of our resources.
 *
 * With this augmentation `t('dashboard:backend.heading')` is checked at compile
 * time: a typo or a key removed from the English bundle becomes a type error
 * instead of a string that silently renders as its own key.
 */
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: typeof DEFAULT_NAMESPACE;
    resources: AppResources;
  }
}
