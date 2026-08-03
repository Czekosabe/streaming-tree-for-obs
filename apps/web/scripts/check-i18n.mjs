#!/usr/bin/env node
/**
 * Translation resource consistency check.
 *
 * English is the canonical key structure. Every other language must mirror it
 * exactly, with one deliberate exception: plural forms. Languages have
 * different CLDR plural categories (English needs `_one`/`_other`, Polish also
 * needs `_few`/`_many`), so plural groups are compared by their base key and
 * each language is checked against the categories `Intl.PluralRules` says it
 * requires.
 *
 * Detected problems:
 *   - a key present in English but missing in the target language,
 *   - a key present in the target language but absent from English,
 *   - incompatible structures (object vs. string) at the same path,
 *   - empty or whitespace-only translation values,
 *   - missing or unexpected plural forms.
 *
 * Written in plain ESM JavaScript so `npm run i18n:check` needs no build step
 * and no extra dependency. The comparison function is exported so the test
 * suite can assert on it directly.
 */

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const RESOURCES_DIR = resolve(SCRIPT_DIR, '..', 'src', 'i18n', 'resources');

/** Language that defines the canonical key structure. */
const SOURCE_LANGUAGE = 'en';

/** Suffixes i18next appends for plural categories. */
const PLURAL_SUFFIXES = ['zero', 'one', 'two', 'few', 'many', 'other'];

const PLURAL_PATTERN = new RegExp(`^(.*)_(${PLURAL_SUFFIXES.join('|')})$`);

/**
 * @typedef {{ type: string, path: string, detail: string }} Issue
 */

/** Plural categories a language must supply, according to CLDR. */
function requiredPluralCategories(language) {
  const categories = new Intl.PluralRules(language, { type: 'cardinal' }).resolvedOptions()
    .pluralCategories;
  return new Set(categories);
}

/**
 * Splits an object's own keys into plain keys and plural groups.
 *
 * @param {Record<string, unknown>} node
 */
function partitionKeys(node) {
  /** @type {Set<string>} */
  const plain = new Set();
  /** @type {Map<string, Set<string>>} */
  const plurals = new Map();

  for (const key of Object.keys(node)) {
    const match = PLURAL_PATTERN.exec(key);
    if (match === null) {
      plain.add(key);
      continue;
    }

    const [, base, category] = match;
    const categories = plurals.get(base) ?? new Set();
    categories.add(category);
    plurals.set(base, categories);
  }

  return { plain, plurals };
}

function isPlainObject(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function joinPath(prefix, key) {
  return prefix === '' ? key : `${prefix}.${key}`;
}

/**
 * Recursively compares one namespace of the target language against English.
 *
 * @param {unknown} source
 * @param {unknown} target
 * @param {string} path
 * @param {string} language
 * @param {Set<string>} pluralCategories
 * @param {Issue[]} issues
 */
function compareNode(source, target, path, language, pluralCategories, issues) {
  if (isPlainObject(source) !== isPlainObject(target)) {
    issues.push({
      type: 'structure-mismatch',
      path,
      detail: `English has ${isPlainObject(source) ? 'an object' : 'a value'}, ${language} has ${
        isPlainObject(target) ? 'an object' : 'a value'
      }`,
    });
    return;
  }

  if (!isPlainObject(source)) {
    if (typeof source !== 'string') {
      issues.push({
        type: 'unsupported-value',
        path,
        detail: `only strings and objects are allowed, found ${Array.isArray(source) ? 'array' : typeof source}`,
      });
      return;
    }
    if (typeof target === 'string' && target.trim() === '') {
      issues.push({ type: 'empty-value', path, detail: `${language} value is empty` });
    }
    return;
  }

  const sourceKeys = partitionKeys(source);
  const targetKeys = partitionKeys(target);

  // --- plain keys ---------------------------------------------------------
  for (const key of sourceKeys.plain) {
    if (!targetKeys.plain.has(key)) {
      issues.push({
        type: 'missing-key',
        path: joinPath(path, key),
        detail: `present in ${SOURCE_LANGUAGE}, missing in ${language}`,
      });
      continue;
    }
    compareNode(
      source[key],
      target[key],
      joinPath(path, key),
      language,
      pluralCategories,
      issues,
    );
  }

  for (const key of targetKeys.plain) {
    if (!sourceKeys.plain.has(key)) {
      issues.push({
        type: 'extra-key',
        path: joinPath(path, key),
        detail: `present in ${language}, missing in ${SOURCE_LANGUAGE}`,
      });
    }
  }

  // --- plural groups ------------------------------------------------------
  for (const [base, categories] of sourceKeys.plurals) {
    const targetCategories = targetKeys.plurals.get(base);

    if (targetCategories === undefined) {
      issues.push({
        type: 'missing-key',
        path: joinPath(path, base),
        detail: `plural group present in ${SOURCE_LANGUAGE} (${[...categories].sort().join(', ')}), missing in ${language}`,
      });
      continue;
    }

    for (const category of pluralCategories) {
      if (!targetCategories.has(category)) {
        issues.push({
          type: 'missing-plural-form',
          path: joinPath(path, `${base}_${category}`),
          detail: `${language} requires the "${category}" plural category`,
        });
      }
    }

    for (const category of targetCategories) {
      if (!pluralCategories.has(category)) {
        issues.push({
          type: 'unexpected-plural-form',
          path: joinPath(path, `${base}_${category}`),
          detail: `"${category}" is not a plural category of ${language}`,
        });
        continue;
      }
      const value = target[`${base}_${category}`];
      if (typeof value !== 'string') {
        issues.push({
          type: 'unsupported-value',
          path: joinPath(path, `${base}_${category}`),
          detail: 'plural forms must be strings',
        });
      } else if (value.trim() === '') {
        issues.push({
          type: 'empty-value',
          path: joinPath(path, `${base}_${category}`),
          detail: `${language} value is empty`,
        });
      }
    }
  }

  for (const base of targetKeys.plurals.keys()) {
    if (!sourceKeys.plurals.has(base)) {
      issues.push({
        type: 'extra-key',
        path: joinPath(path, base),
        detail: `plural group present in ${language}, missing in ${SOURCE_LANGUAGE}`,
      });
    }
  }
}

function listLanguages(resourcesDir) {
  return readdirSync(resourcesDir)
    .filter((entry) => statSync(join(resourcesDir, entry)).isDirectory())
    .sort();
}

function listNamespaces(resourcesDir, language) {
  return readdirSync(join(resourcesDir, language))
    .filter((file) => file.endsWith('.json'))
    .map((file) => file.slice(0, -'.json'.length))
    .sort();
}

function readNamespace(resourcesDir, language, namespace) {
  const file = join(resourcesDir, language, `${namespace}.json`);
  return JSON.parse(readFileSync(file, 'utf8'));
}

/**
 * Compares every non-source language against English.
 *
 * @param {string} [resourcesDir]
 * @returns {{ issues: Issue[], languages: string[], namespaces: string[] }}
 */
export function checkTranslationResources(resourcesDir = RESOURCES_DIR) {
  /** @type {Issue[]} */
  const issues = [];

  const languages = listLanguages(resourcesDir);

  if (!languages.includes(SOURCE_LANGUAGE)) {
    issues.push({
      type: 'missing-language',
      path: SOURCE_LANGUAGE,
      detail: 'the canonical source language directory does not exist',
    });
    return { issues, languages, namespaces: [] };
  }

  const sourceNamespaces = listNamespaces(resourcesDir, SOURCE_LANGUAGE);

  for (const language of languages) {
    if (language === SOURCE_LANGUAGE) continue;

    const targetNamespaces = listNamespaces(resourcesDir, language);
    const pluralCategories = requiredPluralCategories(language);

    for (const namespace of sourceNamespaces) {
      if (!targetNamespaces.includes(namespace)) {
        issues.push({
          type: 'missing-namespace',
          path: `${language}/${namespace}.json`,
          detail: `namespace exists for ${SOURCE_LANGUAGE} but not for ${language}`,
        });
        continue;
      }

      compareNode(
        readNamespace(resourcesDir, SOURCE_LANGUAGE, namespace),
        readNamespace(resourcesDir, language, namespace),
        namespace,
        language,
        pluralCategories,
        issues,
      );
    }

    for (const namespace of targetNamespaces) {
      if (!sourceNamespaces.includes(namespace)) {
        issues.push({
          type: 'extra-namespace',
          path: `${language}/${namespace}.json`,
          detail: `namespace exists for ${language} but not for ${SOURCE_LANGUAGE}`,
        });
      }
    }
  }

  return { issues, languages, namespaces: sourceNamespaces };
}

function main() {
  const { issues, languages, namespaces } = checkTranslationResources();

  if (issues.length === 0) {
    console.log(
      `i18n check passed: ${languages.length} languages (${languages.join(', ')}), ` +
        `${namespaces.length} namespaces, no differences against "${SOURCE_LANGUAGE}".`,
    );
    return;
  }

  console.error(`i18n check failed with ${issues.length} problem(s):\n`);
  for (const issue of issues) {
    console.error(`  [${issue.type}] ${issue.path}\n      ${issue.detail}`);
  }
  console.error(`\n"${SOURCE_LANGUAGE}" defines the canonical key structure.`);
  process.exitCode = 1;
}

// Only run when invoked directly, so importing the module in tests is side-effect free.
if (process.argv[1] !== undefined && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
  main();
}
