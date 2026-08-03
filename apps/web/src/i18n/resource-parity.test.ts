import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { checkTranslationResources } from '../../scripts/check-i18n.mjs';
import { NAMESPACES, SUPPORTED_LANGUAGES } from './config';

describe('shipped translation resources', () => {
  it('are consistent across every language and namespace', () => {
    const { issues, languages, namespaces } = checkTranslationResources();

    // Printed on failure so the mismatching paths are visible in CI output.
    expect(issues).toEqual([]);
    expect(languages.sort()).toEqual([...SUPPORTED_LANGUAGES].sort());
    expect(namespaces.sort()).toEqual([...NAMESPACES].sort());
  });
});

describe('the parity checker itself', () => {
  let fixtureDir: string | null = null;

  afterEach(() => {
    if (fixtureDir !== null) {
      rmSync(fixtureDir, { recursive: true, force: true });
      fixtureDir = null;
    }
  });

  /** Builds a throwaway resource tree so the detection logic can be asserted. */
  function createFixture(
    en: Record<string, unknown>,
    pl: Record<string, unknown>,
  ): string {
    const root = mkdtempSync(join(tmpdir(), 'streaming-tree-i18n-'));
    fixtureDir = root;

    for (const [language, content] of [
      ['en', en],
      ['pl', pl],
    ] as const) {
      mkdirSync(join(root, language), { recursive: true });
      writeFileSync(join(root, language, 'sample.json'), JSON.stringify(content), 'utf8');
    }

    return root;
  }

  function typesOf(root: string): string[] {
    return checkTranslationResources(root).issues.map((issue) => issue.type);
  }

  it('accepts a matching pair of resources', () => {
    const root = createFixture({ greeting: 'Hello' }, { greeting: 'Witaj' });

    expect(checkTranslationResources(root).issues).toEqual([]);
  });

  it('detects a key missing from Polish', () => {
    const root = createFixture({ greeting: 'Hello', farewell: 'Bye' }, { greeting: 'Witaj' });
    const { issues } = checkTranslationResources(root);

    expect(issues).toHaveLength(1);
    expect(issues[0]?.type).toBe('missing-key');
    expect(issues[0]?.path).toBe('sample.farewell');
  });

  it('detects a key that exists only in Polish', () => {
    const root = createFixture({ greeting: 'Hello' }, { greeting: 'Witaj', extra: 'Nadmiar' });

    expect(typesOf(root)).toEqual(['extra-key']);
  });

  it('detects incompatible structures at the same path', () => {
    const root = createFixture({ section: { nested: 'Hello' } }, { section: 'Witaj' });
    const { issues } = checkTranslationResources(root);

    expect(issues).toHaveLength(1);
    expect(issues[0]?.type).toBe('structure-mismatch');
    expect(issues[0]?.path).toBe('sample.section');
  });

  it('detects empty translation values', () => {
    const root = createFixture({ greeting: 'Hello' }, { greeting: '   ' });
    const { issues } = checkTranslationResources(root);

    expect(issues).toHaveLength(1);
    expect(issues[0]?.type).toBe('empty-value');
    expect(issues[0]?.path).toBe('sample.greeting');
  });

  it('detects nested keys, not just top-level ones', () => {
    const root = createFixture(
      { a: { b: { c: 'Deep' } } },
      { a: { b: {} } },
    );
    const { issues } = checkTranslationResources(root);

    expect(issues[0]?.path).toBe('sample.a.b.c');
  });

  it('requires the plural categories the language actually uses', () => {
    // English needs one/other; Polish additionally needs few/many.
    const root = createFixture(
      { items_one: '{{count}} item', items_other: '{{count}} items' },
      { items_one: '{{count}} element', items_other: '{{count}} elementu' },
    );
    const { issues } = checkTranslationResources(root);

    expect(issues.map((issue) => issue.type)).toEqual([
      'missing-plural-form',
      'missing-plural-form',
    ]);
    expect(issues.map((issue) => issue.path).sort()).toEqual([
      'sample.items_few',
      'sample.items_many',
    ]);
  });

  it('accepts a complete Polish plural group', () => {
    const root = createFixture(
      { items_one: '{{count}} item', items_other: '{{count}} items' },
      {
        items_one: '{{count}} element',
        items_few: '{{count}} elementy',
        items_many: '{{count}} elementów',
        items_other: '{{count}} elementu',
      },
    );

    expect(checkTranslationResources(root).issues).toEqual([]);
  });
});
