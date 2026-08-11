import { describe, expect, it } from 'vitest';

import type { VisualDesignDocument } from '@/api/visualdesign-schemas';
import type { VisualTemplate } from '@/api/visualtemplate-schemas';

import { safeTemplateExportFilename, templateHasAssets, toVisualTemplateFile } from './visualtemplate';

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

function baseDoc(): VisualDesignDocument {
  return { version: 3, canvas: { width: 1920, height: 1080, transparent: true }, layers: [] };
}

describe('templateHasAssets', () => {
  it('is false for a document with no asset-referencing layer', () => {
    const doc: VisualDesignDocument = {
      ...baseDoc(),
      layers: [
        {
          id: 'l1', name: 'Rect', kind: 'shape', visible: true, locked: false, order: 0,
          frame: { x: 0, y: 0, width: 10, height: 10 }, opacity: 1,
          shape: { kind: 'rectangle', fill: '#000000', borderColor: '#000000', borderWidth: 0, cornerRadius: 0 },
          entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
        },
      ],
    };
    expect(templateHasAssets(doc)).toBe(false);
  });

  it('is true for an image layer', () => {
    const doc: VisualDesignDocument = {
      ...baseDoc(),
      layers: [
        {
          id: 'l1', name: 'Img', kind: 'image', visible: true, locked: false, order: 0,
          frame: { x: 0, y: 0, width: 10, height: 10 }, opacity: 1,
          image: { assetId: 'asset_1', fit: 'contain' },
          entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
        },
      ],
    };
    expect(templateHasAssets(doc)).toBe(true);
  });

  it('is true for a text layer with a custom font reference', () => {
    const doc: VisualDesignDocument = {
      ...baseDoc(),
      layers: [
        {
          id: 'l1', name: 'Text', kind: 'text', visible: true, locked: false, order: 0,
          frame: { x: 0, y: 0, width: 10, height: 10 }, opacity: 1,
          text: {
            binding: 'static', staticText: 'hi', missingValueBehavior: 'hide', fontFamily: 'system-ui',
            fontAssetId: 'asset_font1', fontSize: 16, fontWeight: 400, lineHeight: 1, letterSpacing: 0,
            textColor: '#fff', horizontalAlign: 'center', verticalAlign: 'middle', outlineWidth: 0,
            outlineColor: '#000', shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000',
          },
          entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
        },
      ],
    };
    expect(templateHasAssets(doc)).toBe(true);
  });
});
