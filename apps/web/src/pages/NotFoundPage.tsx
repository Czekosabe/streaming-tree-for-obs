import { Link } from 'react-router-dom';

import { AppShell } from '@/components/layout/AppShell';
import { Panel, PanelBody } from '@/components/ui/Panel';

export function NotFoundPage() {
  return (
    <AppShell title="Not found" description="This route does not exist.">
      <Panel className="mx-auto max-w-md">
        <PanelBody className="space-y-3 py-10 text-center">
          <p className="font-mono text-3xl text-accent-soft">404</p>
          <p className="text-sm text-ink-muted">
            The page you were looking for is not part of the application.
          </p>
          <Link
            to="/"
            className="inline-flex h-9 items-center rounded-lg border border-line bg-surface-raised px-3.5 text-sm font-medium text-ink transition-colors hover:bg-surface-hover"
          >
            Back to dashboard
          </Link>
        </PanelBody>
      </Panel>
    </AppShell>
  );
}
