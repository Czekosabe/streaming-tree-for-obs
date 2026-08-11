import { useTranslation } from 'react-i18next';

import type {
  AlertEventTypeCapability,
} from '@/api/alerts-schemas';
import type {
  VisualDesignAvatarProps,
  VisualDesignCanvas,
  VisualDesignLayer,
  VisualDesignShapeProps,
  VisualDesignTextProps,
} from '@/api/visualdesign-schemas';
import { FormField } from '@/components/ui/FormField';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  availableTextBindings,
  CANVAS_LANDSCAPE,
  CANVAS_VERTICAL,
  MAX_BORDER_WIDTH,
  MAX_CORNER_RADIUS,
  MAX_FONT_SIZE,
  MAX_LAYER_NAME_CODE_POINTS,
  MAX_OUTLINE_WIDTH,
  MAX_SHADOW_BLUR,
  MIN_BORDER_WIDTH,
  MIN_CORNER_RADIUS,
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
  eventTypeCapability,
  onLayerChange,
  snapping,
  onSnappingChange,
}: {
  canvas: VisualDesignCanvas;
  onCanvasChange: (canvas: VisualDesignCanvas) => void;
  layer: VisualDesignLayer | null;
  eventTypeCapability: AlertEventTypeCapability | undefined;
  onLayerChange: (patch: Partial<VisualDesignLayer>) => void;
  snapping: boolean;
  onSnappingChange: (snapping: boolean) => void;
}) {
  const { t } = useTranslation('alertDesigner');

  return (
    <div className="w-72 shrink-0 overflow-y-auto border-l border-line bg-surface p-3" data-testid="designer-properties-panel">
      <ToggleSwitch label="Snap to guides" checked={snapping} onCheckedChange={onSnappingChange} />

      {layer === null ? (
        <CanvasProperties canvas={canvas} onChange={onCanvasChange} />
      ) : (
        <LayerProperties layer={layer} eventTypeCapability={eventTypeCapability} onChange={onLayerChange} />
      )}
      {layer === null ? <p className="mt-3 text-xs text-ink-muted">{t('properties.noSelection')}</p> : null}
    </div>
  );
}

function CanvasProperties({ canvas, onChange }: { canvas: VisualDesignCanvas; onChange: (canvas: VisualDesignCanvas) => void }) {
  const { t } = useTranslation('alertDesigner');
  const preset =
    canvas.width === CANVAS_LANDSCAPE.width && canvas.height === CANVAS_LANDSCAPE.height
      ? 'landscape'
      : canvas.width === CANVAS_VERTICAL.width && canvas.height === CANVAS_VERTICAL.height
        ? 'vertical'
        : 'custom';

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
              { value: 'landscape', label: t('properties.presetLandscape') },
              { value: 'vertical', label: t('properties.presetVertical') },
              { value: 'custom', label: t('properties.presetCustom') },
            ]}
            onChange={(e) => {
              if (e.target.value === 'landscape') onChange({ ...canvas, ...CANVAS_LANDSCAPE });
              else if (e.target.value === 'vertical') onChange({ ...canvas, ...CANVAS_VERTICAL });
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
  eventTypeCapability,
  onChange,
}: {
  layer: VisualDesignLayer;
  eventTypeCapability: AlertEventTypeCapability | undefined;
  onChange: (patch: Partial<VisualDesignLayer>) => void;
}) {
  const { t } = useTranslation('alertDesigner');
  const available = availableTextBindings(eventTypeCapability);

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
        <TextProperties text={layer.text} available={available} onChange={(text) => onChange({ text })} />
      ) : null}
      {layer.kind === 'avatar' && layer.avatar !== undefined ? (
        <AvatarProperties avatar={layer.avatar} onChange={(avatar) => onChange({ avatar })} />
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

function TextProperties({
  text,
  available,
  onChange,
}: {
  text: VisualDesignTextProps;
  available: readonly string[];
  onChange: (text: VisualDesignTextProps) => void;
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
    </>
  );
}
