import { useParams } from 'react-router-dom';

import { AlertDesignerWorkspace } from '@/components/alert-designer/AlertDesignerWorkspace';
import { useAlertEventTypesQuery, useAlertProfileQuery, useAlertRuleQuery } from '@/hooks/use-alerts';
import { useVisualDesignQuery } from '@/hooks/use-visual-design';

/**
 * `/alerts/rules/{ruleId}/designer` - the Alert Overlay Designer (Stage
 * 13A task Part 26). Deliberately **not** wrapped in `AppShell`: like
 * the two public Browser Source routes, this page wants full control
 * of the viewport for its own canvas/panel layout and its own top bar,
 * never the operator dashboard's sidebar chrome competing for space.
 * This component only resolves loading/error state; all editing state
 * lives in `AlertDesignerWorkspace`, which never mounts until every
 * dependency (rule, profile, event-type capability, current design)
 * has actually loaded.
 */
export function AlertDesignerPage() {
  const { ruleId } = useParams<{ ruleId: string }>();
  const id = ruleId ?? null;

  const ruleQuery = useAlertRuleQuery(id);
  const profileQuery = useAlertProfileQuery(ruleQuery.data?.profileId ?? null);
  const eventTypesQuery = useAlertEventTypesQuery();
  const designQuery = useVisualDesignQuery('alert-rules', id);

  if (id === null) {
    return <div className="flex h-dvh items-center justify-center text-sm text-ink-muted" data-testid="alert-designer-error">Missing rule id.</div>;
  }

  const isLoading = ruleQuery.isLoading || profileQuery.isLoading || eventTypesQuery.isLoading || designQuery.isLoading;
  const isError = ruleQuery.isError || profileQuery.isError || eventTypesQuery.isError || designQuery.isError;

  if (isLoading) {
    return <div className="flex h-dvh items-center justify-center text-sm text-ink-muted" data-testid="alert-designer-loading">Loading…</div>;
  }
  if (isError || ruleQuery.data === undefined || profileQuery.data === undefined || designQuery.data === undefined) {
    return <div className="flex h-dvh items-center justify-center text-sm text-status-error" data-testid="alert-designer-error">Could not load this rule's design.</div>;
  }

  const capability = (eventTypesQuery.data ?? []).find((c) => c.eventType === ruleQuery.data.eventType);

  return (
    <AlertDesignerWorkspace
      rule={ruleQuery.data}
      profile={profileQuery.data}
      eventTypeCapability={capability}
      initialResponse={designQuery.data}
    />
  );
}
