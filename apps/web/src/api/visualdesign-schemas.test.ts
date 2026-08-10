import { describe, expect, it } from 'vitest';

import {
  publicVisualDesignDocumentSchema,
  visualDesignDocumentSchema,
  visualDesignLayerKindSchema,
  visualDesignResponseSchema,
  visualDesignTextBindingSchema,
} from './visualdesign-schemas';

function shapeLayer(overrides: Record<string, unknown> = {}) {
  return {
    id: 'layer_1', name: 'Background', kind: 'shape', visible: true, locked: false, order: 0,
    frame: { x: 0, y: 0, width: 400, height: 200 }, opacity: 1,
    shape: { kind: 'rectangle', fill: '#112233', borderColor: '#000000', borderWidth: 0, cornerRadius: 8 },
    entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 300,
    ...overrides,
  };
}

function textLayer(overrides: Record<string, unknown> = {}) {
  return {
    id: 'layer_2', name: 'Text', kind: 'text', visible: true, locked: false, order: 1,
    frame: { x: 10, y: 10, width: 380, height: 100 }, opacity: 1,
    text: {
      binding: 'alert_rendered_text', missingValueBehavior: 'hide',
      fontFamily: 'system-ui', fontSize: 32, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
      textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
      outlineWidth: 0, outlineColor: '#000000',
      shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
    },
    entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    ...overrides,
  };
}

describe('visualDesignLayerKindSchema / visualDesignTextBindingSchema', () => {
  it.each(['shape', 'text', 'platform_icon', 'avatar'])('accepts layer kind %s', (kind) => {
    expect(visualDesignLayerKindSchema.parse(kind)).toBe(kind);
  });
  it.each([
    'static', 'alert_rendered_text', 'username', 'platform', 'event_type', 'message', 'quantity', 'group_count',
  ])('accepts text binding %s', (binding) => {
    expect(visualDesignTextBindingSchema.parse(binding)).toBe(binding);
  });
  it('rejects an unknown binding', () => {
    expect(visualDesignTextBindingSchema.safeParse('event.user.name').success).toBe(false);
  });
});

describe('visualDesignDocumentSchema', () => {
  it('parses a document with a shape and a text layer', () => {
    const doc = visualDesignDocumentSchema.parse({
      version: 1,
      canvas: { width: 1920, height: 1080, transparent: true },
      layers: [shapeLayer(), textLayer()],
    });
    expect(doc.layers).toHaveLength(2);
    expect(doc.layers[0]?.name).toBe('Background');
  });

  it('parses a platform_icon layer with an empty payload object', () => {
    const doc = visualDesignDocumentSchema.parse({
      version: 1,
      canvas: { width: 1920, height: 1080, transparent: true },
      layers: [{
        id: 'layer_3', name: 'Icon', kind: 'platform_icon', visible: true, locked: false, order: 0,
        frame: { x: 0, y: 0, width: 64, height: 64 }, opacity: 1, platformIcon: {},
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      }],
    });
    expect(doc.layers[0]?.kind).toBe('platform_icon');
  });

  it('rejects a layer with no name (management schema requires it)', () => {
    const bad = shapeLayer();
    delete (bad as Record<string, unknown>).name;
    const result = visualDesignDocumentSchema.safeParse({
      version: 1, canvas: { width: 1920, height: 1080, transparent: true }, layers: [bad],
    });
    expect(result.success).toBe(false);
  });
});

describe('visualDesignResponseSchema', () => {
  it('parses an unpersisted draft response', () => {
    const parsed = visualDesignResponseSchema.parse({
      persisted: false, revision: 0,
      document: { version: 1, canvas: { width: 1920, height: 1080, transparent: true }, layers: [textLayer()] },
    });
    expect(parsed.persisted).toBe(false);
    expect(parsed.revision).toBe(0);
  });

  it('parses a persisted response with a real revision', () => {
    const parsed = visualDesignResponseSchema.parse({
      persisted: true, revision: 3,
      document: { version: 1, canvas: { width: 1080, height: 1920, transparent: true }, layers: [] },
    });
    expect(parsed.revision).toBe(3);
  });
});

describe('publicVisualDesignDocumentSchema', () => {
  it('parses the public layer shape', () => {
    const parsed = publicVisualDesignDocumentSchema.parse({
      schemaVersion: 1,
      canvas: { width: 1920, height: 1080, transparent: true },
      layers: [{
        id: 'layer_2', kind: 'text',
        frame: { x: 10, y: 10, width: 380, height: 100 }, opacity: 1,
        text: {
          binding: 'alert_rendered_text', missingValueBehavior: 'hide',
          fontFamily: 'system-ui', fontSize: 32, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
          textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
          outlineWidth: 0, outlineColor: '#000000',
          shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
        },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      }],
    });
    expect(parsed.layers).toHaveLength(1);
  });

  it('strips a management-only name/locked field even if a server response somehow carried one (defense in depth)', () => {
    const parsed = publicVisualDesignDocumentSchema.parse({
      schemaVersion: 1,
      canvas: { width: 1920, height: 1080, transparent: true },
      layers: [{
        id: 'layer_1', name: 'Should never appear', locked: true, kind: 'shape',
        frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
        shape: { kind: 'rectangle', fill: '#000000', borderColor: '#000000', borderWidth: 0, cornerRadius: 0 },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      }],
    });
    expect('name' in parsed.layers[0]!).toBe(false);
    expect('locked' in parsed.layers[0]!).toBe(false);
  });
});
