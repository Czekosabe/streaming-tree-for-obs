import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import type { RenderableLayer } from './VisualLayer';
import { VisualDesignRenderer, type VisualDesignAlertData } from './VisualDesignRenderer';

const canvas = { width: 1920, height: 1080, transparent: true };

function baseAlert(overrides: Partial<VisualDesignAlertData> = {}): VisualDesignAlertData {
  return {
    eventType: 'follow', providerId: 'twitch', username: 'Ann', message: null, quantity: null,
    groupCount: 1, renderedText: 'Ann followed!', avatarUrl: null,
    ...overrides,
  };
}

function textLayer(overrides: Partial<RenderableLayer> = {}): RenderableLayer {
  return {
    id: 'layer_text', kind: 'text', frame: { x: 0, y: 0, width: 400, height: 100 }, opacity: 1,
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

function shapeLayer(overrides: Partial<RenderableLayer> = {}): RenderableLayer {
  return {
    id: 'layer_shape', kind: 'shape', frame: { x: 0, y: 0, width: 400, height: 200 }, opacity: 1,
    shape: { kind: 'rectangle', fill: '#112233', borderColor: '#000000', borderWidth: 0, cornerRadius: 8 },
    entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    ...overrides,
  };
}

describe('VisualDesignRenderer', () => {
  it('renders a text layer bound to alert_rendered_text', () => {
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[textLayer()]} alert={baseAlert()} mode="public" prefersReducedMotion={false} />,
    );
    expect(screen.getByText('Ann followed!')).toBeInTheDocument();
  });

  it('renders a shape layer', () => {
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[shapeLayer()]} alert={baseAlert()} mode="public" prefersReducedMotion={false} />,
    );
    expect(screen.getByTestId('visual-design-shape')).toBeInTheDocument();
  });

  it('renders a platform_icon layer using the application-owned glyph, never an image', () => {
    const layer: RenderableLayer = {
      id: 'layer_icon', kind: 'platform_icon', frame: { x: 0, y: 0, width: 64, height: 64 }, opacity: 1,
      platformIcon: {}, entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} alert={baseAlert()} mode="public" prefersReducedMotion={false} />,
    );
    expect(screen.getByTestId('visual-design-platform-icon')).toBeInTheDocument();
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
  });

  it('never renders an avatar layer when avatarUrl is absent (the honest default for every real event today)', () => {
    const layer: RenderableLayer = {
      id: 'layer_avatar', kind: 'avatar', frame: { x: 0, y: 0, width: 96, height: 96 }, opacity: 1,
      avatar: { cornerRadius: 48, borderColor: '#FFFFFF', borderWidth: 0 },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} alert={baseAlert({ avatarUrl: null })} mode="public" prefersReducedMotion={false} />,
    );
    expect(screen.queryByTestId('visual-design-avatar')).not.toBeInTheDocument();
  });

  it('renders an avatar layer for a safe https URL', () => {
    const layer: RenderableLayer = {
      id: 'layer_avatar', kind: 'avatar', frame: { x: 0, y: 0, width: 96, height: 96 }, opacity: 1,
      avatar: { cornerRadius: 48, borderColor: '#FFFFFF', borderWidth: 0 },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[layer]}
        alert={baseAlert({ avatarUrl: 'https://static-cdn.example/avatar.png' })}
        mode="public"
        prefersReducedMotion={false}
      />,
    );
    expect(screen.getByTestId('visual-design-avatar')).toBeInTheDocument();
  });

  it('never renders an avatar layer for a non-https URL, even if somehow present', () => {
    const layer: RenderableLayer = {
      id: 'layer_avatar', kind: 'avatar', frame: { x: 0, y: 0, width: 96, height: 96 }, opacity: 1,
      avatar: { cornerRadius: 48, borderColor: '#FFFFFF', borderWidth: 0 },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[layer]}
        alert={baseAlert({ avatarUrl: 'javascript:alert(1)' })}
        mode="public"
        prefersReducedMotion={false}
      />,
    );
    expect(screen.queryByTestId('visual-design-avatar')).not.toBeInTheDocument();
  });

  it('never renders a hidden (visible=false) layer, even in preview mode', () => {
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[textLayer({ visible: false })]}
        alert={baseAlert()}
        mode="preview"
        prefersReducedMotion={false}
      />,
    );
    expect(screen.queryByText('Ann followed!')).not.toBeInTheDocument();
  });

  it('public mode hides a text layer whose bound value is absent', () => {
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[textLayer({ text: { ...textLayer().text!, binding: 'quantity' } })]}
        alert={baseAlert({ quantity: null })}
        mode="public"
        prefersReducedMotion={false}
      />,
    );
    expect(screen.queryByTestId('visual-design-text')).not.toBeInTheDocument();
    expect(screen.queryByTestId('visual-design-text-missing')).not.toBeInTheDocument();
  });

  it('preview mode shows a synthetic missing-data placeholder when missingValueBehavior is "placeholder"', () => {
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[textLayer({ text: { ...textLayer().text!, binding: 'quantity', missingValueBehavior: 'placeholder' } })]}
        alert={baseAlert({ quantity: null })}
        mode="preview"
        prefersReducedMotion={false}
      />,
    );
    expect(screen.getByTestId('visual-design-text-missing')).toBeInTheDocument();
  });

  it('renders multiple layers in the given order', () => {
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[shapeLayer({ id: 'a' }), textLayer({ id: 'b' })]}
        alert={baseAlert()}
        mode="public"
        prefersReducedMotion={false}
      />,
    );
    const layerEls = screen.getAllByTestId('visual-design-layer');
    expect(layerEls.map((el) => el.getAttribute('data-layer-id'))).toEqual(['a', 'b']);
  });
});
