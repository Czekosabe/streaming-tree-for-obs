import { useParams } from 'react-router-dom';

import { GoalWidgetRenderer } from '@/components/goals/GoalWidgetRenderer';
import { useWidgetStream } from '@/hooks/use-widget-stream';

/**
 * The public Stage 18A goal-widget Browser Source route
 * (`/overlay/widgets/:publicSlug`) - a standalone page with no
 * application shell, sidebar, top bar, operator controls, or settings
 * UI (see App.tsx: registered outside every layout wrapper other pages
 * use, exactly like PublicAudioPage.tsx/PublicAlertPage.tsx). Renders
 * nothing until the first `widget.reset` snapshot arrives, then the
 * exact same `GoalWidgetRenderer` the management page's own preview
 * uses - never a second, visually-different implementation.
 */
export function PublicWidgetPage() {
  const { publicSlug } = useParams<{ publicSlug: string }>();
  const { snapshot } = useWidgetStream(publicSlug);

  return (
    <div className="flex h-screen w-screen items-start justify-start overflow-hidden bg-transparent" data-testid="widget-page-root">
      {snapshot !== null && <GoalWidgetRenderer snapshot={snapshot} />}
    </div>
  );
}
