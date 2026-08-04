import { Download, Loader2, Play, RotateCcw, Square } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { MediaMtxSnapshot } from '@/api/runtime-schemas';
import { Button } from '@/components/ui/Button';
import {
  useRestartMediaMtxMutation,
  useStartMediaMtxMutation,
  useStopMediaMtxMutation,
} from '@/hooks/use-runtime';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { controlsFor } from '@/models/runtime-presentation';

import { InstallMediaMtxDialog } from './InstallMediaMtxDialog';

type RuntimeControlsProps = {
  mediaMtx: MediaMtxSnapshot;
  /** Renders a compact row for the sidebar instead of the full control set. */
  compact?: boolean;
};

/**
 * Start, stop, restart and install controls.
 *
 * Availability comes from the state machine rather than from ad-hoc conditions,
 * so a state can never leave a control enabled that the backend would reject.
 * Every button is a real element, keyboard operable, with a pending label.
 *
 * These start the LOCAL INGEST SERVICE only. The wording says so, because
 * "Start" next to a list of platforms would otherwise read as "go live".
 */
export function RuntimeControls({ mediaMtx, compact = false }: RuntimeControlsProps) {
  const { t } = useTranslation(['runtime', 'errors']);
  const tErrors = useTranslation('errors').t;

  const start = useStartMediaMtxMutation();
  const stop = useStopMediaMtxMutation();
  const restart = useRestartMediaMtxMutation();
  const [installOpen, setInstallOpen] = useState(false);

  const controls = controlsFor(mediaMtx.state);
  const pending = start.isPending || stop.isPending || restart.isPending;

  // Only command failures are surfaced here; the process's own last error is
  // rendered by the panels, which have room for a full explanation.
  const commandError =
    start.error ?? stop.error ?? restart.error ?? null;

  return (
    <>
      <div className={compact ? 'flex flex-wrap gap-1.5' : 'flex flex-wrap items-center gap-2'}>
        {controls.canInstall && (
          <Button
            type="button"
            variant="primary"
            size={compact ? 'sm' : 'md'}
            disabled={pending}
            onClick={() => setInstallOpen(true)}
            icon={<Download className="size-3.5" />}
          >
            {t('runtime:controls.install')}
          </Button>
        )}

        {controls.canStart && (
          <Button
            type="button"
            variant="success"
            size={compact ? 'sm' : 'md'}
            disabled={pending}
            onClick={() => start.mutate()}
            icon={
              start.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Play className="size-3.5" />
              )
            }
          >
            {start.isPending ? t('runtime:controls.starting') : t('runtime:controls.start')}
          </Button>
        )}

        {controls.canStop && (
          <Button
            type="button"
            variant="danger"
            size={compact ? 'sm' : 'md'}
            disabled={pending}
            onClick={() => stop.mutate()}
            icon={
              stop.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Square className="size-3.5" />
              )
            }
          >
            {stop.isPending ? t('runtime:controls.stopping') : t('runtime:controls.stop')}
          </Button>
        )}

        {controls.canRestart && (
          <Button
            type="button"
            size={compact ? 'sm' : 'md'}
            disabled={pending}
            onClick={() => restart.mutate()}
            icon={
              restart.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <RotateCcw className="size-3.5" />
              )
            }
          >
            {restart.isPending ? t('runtime:controls.restarting') : t('runtime:controls.restart')}
          </Button>
        )}
      </div>

      {commandError !== null && (
        <p
          role="alert"
          className="mt-2 rounded-lg border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-[11px] text-status-error"
        >
          {resolveApiErrorMessage(tErrors, commandError)}
        </p>
      )}

      {!compact && (
        <p className="mt-2 text-[11px] leading-relaxed text-ink-faint">
          {t('runtime:controls.note')}
        </p>
      )}

      <InstallMediaMtxDialog
        open={installOpen}
        onClose={() => setInstallOpen(false)}
        supportedVersion={mediaMtx.supportedVersion}
      />
    </>
  );
}
