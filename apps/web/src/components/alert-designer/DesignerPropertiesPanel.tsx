import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type {
  VisualDesignAvatarProps,
  VisualDesignBadgeListProps,
  VisualDesignCanvas,
  VisualDesignImageProps,
  VisualDesignLayer,
  VisualDesignMessageFragmentsProps,
  VisualDesignShapeProps,
  VisualDesignTextBinding,
  VisualDesignTextProps,
  VisualDesignVideoProps,
} from '@/api/visualdesign-schemas';
import type { VisualAsset } from '@/api/visualasset-schemas';
import { Button } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { VisualAssetPicker } from '@/components/visual-design/VisualAssetPicker';
import {
  CANVAS_CHAT_ITEM,
  CANVAS_LANDSCAPE,
  CANVAS_VERTICAL,
  MAX_BADGE_COUNT,
  MAX_BADGE_GAP,
  MAX_BADGE_SIZE,
  MAX_BORDER_WIDTH,
  MAX_CORNER_RADIUS,
  MAX_EMOTE_SIZE,
  MAX_FONT_SIZE,
  MAX_LAYER_NAME_CODE_POINTS,
  MAX_OUTLINE_WIDTH,
  MAX_SHADOW_BLUR,
  MIN_BADGE_COUNT,
  MIN_BADGE_GAP,
  MIN_BADGE_SIZE,
  MIN_BORDER_WIDTH,
  MIN_CORNER_RADIUS,
  MIN_EMOTE_SIZE,
  MIN_FONT_SIZE,
  MIN_OUTLINE_WIDTH,
  MIN_SHADOW_BLUR,
  VISUAL_DESIGN_FONT_FAMILIES,
  VISUAL_DESIGN_TEXT_BINDINGS,
} from '@/models/visualdesign';

function NumberField({
  label,
  value,
  min,
  max,
  step = 1,
  onChange,
  testId,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step?: number;
  onChange: (value: number) => void;
  testId: string;
}) {
  return (
    <FormField label={label}>
      {({ inputId }) => (
        <TextInput
          id={inputId}
          type="number"
          min={min}
          max={max}
          step={step}
          value={value}
          onChange={(e) => onChange(Number(e.target.value))}
          data-testid={testId}
        />
      )}
    </FormField>
  );
}

function ColorField({ label, value, onChange, testId }: { label: string; value: string; onChange: (value: string) => void; testId: string }) {
  return (
    <FormField label={label}>
      {({ inputId }) => (
        <div className="flex items-center gap-2">
          <input
            type="color"
            aria-hidden
            tabIndex={-1}
            value={value.length >= 7 ? value.slice(0, 7) : '#000000'}
            onChange={(e) => onChange(e.target.value)}
            className="h-8 w-8 shrink-0 rounded border border-line"
          />
          <TextInput id={inputId} value={value} onChange={(e) => onChange(e.target.value)} data-testid={testId} />
        </div>
      )}
    </FormField>
  );
}

/**
 * Right panel: canvas properties when nothing is selected, the
 * selected layer's own properties otherwise (Stage 13A task Part
 * 26/30) - the numeric X/Y/Width/Height inputs here are the always-
 * available accessible fallback to pointer drag/resize (Part 29/30).
 */
export function DesignerPropertiesPanel({
  canvas,
  onCanvasChange,
  layer,
  availableBindings,
  onLayerChange,
  snapping,
  onSnappingChange,
}: {
  canvas: VisualDesignCanvas;
  onCanvasChange: (canvas: VisualDesignCanvas) => void;
  layer: VisualDesignLayer | null;
  /** The subset of the closed text-binding vocabulary meaningful for
   * the layer's own owner (an alert rule's event-type capability, or a
   * chat overlay's own fixed item-kind union - see
   * models/visualdesign.ts's own `availableTextBindings`/
   * `CHAT_VISUAL_DESIGN_TEXT_BINDINGS`). Owner-agnostic here: this
   * panel is shared by both designers (Stage 13B task Part 25). */
  availableBindings: readonly VisualDesignTextBinding[];
  onLayerChange: (patch: Partial<VisualDesignLayer>) => void;
  snapping: boolean;
  onSnappingChange: (snapping: boolean) => void;
}) {
  const { t } = useTranslation('alertDesigner');
  const [assetRequest, setAssetRequest] = useState<{ kind: 'image' | 'video' | 'font'; onSelect: (asset: VisualAsset) => void } | null>(
    null,
  );

  return (
    <div className="w-72 shrink-0 overflow-y-auto border-l border-line bg-surface p-3" data-testid="designer-properties-panel">
      <ToggleSwitch label="Snap to guides" checked={snapping} onCheckedChange={onSnappingChange} />

      {layer === null ? (
        <CanvasProperties canvas={canvas} onChange={onCanvasChange} />
      ) : (
        <LayerProperties
          layer={layer}
          availableBindings={availableBindings}
          onChange={onLayerChange}
          onRequestAsset={(kind, onSelect) => setAssetRequest({ kind, onSelect })}
        />
      )}
      {layer === null ? <p className="mt-3 text-xs text-ink-muted">{t('properties.noSelection')}</p> : null}

      {assetRequest !== null && (
        <VisualAssetPicker
          open
          onClose={() => setAssetRequest(null)}
          kind={assetRequest.kind}
          onSelect={(asset) => {
            assetRequest.onSelect(asset);
            setAssetRequest(null);
          }}
        />
      )}
    </div>
  );
}

/** Requests a real managed asset from the operator before applying
 * `onSelect` - shared by image/video "change asset" and every text-
 * capable layer's own "choose custom font" (Stage 14B task Part 32/54:
 * one shared picker, never a second implementation per field). */
type RequestAsset = (kind: 'image' | 'video' | 'font', onSelect: (asset: VisualAsset) => void) => void;

/** Every built-in canvas preset both designers offer (Stage 13B task
 * Part 25) - Landscape/Vertical from Stage 13A, Chat item added in
 * Stage 13B. Available in both designers rather than gated per owner:
 * a preset is just a starting size, never itself owner-specific. */
const CANVAS_PRESETS = [
  { value: 'landscape', size: CANVAS_LANDSCAPE, labelKey: 'properties.presetLandscape' },
  { value: 'vertical', size: CANVAS_VERTICAL, labelKey: 'properties.presetVertical' },
  { value: 'chat_item', size: CANVAS_CHAT_ITEM, labelKey: 'properties.presetChatItem' },
] as const;

function CanvasProperties({ canvas, onChange }: { canvas: VisualDesignCanvas; onChange: (canvas: VisualDesignCanvas) => void }) {
  const { t } = useTranslation('alertDesigner');
  const matched = CANVAS_PRESETS.find((p) => p.size.width === canvas.width && p.size.height === canvas.height);
  const preset = matched?.value ?? 'custom';

  return (
    <div className="mt-3 space-y-3">
      <p className="text-xs font-semibold uppercase text-ink-muted">{t('properties.canvasTitle')}</p>
      <FormField label={t('properties.canvasPreset')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={preset}
            data-testid="designer-canvas-preset"
            options={[
              ...CANVAS_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) })),
              { value: 'custom', label: t('properties.presetCustom') },
            ]}
            onChange={(e) => {
              const found = CANVAS_PRESETS.find((p) => p.value === e.target.value);
              if (found !== undefined) onChange({ ...canvas, width: found.size.width, height: found.size.height });
            }}
          />
        )}
      </FormField>
      <NumberField label={t('properties.canvasWidth')} value={canvas.width} min={320} max={3840} onChange={(width) => onChange({ ...canvas, width })} testId="designer-canvas-width" />
      <NumberField label={t('properties.canvasHeight')} value={canvas.height} min={240} max={3840} onChange={(height) => onChange({ ...canvas, height })} testId="designer-canvas-height" />
    </div>
  );
}

function LayerProperties({
  layer,
  availableBindings,
  onChange,
  onRequestAsset,
}: {
  layer: VisualDesignLayer;
  availableBindings: readonly VisualDesignTextBinding[];
  onChange: (patch: Partial<VisualDesignLayer>) => void;
  onRequestAsset: RequestAsset;
}) {
  const { t } = useTranslation('alertDesigner');
  const available = availableBindings;

  return (
    <div className="mt-3 space-y-3">
      <p className="text-xs font-semibold uppercase text-ink-muted">{t('properties.title')}</p>

      <FormField label={t('properties.layerName')} error={layer.name.length > MAX_LAYER_NAME_CODE_POINTS ? 'Too long' : undefined}>
        {({ inputId }) => (
          <TextInput id={inputId} value={layer.name} onChange={(e) => onChange({ name: e.target.value })} data-testid="designer-layer-name" />
        )}
      </FormField>

      <div className="grid grid-cols-2 gap-2">
        <NumberField label={t('properties.x')} value={layer.frame.x} min={0} max={3840} onChange={(x) => onChange({ frame: { ...layer.frame, x } })} testId="designer-layer-x" />
        <NumberField label={t('properties.y')} value={layer.frame.y} min={0} max={3840} onChange={(y) => onChange({ frame: { ...layer.frame, y } })} testId="designer-layer-y" />
        <NumberField label={t('properties.width')} value={layer.frame.width} min={8} max={3840} onChange={(width) => onChange({ frame: { ...layer.frame, width } })} testId="designer-layer-width" />
        <NumberField label={t('properties.height')} value={layer.frame.height} min={8} max={3840} onChange={(height) => onChange({ frame: { ...layer.frame, height } })} testId="designer-layer-height" />
      </div>

      <NumberField label={t('properties.opacity')} value={layer.opacity} min={0} max={1} step={0.05} onChange={(opacity) => onChange({ opacity })} testId="designer-layer-opacity" />

      {layer.kind === 'shape' && layer.shape !== undefined ? (
        <ShapeProperties shape={layer.shape} onChange={(shape) => onChange({ shape })} />
      ) : null}
      {layer.kind === 'text' && layer.text !== undefined ? (
        <TextProperties text={layer.text} available={available} onChange={(text) => onChange({ text })} onRequestAsset={onRequestAsset} />
      ) : null}
      {layer.kind === 'avatar' && layer.avatar !== undefined ? (
        <AvatarProperties avatar={layer.avatar} onChange={(avatar) => onChange({ avatar })} />
      ) : null}
      {layer.kind === 'message_fragments' && layer.messageFragments !== undefined ? (
        <MessageFragmentsProperties
          messageFragments={layer.messageFragments}
          onChange={(messageFragments) => onChange({ messageFragments })}
          onRequestAsset={onRequestAsset}
        />
      ) : null}
      {layer.kind === 'badge_list' && layer.badgeList !== undefined ? (
        <BadgeListProperties badgeList={layer.badgeList} onChange={(badgeList) => onChange({ badgeList })} />
      ) : null}
      {layer.kind === 'image' && layer.image !== undefined ? (
        <ImageProperties image={layer.image} onChange={(image) => onChange({ image })} onRequestAsset={onRequestAsset} />
      ) : null}
      {layer.kind === 'video' && layer.video !== undefined ? (
        <VideoProperties video={layer.video} onChange={(video) => onChange({ video })} onRequestAsset={onRequestAsset} />
      ) : null}

      <FormField label={t('properties.entryAnimation')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={layer.entryAnimation}
            onChange={(e) => onChange({ entryAnimation: e.target.value as VisualDesignLayer['entryAnimation'] })}
            options={['none', 'fade', 'slide_up', 'slide_left', 'scale'].map((v) => ({ value: v, label: v }))}
            data-testid="designer-layer-entry-animation"
          />
        )}
      </FormField>
      <FormField label={t('properties.exitAnimation')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={layer.exitAnimation}
            onChange={(e) => onChange({ exitAnimation: e.target.value as VisualDesignLayer['exitAnimation'] })}
            options={['none', 'fade', 'slide_up', 'slide_left', 'scale'].map((v) => ({ value: v, label: v }))}
            data-testid="designer-layer-exit-animation"
          />
        )}
      </FormField>
      <NumberField
        label={t('properties.animationDuration')}
        value={layer.animationDurationMs}
        min={0}
        max={2000}
        step={50}
        onChange={(animationDurationMs) => onChange({ animationDurationMs })}
        testId="designer-layer-animation-duration"
      />
    </div>
  );
}

function ShapeProperties({ shape, onChange }: { shape: VisualDesignShapeProps; onChange: (shape: VisualDesignShapeProps) => void }) {
  const { t } = useTranslation('alertDesigner');
  return (
    <>
      <ColorField label={t('properties.fill')} value={shape.fill} onChange={(fill) => onChange({ ...shape, fill })} testId="designer-shape-fill" />
      <ColorField label={t('properties.borderColor')} value={shape.borderColor} onChange={(borderColor) => onChange({ ...shape, borderColor })} testId="designer-shape-border-color" />
      <NumberField label={t('properties.borderWidth')} value={shape.borderWidth} min={MIN_BORDER_WIDTH} max={MAX_BORDER_WIDTH} onChange={(borderWidth) => onChange({ ...shape, borderWidth })} testId="designer-shape-border-width" />
      <NumberField label={t('properties.cornerRadius')} value={shape.cornerRadius} min={MIN_CORNER_RADIUS} max={MAX_CORNER_RADIUS} onChange={(cornerRadius) => onChange({ ...shape, cornerRadius })} testId="designer-shape-corner-radius" />
    </>
  );
}

function AvatarProperties({ avatar, onChange }: { avatar: VisualDesignAvatarProps; onChange: (avatar: VisualDesignAvatarProps) => void }) {
  const { t } = useTranslation('alertDesigner');
  return (
    <>
      <NumberField label={t('properties.cornerRadius')} value={avatar.cornerRadius} min={MIN_CORNER_RADIUS} max={MAX_CORNER_RADIUS} onChange={(cornerRadius) => onChange({ ...avatar, cornerRadius })} testId="designer-avatar-corner-radius" />
      <ColorField label={t('properties.borderColor')} value={avatar.borderColor} onChange={(borderColor) => onChange({ ...avatar, borderColor })} testId="designer-avatar-border-color" />
      <NumberField label={t('properties.borderWidth')} value={avatar.borderWidth} min={MIN_BORDER_WIDTH} max={MAX_BORDER_WIDTH} onChange={(borderWidth) => onChange({ ...avatar, borderWidth })} testId="designer-avatar-border-width" />
    </>
  );
}

function ImageProperties({
  image,
  onChange,
  onRequestAsset,
}: {
  image: VisualDesignImageProps;
  onChange: (value: VisualDesignImageProps) => void;
  onRequestAsset: RequestAsset;
}) {
  const { t } = useTranslation('alertDesigner');
  return (
    <>
      <Button
        variant="secondary"
        onClick={() => onRequestAsset('image', (asset) => onChange({ ...image, assetId: asset.id }))}
        data-testid="designer-image-change-asset"
      >
        {t('properties.changeAsset')}
      </Button>
      <FormField label={t('properties.imageFit')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={image.fit}
            onChange={(e) => onChange({ ...image, fit: e.target.value as VisualDesignImageProps['fit'] })}
            options={[
              { value: 'contain', label: t('properties.fitContain') },
              { value: 'cover', label: t('properties.fitCover') },
            ]}
            data-testid="designer-image-fit"
          />
        )}
      </FormField>
      <FormField label={t('properties.imageAlt')}>
        {({ inputId }) => (
          <TextInput id={inputId} value={image.alt ?? ''} onChange={(e) => onChange({ ...image, alt: e.target.value })} data-testid="designer-image-alt" />
        )}
      </FormField>
    </>
  );
}

function VideoProperties({
  video,
  onChange,
  onRequestAsset,
}: {
  video: VisualDesignVideoProps;
  onChange: (value: VisualDesignVideoProps) => void;
  onRequestAsset: RequestAsset;
}) {
  const { t } = useTranslation('alertDesigner');
  return (
    <>
      <Button
        variant="secondary"
        onClick={() => onRequestAsset('video', (asset) => onChange({ ...video, assetId: asset.id }))}
        data-testid="designer-video-change-asset"
      >
        {t('properties.changeVideoAsset')}
      </Button>
      <FormField label={t('properties.videoFit')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={video.fit}
            onChange={(e) => onChange({ ...video, fit: e.target.value as VisualDesignVideoProps['fit'] })}
            options={[
              { value: 'contain', label: t('properties.fitContain') },
              { value: 'cover', label: t('properties.fitCover') },
            ]}
            data-testid="designer-video-fit"
          />
        )}
      </FormField>
      <ToggleSwitch label={t('properties.videoLoop')} checked={video.loop} onCheckedChange={(loop) => onChange({ ...video, loop })} />
    </>
  );
}

/** Shared by TextProperties/MessageFragmentsProperties - the optional
 * custom-WOFF2-font control every text-capable layer offers (Stage 14B
 * task Part 41/54). */
function CustomFontField({
  fontAssetId,
  onChange,
  onRequestAsset,
}: {
  fontAssetId: string | undefined;
  onChange: (fontAssetId: string | undefined) => void;
  onRequestAsset: RequestAsset;
}) {
  const { t } = useTranslation('alertDesigner');
  return (
    <FormField label={t('properties.customFont')}>
      {() =>
        fontAssetId === undefined || fontAssetId === '' ? (
          <Button variant="secondary" onClick={() => onRequestAsset('font', (asset) => onChange(asset.id))} data-testid="designer-choose-custom-font">
            {t('properties.chooseCustomFont')}
          </Button>
        ) : (
          <Button variant="secondary" onClick={() => onChange(undefined)} data-testid="designer-remove-custom-font">
            {t('properties.removeCustomFont')}
          </Button>
        )
      }
    </FormField>
  );
}

function MessageFragmentsProperties({
  messageFragments,
  onChange,
  onRequestAsset,
}: {
  messageFragments: VisualDesignMessageFragmentsProps;
  onChange: (value: VisualDesignMessageFragmentsProps) => void;
  onRequestAsset: RequestAsset;
}) {
  const { t } = useTranslation('alertDesigner');
  return (
    <>
      <FormField label={t('properties.fontFamily')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={messageFragments.fontFamily}
            onChange={(e) => onChange({ ...messageFragments, fontFamily: e.target.value as VisualDesignMessageFragmentsProps['fontFamily'] })}
            options={VISUAL_DESIGN_FONT_FAMILIES.map((f) => ({ value: f, label: f }))}
            data-testid="designer-fragments-font-family"
          />
        )}
      </FormField>
      <div className="grid grid-cols-2 gap-2">
        <NumberField label={t('properties.fontSize')} value={messageFragments.fontSize} min={MIN_FONT_SIZE} max={MAX_FONT_SIZE} onChange={(fontSize) => onChange({ ...messageFragments, fontSize })} testId="designer-fragments-font-size" />
        <NumberField label={t('properties.fontWeight')} value={messageFragments.fontWeight} min={100} max={900} step={100} onChange={(fontWeight) => onChange({ ...messageFragments, fontWeight })} testId="designer-fragments-font-weight" />
      </div>
      <ColorField label={t('properties.textColor')} value={messageFragments.textColor} onChange={(textColor) => onChange({ ...messageFragments, textColor })} testId="designer-fragments-text-color" />
      <FormField label={t('properties.horizontalAlign')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={messageFragments.horizontalAlign}
            onChange={(e) => onChange({ ...messageFragments, horizontalAlign: e.target.value as VisualDesignMessageFragmentsProps['horizontalAlign'] })}
            options={[
              { value: 'left', label: t('properties.alignLeft') },
              { value: 'center', label: t('properties.alignCenter') },
              { value: 'right', label: t('properties.alignRight') },
            ]}
            data-testid="designer-fragments-halign"
          />
        )}
      </FormField>
      <NumberField label={t('properties.emoteSize')} value={messageFragments.emoteSize} min={MIN_EMOTE_SIZE} max={MAX_EMOTE_SIZE} onChange={(emoteSize) => onChange({ ...messageFragments, emoteSize })} testId="designer-fragments-emote-size" />
      <CustomFontField
        fontAssetId={messageFragments.fontAssetId}
        onChange={(fontAssetId) => onChange({ ...messageFragments, fontAssetId })}
        onRequestAsset={onRequestAsset}
      />
    </>
  );
}

function BadgeListProperties({
  badgeList,
  onChange,
}: {
  badgeList: VisualDesignBadgeListProps;
  onChange: (value: VisualDesignBadgeListProps) => void;
}) {
  const { t } = useTranslation('alertDesigner');
  return (
    <>
      <NumberField label={t('properties.badgeMaxCount')} value={badgeList.maxCount} min={MIN_BADGE_COUNT} max={MAX_BADGE_COUNT} onChange={(maxCount) => onChange({ ...badgeList, maxCount })} testId="designer-badges-max-count" />
      <NumberField label={t('properties.badgeSize')} value={badgeList.badgeSize} min={MIN_BADGE_SIZE} max={MAX_BADGE_SIZE} onChange={(badgeSize) => onChange({ ...badgeList, badgeSize })} testId="designer-badges-size" />
      <NumberField label={t('properties.badgeGap')} value={badgeList.gap} min={MIN_BADGE_GAP} max={MAX_BADGE_GAP} onChange={(gap) => onChange({ ...badgeList, gap })} testId="designer-badges-gap" />
    </>
  );
}

function TextProperties({
  text,
  available,
  onChange,
  onRequestAsset,
}: {
  text: VisualDesignTextProps;
  available: readonly string[];
  onChange: (text: VisualDesignTextProps) => void;
  onRequestAsset: RequestAsset;
}) {
  const { t } = useTranslation('alertDesigner');
  const unavailable = !available.includes(text.binding);

  return (
    <>
      <FormField label={t('properties.textBinding')} error={unavailable ? t('layers.unavailableBindingWarning') : undefined}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={text.binding}
            onChange={(e) => onChange({ ...text, binding: e.target.value as VisualDesignTextProps['binding'] })}
            options={VISUAL_DESIGN_TEXT_BINDINGS.map((b) => ({
              value: b,
              label: available.includes(b) ? t(`layers.textVariant.${b}`) : `${t(`layers.textVariant.${b}`)} ⚠`,
            }))}
            data-testid="designer-text-binding"
          />
        )}
      </FormField>

      {text.binding === 'static' ? (
        <FormField label={t('properties.staticText')} counter={`${text.staticText?.length ?? 0} / 500`}>
          {({ inputId }) => (
            <TextInput
              id={inputId}
              value={text.staticText ?? ''}
              onChange={(e) => onChange({ ...text, staticText: e.target.value })}
              data-testid="designer-text-static-text"
            />
          )}
        </FormField>
      ) : (
        <FormField label={t('properties.missingValueBehavior')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              value={text.missingValueBehavior}
              onChange={(e) => onChange({ ...text, missingValueBehavior: e.target.value as VisualDesignTextProps['missingValueBehavior'] })}
              options={[
                { value: 'hide', label: t('properties.missingHide') },
                { value: 'placeholder', label: t('properties.missingPlaceholder') },
              ]}
              data-testid="designer-text-missing-behavior"
            />
          )}
        </FormField>
      )}

      <FormField label={t('properties.fontFamily')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={text.fontFamily}
            onChange={(e) => onChange({ ...text, fontFamily: e.target.value as VisualDesignTextProps['fontFamily'] })}
            options={VISUAL_DESIGN_FONT_FAMILIES.map((f) => ({ value: f, label: f }))}
            data-testid="designer-text-font-family"
          />
        )}
      </FormField>
      <div className="grid grid-cols-2 gap-2">
        <NumberField label={t('properties.fontSize')} value={text.fontSize} min={MIN_FONT_SIZE} max={MAX_FONT_SIZE} onChange={(fontSize) => onChange({ ...text, fontSize })} testId="designer-text-font-size" />
        <NumberField label={t('properties.fontWeight')} value={text.fontWeight} min={100} max={900} step={100} onChange={(fontWeight) => onChange({ ...text, fontWeight })} testId="designer-text-font-weight" />
      </div>
      <ColorField label={t('properties.textColor')} value={text.textColor} onChange={(textColor) => onChange({ ...text, textColor })} testId="designer-text-color" />

      <FormField label={t('properties.horizontalAlign')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={text.horizontalAlign}
            onChange={(e) => onChange({ ...text, horizontalAlign: e.target.value as VisualDesignTextProps['horizontalAlign'] })}
            options={[
              { value: 'left', label: t('properties.alignLeft') },
              { value: 'center', label: t('properties.alignCenter') },
              { value: 'right', label: t('properties.alignRight') },
            ]}
            data-testid="designer-text-halign"
          />
        )}
      </FormField>
      <FormField label={t('properties.verticalAlign')}>
        {({ inputId }) => (
          <SelectInput
            id={inputId}
            value={text.verticalAlign}
            onChange={(e) => onChange({ ...text, verticalAlign: e.target.value as VisualDesignTextProps['verticalAlign'] })}
            options={[
              { value: 'top', label: t('properties.alignTop') },
              { value: 'middle', label: t('properties.alignMiddle') },
              { value: 'bottom', label: t('properties.alignBottom') },
            ]}
            data-testid="designer-text-valign"
          />
        )}
      </FormField>

      <NumberField label={t('properties.outlineWidth')} value={text.outlineWidth} min={MIN_OUTLINE_WIDTH} max={MAX_OUTLINE_WIDTH} onChange={(outlineWidth) => onChange({ ...text, outlineWidth })} testId="designer-text-outline-width" />
      {text.outlineWidth > 0 ? (
        <ColorField label={t('properties.outlineColor')} value={text.outlineColor} onChange={(outlineColor) => onChange({ ...text, outlineColor })} testId="designer-text-outline-color" />
      ) : null}

      <ToggleSwitch
        label={t('properties.shadowEnabled')}
        checked={text.shadowEnabled}
        onCheckedChange={(shadowEnabled) => onChange({ ...text, shadowEnabled })}
      />
      {text.shadowEnabled ? (
        <>
          <NumberField label={t('properties.shadowOffsetX')} value={text.shadowOffsetX} min={-32} max={32} onChange={(shadowOffsetX) => onChange({ ...text, shadowOffsetX })} testId="designer-text-shadow-offset-x" />
          <NumberField label={t('properties.shadowOffsetY')} value={text.shadowOffsetY} min={-32} max={32} onChange={(shadowOffsetY) => onChange({ ...text, shadowOffsetY })} testId="designer-text-shadow-offset-y" />
          <NumberField label={t('properties.shadowBlur')} value={text.shadowBlur} min={MIN_SHADOW_BLUR} max={MAX_SHADOW_BLUR} onChange={(shadowBlur) => onChange({ ...text, shadowBlur })} testId="designer-text-shadow-blur" />
          <ColorField label={t('properties.shadowColor')} value={text.shadowColor} onChange={(shadowColor) => onChange({ ...text, shadowColor })} testId="designer-text-shadow-color" />
        </>
      ) : null}

      <CustomFontField fontAssetId={text.fontAssetId} onChange={(fontAssetId) => onChange({ ...text, fontAssetId })} onRequestAsset={onRequestAsset} />
    </>
  );
}
