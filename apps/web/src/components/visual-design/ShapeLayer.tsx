import type { VisualDesignShapeProps } from '@/api/visualdesign-schemas';

import { shapeLayerStyle } from './design-style';

export function ShapeLayer({ shape, scale }: { shape: VisualDesignShapeProps; scale: number }) {
  return <div style={shapeLayerStyle(shape, scale)} data-testid="visual-design-shape" />;
}
