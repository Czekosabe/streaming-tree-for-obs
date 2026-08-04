import { Loader2, Play } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { BranchSnapshot } from '@/api/branch-schemas';
import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { blockerKey, branchFor } from '@/models/branch-presentation';

type StartEnabledConfirmDialogProps = {
  open: boolean;
  platforms: ConfiguredPlatform[];
  branches: BranchSnapshot[] | undefined;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

/**
 * Application-styled confirmation before starting every eligible enabled
 * destination at once - never `window.confirm`.
 *
 * Explains, before anything happens: which destinations will actually start,
 * which are enabled but currently blocked (and why), that bandwidth use
 * scales with the number of destinations started, that no video is
 * re-encoded, and that this begins real transmission.
 */
export function StartEnabledConfirmDialog({
  open,
  platforms,
  branches,
  busy,
  onConfirm,
  onCancel,
}: StartEnabledConfirmDialogProps) {
  const { t } = useTranslation(['runtime', 'platforms', 'common']);

  const enabled = platforms.filter((p) => p.enabled);
  const eligible: ConfiguredPlatform[] = [];
  const skipped: { platform: ConfiguredPlatform; blockers: string[] }[] = [];

  for (const platform of enabled) {
    const branch = branchFor(branches, platform.id);
    const blockers = branch?.blockers ?? [];
    if (blockers.length === 0) {
      eligible.push(platform);
    } else {
      skipped.push({ platform, blockers });
    }
  }

  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={t('runtime:branch.confirmStart.title')}
      dismissible={!busy}
      footer={
        <>
          <Button type="button" onClick={onCancel} disabled={busy}>
            {t('common:actions.cancel')}
          </Button>
          <Button
            type="button"
            variant="primary"
            onClick={onConfirm}
            disabled={busy || eligible.length === 0}
            icon={busy ? <Loader2 className="size-3.5 animate-spin" /> : <Play className="size-3.5" />}
          >
            {t('runtime:branch.confirmStart.confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-3 text-sm">
        {eligible.length > 0 ? (
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-ink-faint">
              {t('runtime:branch.confirmStart.eligibleHeading')}
            </p>
            <ul className="mt-1 space-y-0.5">
              {eligible.map((platform) => (
                <li key={platform.id} className="text-ink">
                  {platform.displayName}
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <p className="text-status-warning">{t('runtime:branch.confirmStart.noneEligible')}</p>
        )}

        {skipped.length > 0 && (
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-ink-faint">
              {t('runtime:branch.confirmStart.skippedHeading')}
            </p>
            <ul className="mt-1 space-y-0.5">
              {skipped.map(({ platform, blockers }) => (
                <li key={platform.id} className="text-ink-muted">
                  {platform.displayName} —{' '}
                  {blockers
                    .map((blocker) => {
                      const key = blockerKey(blocker);
                      return key === null ? blocker : t(key);
                    })
                    .join(', ')}
                </li>
              ))}
            </ul>
          </div>
        )}

        <div className="space-y-1 border-t border-line pt-2 text-xs text-ink-faint">
          <p>{t('runtime:branch.confirmStart.bandwidthNote')}</p>
          <p>{t('runtime:branch.confirmStart.streamCopyNote')}</p>
          <p className="font-medium text-ink-muted">
            {t('runtime:branch.confirmStart.realTransmissionNote')}
          </p>
        </div>
      </div>
    </Modal>
  );
}
