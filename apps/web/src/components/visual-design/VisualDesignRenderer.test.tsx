import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import type { RenderableLayer } from './VisualLayer';
import { VisualDesignRenderer, type VisualDesignDataContext } from './VisualDesignRenderer';

const canvas = { width: 1920, height: 1080, transparent: true };

function baseDataContext(overrides: Partial<VisualDesignDataContext> = {}): VisualDesignDataContext {
  return {
    providerId: 'twitch',
    avatarUrl: null,
    bindings: {
      renderedText: 'Ann followed!', username: 'Ann', platform: 'Twitch', eventType: 'Follow',
      message: null, quantity: null, groupCount: 1, timestamp: null, accountLabel: null,
    },
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
      <VisualDesignRenderer canvas={canvas} layers={[textLayer()]} dataContext={baseDataContext()} mode="public" prefersReducedMotion={false} />,
    );
    expect(screen.getByText('Ann followed!')).toBeInTheDocument();
  });

  it('renders a shape layer', () => {
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[shapeLayer()]} dataContext={baseDataContext()} mode="public" prefersReducedMotion={false} />,
    );
    expect(screen.getByTestId('visual-design-shape')).toBeInTheDocument();
  });

  it('renders a platform_icon layer using the application-owned glyph, never an image', () => {
    const layer: RenderableLayer = {
      id: 'layer_icon', kind: 'platform_icon', frame: { x: 0, y: 0, width: 64, height: 64 }, opacity: 1,
      platformIcon: {}, entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="public" prefersReducedMotion={false} />,
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
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext({ avatarUrl: null })} mode="public" prefersReducedMotion={false} />,
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
        dataContext={baseDataContext({ avatarUrl: 'https://static-cdn.example/avatar.png' })}
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
        dataContext={baseDataContext({ avatarUrl: 'javascript:alert(1)' })}
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
        dataContext={baseDataContext()}
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
        dataContext={baseDataContext({ bindings: { ...baseDataContext().bindings, quantity: null } })}
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
        dataContext={baseDataContext({ bindings: { ...baseDataContext().bindings, quantity: null } })}
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
        dataContext={baseDataContext()}
        mode="public"
        prefersReducedMotion={false}
      />,
    );
    const layerEls = screen.getAllByTestId('visual-design-layer');
    expect(layerEls.map((el) => el.getAttribute('data-layer-id'))).toEqual(['a', 'b']);
  });

  it('renders a message_fragments layer from the data context', () => {
    const layer: RenderableLayer = {
      id: 'layer_fragments', kind: 'message_fragments', frame: { x: 0, y: 0, width: 400, height: 100 }, opacity: 1,
      messageFragments: {
        fontFamily: 'system-ui', fontSize: 16, fontWeight: 400, lineHeight: 1.2, letterSpacing: 0,
        textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'top', emoteSize: 24,
      },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[layer]}
        dataContext={baseDataContext({ messageFragments: [{ type: 'text', text: 'hello chat' }] })}
        mode="public"
        prefersReducedMotion={false}
      />,
    );
    expect(screen.getByText('hello chat')).toBeInTheDocument();
  });

  it('renders a badge_list layer from the data context, bounded to maxCount', () => {
    const layer: RenderableLayer = {
      id: 'layer_badges', kind: 'badge_list', frame: { x: 0, y: 0, width: 200, height: 32 }, opacity: 1,
      badgeList: { maxCount: 1, badgeSize: 18, gap: 4 },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[layer]}
        dataContext={baseDataContext({
          badges: [
            { setId: 'moderator', id: '1', imageUrl1x: 'https://static-cdn.example/mod.png' },
            { setId: 'subscriber', id: '1', imageUrl1x: 'https://static-cdn.example/sub.png' },
          ],
        })}
        mode="public"
        prefersReducedMotion={false}
      />,
    );
    expect(screen.getAllByTestId('visual-design-badge')).toHaveLength(1);
  });

  // --- Stage 14B: image/video layers ------------------------------------

  it('renders an image layer whose url is already resolved (public shape)', () => {
    const layer: RenderableLayer = {
      id: 'layer_img', kind: 'image', frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
      image: { fit: 'contain', alt: 'Badge', url: '/api/public/visual-assets/tok1', mediaType: 'image/png' },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="public" prefersReducedMotion={false} />,
    );
    const img = screen.getByTestId('visual-design-image');
    expect(img).toHaveAttribute('src', '/api/public/visual-assets/tok1');
    expect(img).toHaveAttribute('alt', 'Badge');
  });

  it('resolves an image layer via assetMap when only a local assetId is present (management shape)', () => {
    const layer: RenderableLayer = {
      id: 'layer_img', kind: 'image', frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
      image: { assetId: 'asset_1', fit: 'contain' },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer
        canvas={canvas}
        layers={[layer]}
        dataContext={baseDataContext()}
        mode="preview"
        prefersReducedMotion={false}
        assetMap={{ asset_1: { url: '/api/public/visual-assets/tok-managed', mediaType: 'image/png' } }}
      />,
    );
    expect(screen.getByTestId('visual-design-image')).toHaveAttribute('src', '/api/public/visual-assets/tok-managed');
  });

  it('renders nothing for an image layer whose reference cannot be resolved (fails safe)', () => {
    const layer: RenderableLayer = {
      id: 'layer_img', kind: 'image', frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
      image: { assetId: 'asset_missing', fit: 'contain' },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="preview" prefersReducedMotion={false} />,
    );
    expect(screen.queryByTestId('visual-design-image')).not.toBeInTheDocument();
  });

  it('hides a potentially-animated (GIF/WebP) image layer under prefers-reduced-motion', () => {
    const layer: RenderableLayer = {
      id: 'layer_img', kind: 'image', frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
      image: { fit: 'contain', url: '/api/public/visual-assets/tok-gif', mediaType: 'image/gif' },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="public" prefersReducedMotion />,
    );
    expect(screen.queryByTestId('visual-design-image')).not.toBeInTheDocument();
  });

  it('still renders a static PNG image layer under prefers-reduced-motion', () => {
    const layer: RenderableLayer = {
      id: 'layer_img', kind: 'image', frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
      image: { fit: 'contain', url: '/api/public/visual-assets/tok-png', mediaType: 'image/png' },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="public" prefersReducedMotion />,
    );
    expect(screen.getByTestId('visual-design-image')).toBeInTheDocument();
  });

  it('renders a video layer always muted, playsInline, with no controls', () => {
    const layer: RenderableLayer = {
      id: 'layer_vid', kind: 'video', frame: { x: 0, y: 0, width: 200, height: 200 }, opacity: 1,
      video: { fit: 'cover', loop: true, url: '/api/public/visual-assets/tok-vid' },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="public" prefersReducedMotion={false} />,
    );
    const video = screen.getByTestId('visual-design-video') as HTMLVideoElement;
    expect(video.muted).toBe(true);
    expect(video).toHaveAttribute('playsinline');
    expect(video.controls).toBe(false);
    expect(video.loop).toBe(true);
  });

  it('never autoplays a video layer under prefers-reduced-motion', () => {
    const layer: RenderableLayer = {
      id: 'layer_vid', kind: 'video', frame: { x: 0, y: 0, width: 200, height: 200 }, opacity: 1,
      video: { fit: 'cover', loop: false, url: '/api/public/visual-assets/tok-vid' },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="public" prefersReducedMotion />,
    );
    const video = screen.getByTestId('visual-design-video') as HTMLVideoElement;
    expect(video.autoplay).toBe(false);
  });

  it('renders nothing for a video layer whose reference cannot be resolved (fails safe)', () => {
    const layer: RenderableLayer = {
      id: 'layer_vid', kind: 'video', frame: { x: 0, y: 0, width: 200, height: 200 }, opacity: 1,
      video: { assetId: 'asset_missing', fit: 'cover', loop: false },
      entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
    };
    renderWithProviders(
      <VisualDesignRenderer canvas={canvas} layers={[layer]} dataContext={baseDataContext()} mode="preview" prefersReducedMotion={false} />,
    );
    expect(screen.queryByTestId('visual-design-video')).not.toBeInTheDocument();
  });
});
