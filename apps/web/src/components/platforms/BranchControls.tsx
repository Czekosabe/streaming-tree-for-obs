import { Loader2, Play, RefreshCw, Square } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { BranchSnapshot } from '@/api/branch-schemas';
import { Button } from '@/components/ui/Button';
import { StatusBadge } from '@/components/ui/StatusBadge';
import {
  useRestartBranchMutation,
  useStartBranchMutation,
  useStopBranchMutation,
} from '@/hooks/use-branches';
import {
  blockerKey,
  branchControlsFor,
  branchStateKey,
  branchTone,
} from '@/models/branch-presentation';

type BranchControlsProps = {
  platformId: string;
  branch: BranchSnapshot | undefined;
  /** Skips the internal status badge/spinner row - used when the caller
   * (the Dashboard card header) already shows the same real state more
   * prominently, so the two never show it twice at different sizes. */
  hideStatus?: boolean;
};

/**
 * Real destination-branch status and controls for one platform card.
 *
 * Replaces the previous permanently-disabled "Start" button now that
 * outgoing FFmpeg branches exist. Shows only real state reported by the
 * backend - never a fake viewer count, connection quality or bitrate, and
 * "Sending" only once the backend itself reports advancing FFmpeg output.
 *
 * Start/Stop are the card's primary action - sized and coloured (`primary`/
 * `danger`) to match, rather than the neutral secondary buttons every other
 * control on the card uses. Restart stays a lower-emphasis ghost action.
 */
export function BranchControls({ platformId, branch, hideStatus = false }: BranchControlsProps) {
  const { t } = useTranslation(['runtime', 'platforms']);

  const startMutation = useStartBranchMutation();
  const stopMutation = useStopBranchMutation();
  const restartMutation = useRestartBranchMutation();

  const state = branch?.state ?? 'idle';
  const controls = branchControlsFor(state);
  const busy = startMutation.isPending || stopMutation.isPending || restartMutation.isPending;

  const blockers = branch?.blockers ?? [];
  const blockerText = blockers
    .map((blocker) => {
      const key = blockerKey(blocker);
      return key === null ? blocker : t(key);
    })
    .join(' · ');

  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      {!hideStatus && (
        <div className="flex items-center gap-2">
          <StatusBadge status={branchTone(state)} label={t(branchStateKey(state))} />
          {busy && <Loader2 aria-hidden="true" className="size-3 shrink-0 animate-spin text-ink-faint" />}
        </div>
      )}

      {blockers.length > 0 && (
        <p className="truncate text-[10px] text-ink-faint" title={blockerText}>
          {blockerText}
        </p>
      )}

      <div className="flex items-center gap-1.5">
        {controls.canStart && (
          <Button
            variant="primary"
            disabled={busy}
            title={t('runtime:branch.startSingleNote')}
            icon={
              busy ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Play className="size-3.5" />
              )
            }
            onClick={() => startMutation.mutate(platformId)}
          >
            {t('runtime:branch.start')}
          </Button>
        )}
        {controls.canStop && (
          <Button
            variant="danger"
            disabled={busy}
            icon={
              busy ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Square className="size-3.5" />
              )
            }
            onClick={() => stopMutation.mutate(platformId)}
          >
            {t('runtime:branch.stop')}
          </Button>
        )}
        {controls.canRestart && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            icon={<RefreshCw className="size-3.5" />}
            onClick={() => restartMutation.mutate(platformId)}
          >
            {t('runtime:branch.restart')}
          </Button>
        )}
      </div>
    </div>
  );
}
