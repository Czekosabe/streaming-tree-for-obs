import { describe, expect, it } from 'vitest';

import type { VisualTemplate } from '@/api/visualtemplate-schemas';

import { safeTemplateExportFilename, toVisualTemplateFile } from './visualtemplate';

describe('safeTemplateExportFilename', () => {
  it('appends the fixed Stage 14A extension', () => {
    expect(safeTemplateExportFilename('Minimal Dark')).toBe('Minimal Dark.streaming-tree-template.json');
  });

  it('replaces path separators', () => {
    expect(safeTemplateExportFilename('My/Weird\\Name:Here')).toBe('My-Weird-Name-Here.streaming-tree-template.json');
  });

  it('strips control characters, including CR/LF', () => {
    expect(safeTemplateExportFilename('Evil\r\nHeader-Injection')).toBe('EvilHeader-Injection.streaming-tree-template.json');
  });

  it('bounds the length', () => {
    const long = 'a'.repeat(200);
    const result = safeTemplateExportFilename(long);
    expect(result.length).toBeLessThan(200);
  });

  it('falls back to a generic name when everything is stripped', () => {
    expect(safeTemplateExportFilename('\r\n')).toBe('template.streaming-tree-template.json');
  });
});

describe('toVisualTemplateFile', () => {
  it('converts a management-shape template into the portable file shape', () => {
    const template: VisualTemplate = {
      id: 'tpl_1',
      target: 'chat',
      source: 'user',
      name: 'Mine',
      description: 'd',
      author: 'a',
      license: 'l',
      templateSchemaVersion: 1,
      document: { version: 2, canvas: { width: 960, height: 280, transparent: true }, layers: [] },
    };
    const file = toVisualTemplateFile(template);
    expect(file.format).toBe('streaming-tree-visual-template');
    expect(file.schemaVersion).toBe(1);
    expect(file.target).toBe('chat');
    expect(file.visualDesign).toEqual(template.document);
    expect(file).not.toHaveProperty('id');
    expect(file).not.toHaveProperty('createdAt');
  });
});
