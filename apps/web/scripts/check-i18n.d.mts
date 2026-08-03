/**
 * Types for `check-i18n.mjs`.
 *
 * The checker itself is plain ESM JavaScript so it can run through `node`
 * without a build step; this declaration lets the TypeScript test suite import
 * it with full type safety.
 */

export type TranslationIssue = {
  /**
   * Problem category, e.g. `missing-key`, `extra-key`, `structure-mismatch`,
   * `empty-value`, `missing-plural-form`.
   */
  type: string;
  /** Dotted path of the offending key, prefixed with its namespace. */
  path: string;
  /** Human readable explanation. */
  detail: string;
};

export type TranslationCheckResult = {
  issues: TranslationIssue[];
  languages: string[];
  namespaces: string[];
};

/**
 * Compares every non-English resource directory against the English one.
 *
 * @param resourcesDir Defaults to `src/i18n/resources`.
 */
export function checkTranslationResources(resourcesDir?: string): TranslationCheckResult;
