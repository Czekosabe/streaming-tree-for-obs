import { useTranslation } from 'react-i18next';

import type { VisualDesignTextProps } from '@/api/visualdesign-schemas';

import { textLayerContainerStyle, textLayerStyle } from './design-style';
import { resolveTextBindingValue, type AlertBindingContext } from './text-binding';

/**
 * Renders one text layer's resolved value as plain text - never
 * `dangerouslySetInnerHTML` (Stage 13A task Part 45). `mode="public"`
 * always hides a layer whose bound value is absent; `mode="preview"`
 * (editor-only) instead shows an obviously synthetic missing-data
 * indicator when the layer's own `missingValueBehavior` is
 * `"placeholder"` - that indicator is a translated UI string, never
 * something that could be mistaken for real content or accidentally
 * saved as static text.
 */
export function TextLayer({
  text,
  scale,
  context,
  mode,
}: {
  text: VisualDesignTextProps;
  scale: number;
  context: AlertBindingContext;
  mode: 'public' | 'preview';
}) {
  const { t } = useTranslation('alertDesigner');
  const value = resolveTextBindingValue(text, context);

  if (value === null) {
    if (mode === 'public' || text.missingValueBehavior === 'hide') return null;
    return (
      <div style={textLayerContainerStyle(text)} data-testid="visual-design-text-missing">
        <span style={{ ...textLayerStyle(text, scale), opacity: 0.5, fontStyle: 'italic' }}>
          {t('renderer.missingValuePlaceholder')}
        </span>
      </div>
    );
  }

  return (
    <div style={textLayerContainerStyle(text)} data-testid="visual-design-text">
      <span style={textLayerStyle(text, scale)}>{value}</span>
    </div>
  );
}
