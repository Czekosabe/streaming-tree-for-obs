import { useEffect, useRef, useState } from 'react';

import type { VisualDesignCanvas } from '@/api/visualdesign-schemas';
import { containScale } from '@/models/visualdesign';

export type ScaleTransform = { scale: number; offsetX: number; offsetY: number };

/**
 * Measures the wrapper element's own live size (via ResizeObserver) and
 * derives the contain-style scale/centering transform for canvas
 * (docs/visual-designs.md §3) - the same policy both the real public
 * Browser Source route and the Designer's own preview/canvas apply, so
 * a design looks identical in both places at any viewport size.
 */
export function useContainScale(canvas: VisualDesignCanvas): [React.RefObject<HTMLDivElement | null>, ScaleTransform] {
  const ref = useRef<HTMLDivElement>(null);
  const [transform, setTransform] = useState<ScaleTransform>({ scale: 1, offsetX: 0, offsetY: 0 });

  useEffect(() => {
    const el = ref.current;
    if (el === null) return;

    const measure = () => {
      const { width, height } = el.getBoundingClientRect();
      const scale = containScale(canvas, width, height);
      const offsetX = Math.max(0, (width - canvas.width * scale) / 2);
      const offsetY = Math.max(0, (height - canvas.height * scale) / 2);
      setTransform({ scale, offsetX, offsetY });
    };

    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- re-measure on width/height value change only, never on a new-but-equal canvas object identity
  }, [canvas.width, canvas.height]);

  return [ref, transform];
}
