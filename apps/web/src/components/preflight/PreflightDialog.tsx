import { AlertTriangle, CheckCircle2, Play, XCircle } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import type { PreflightAction, PreflightDestination, PreflightFinding } from '@/api/preflight-schemas';
import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { StartEnabledConfirmDialog } from '@/components/runtime/StartEnabledConfirmDialog';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { SelectInput } from '@/components/ui/SelectInput';
import { useBranchRuntimeQuery, useStartEnabledBranchesMutation } from '@/hooks/use-branches';
import { usePreflightQuery } from '@/hooks/use-preflight';
import { useStreamSetupsQuery } from '@/hooks/use-stream-setups';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { blockerKey } from '@/models/branch-presentation';

import { ProviderBrand } from '../providers/ProviderBrand';

type PreflightDialogProps = {
  open: boolean;
  onClose: () => void;
  platforms: readonly ConfiguredPlatform[];
  /** Opens PlatformSettingsDialog for one destination (add_stream_key / open_destination_settings). */
  onOpenDestinationSettings: (platformId: string) => void;
  /** Selects and scrolls to one destination's Stream details tab (fix_metadata). */
  onEditMetadata: (platformId: string) => void;
  /** Opens the Stream Setups manager (repair_setup_profile). */
  onOpenStreamSetups: () => void;
};

const CURRENT_CONFIG = '';

const SEVERITY_CLASSES: Record<string, string> = {
  blocker: 'bg-status-error/15 text-status-error border-status-error/30',
  warning: 'bg-status-warning/15 text-status-warning border-status-warning/30',
};

/**
 * "Is this setup actually ready to stream?" (docs/stream-preflight.md)
 * - aggregates existing readiness state only, never a score, and never
 * starts anything itself. The explicit launch action reuses the exact
 * same `StartEnabledConfirmDialog`/`useStartEnabledBranchesMutation`
 * the Dashboard's own Quick Actions card already uses.
 */
export function PreflightDialog({
  open,
  onClose,
  platforms,
  onOpenDestinationSettings,
  onEditMetadata,
  onOpenStreamSetups,
}: PreflightDialogProps) {
  const { t } = useTranslation(['streamPreflight', 'runtime', 'dashboard', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;
  const navigate = useNavigate();

  const [profileId, setProfileId] = useState<string>(CURRENT_CONFIG);
  const [confirmingStartEnabled, setConfirmingStartEnabled] = useState(false);

  const setupsQuery = useStreamSetupsQuery();
  const branchesQuery = useBranchRuntimeQuery();
  const startEnabledMutation = useStartEnabledBranchesMutation();
  const preflight = usePreflightQuery(profileId === CURRENT_CONFIG ? null : profileId, open);

  useEffect(() => {
    if (open) setProfileId(CURRENT_CONFIG);
  }, [open]);

  const handleClose = () => {
    if (startEnabledMutation.isPending) return;
    onClose();
  };

  const handleAction = (action: PreflightAction) => {
    switch (action.code) {
      case 'add_stream_key':
      case 'open_destination_settings':
        if (action.platformId !== undefined) onOpenDestinationSettings(action.platformId);
        handleClose();
        return;
      case 'fix_metadata':
        if (action.platformId !== undefined) onEditMetadata(action.platformId);
        handleClose();
        return;
      case 'repair_setup_profile':
        onOpenStreamSetups();
        handleClose();
        return;
      case 'reconnect_account':
        handleClose();
        void navigate('/settings');
        return;
      case 'install_ffmpeg':
      case 'start_mediamtx':
        handleClose();
        void navigate('/streams');
        return;
      default:
        return;
    }
  };

  const findingLabel = (finding: PreflightFinding): string => {
    if (finding.severity === 'blocker') {
      const key = blockerKey(finding.code);
      if (key !== null) return t(`runtime:${key}`);
      return finding.code;
    }
    switch (finding.code) {
      case 'metadata_invalid':
        return t('streamPreflight:findings.metadataInvalid');
      case 'account_reconnect_required':
        return t('streamPreflight:findings.accountReconnectRequired');
      case 'setup_destination_missing':
        return t('streamPreflight:findings.setupDestinationMissing');
      case 'setup_metadata_preset_missing':
        return t('streamPreflight:findings.setupMetadataPresetMissing');
      default:
        return finding.code;
    }
  };

  const actionLabel = (action: PreflightAction): string => {
    switch (action.code) {
      case 'add_stream_key':
        return t('streamPreflight:actions.addStreamKey');
      case 'open_destination_settings':
        return t('streamPreflight:actions.openDestinationSettings');
      case 'fix_metadata':
        return t('streamPreflight:actions.fixMetadata');
      case 'repair_setup_profile':
        return t('streamPreflight:actions.repairSetupProfile');
      case 'reconnect_account':
        return t('streamPreflight:actions.reconnectAccount');
      case 'install_ffmpeg':
        return t('streamPreflight:actions.installFfmpeg');
      case 'start_mediamtx':
        return t('streamPreflight:actions.startMediamtx');
      default:
        return t('streamPreflight:actions.generic');
    }
  };

  const renderFinding = (finding: PreflightFinding, key: string) => (
    <li key={key} className="flex flex-wrap items-center gap-2 text-xs">
      <span className={`rounded-full border px-2 py-0.5 font-medium ${SEVERITY_CLASSES[finding.severity] ?? ''}`}>
        {findingLabel(finding)}
      </span>
      {finding.action !== undefined && (
        <button
          type="button"
          className="text-accent underline-offset-2 hover:underline"
          onClick={() => handleAction(finding.action!)}
        >
          {actionLabel(finding.action)}
        </button>
      )}
    </li>
  );

  const renderDestination = (destination: PreflightDestination) => (
    <li key={destination.platformId} className="rounded-lg border border-line p-3">
      <div className="flex items-center gap-2 text-sm text-ink">
        <ProviderBrand
          providerId={destination.providerId}
          fallbackLabel={destination.providerId.slice(0, 2).toUpperCase()}
          size="sm"
        />
        <span className="min-w-0 flex-1 truncate">{destination.displayName}</span>
        {destination.findings.length === 0 && (
          <CheckCircle2 aria-hidden="true" className="size-4 shrink-0 text-status-live" />
        )}
      </div>
      {destination.findings.length > 0 && (
        <ul className="mt-2 space-y-1.5 pl-6">
          {destination.findings.map((f, i) => renderFinding(f, `${destination.platformId}-${i}`))}
        </ul>
      )}
    </li>
  );

  const report = preflight.data;
  const globalFindings = report?.findings.filter((f) => f.platformId === undefined || f.platformId === '') ?? [];

  const statusIcon = report?.status === 'ready'
    ? <CheckCircle2 aria-hidden="true" className="size-4 text-status-live" />
    : report?.status === 'ready_with_warnings'
      ? <AlertTriangle aria-hidden="true" className="size-4 text-status-warning" />
      : <XCircle aria-hidden="true" className="size-4 text-status-error" />;

  const profileOptions = [
    { value: CURRENT_CONFIG, label: t('streamPreflight:fields.currentConfig') },
    ...(setupsQuery.data ?? []).map((p) => ({ value: p.id, label: p.name })),
  ];

  return (
    <>
      <Modal
        open={open}
        onClose={handleClose}
        title={t('streamPreflight:title')}
        description={t('streamPreflight:description')}
        dismissible={!startEnabledMutation.isPending}
        footer={
          <>
            <Button type="button" onClick={handleClose}>
              {t('common:actions.close')}
            </Button>
            <Button
              type="button"
              variant="primary"
              icon={<Play className="size-3.5" />}
              disabled={platforms.length === 0}
              onClick={() => setConfirmingStartEnabled(true)}
            >
              {t('dashboard:quickActions.startEnabled')}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <label className="block space-y-1.5">
            <span className="text-xs font-medium text-ink-muted">{t('streamPreflight:fields.profileLabel')}</span>
            <SelectInput
              value={profileId}
              options={profileOptions}
              onChange={(event) => setProfileId(event.target.value)}
            />
          </label>

          {preflight.isLoading && <p className="text-sm text-ink-muted">{t('streamPreflight:checking')}</p>}

          {preflight.isError && (
            <p role="alert" className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error">
              {resolveApiErrorMessage(tErrors, preflight.error)}
            </p>
          )}

          {report !== undefined && report.streamingActive && (
            <p className="rounded-lg border border-line bg-surface-sunken/70 px-3 py-2 text-xs text-ink-muted">
              {t('streamPreflight:streamingActiveNotice')}
            </p>
          )}

          {report !== undefined && !report.streamingActive && (
            <>
              <div className="flex items-center gap-2 rounded-lg border border-line px-3 py-2 text-sm font-medium text-ink">
                {statusIcon}
                {t(`streamPreflight:status.${report.status}` as const)}
              </div>

              {globalFindings.length > 0 && (
                <div className="space-y-1.5">
                  <p className="text-xs font-semibold uppercase tracking-wide text-ink-faint">
                    {t('streamPreflight:sections.setup')}
                  </p>
                  <ul className="space-y-1.5">
                    {globalFindings.map((f, i) => renderFinding(f, `global-${i}`))}
                  </ul>
                </div>
              )}

              <div className="space-y-1.5">
                <p className="text-xs font-semibold uppercase tracking-wide text-ink-faint">
                  {t('streamPreflight:sections.destinations')}
                </p>
                {report.destinations.length === 0 ? (
                  <p className="text-xs text-ink-faint">{t('streamPreflight:noDestinations')}</p>
                ) : (
                  <ul className="space-y-2">{report.destinations.map(renderDestination)}</ul>
                )}
              </div>
            </>
          )}
        </div>
      </Modal>

      <StartEnabledConfirmDialog
        open={confirmingStartEnabled}
        platforms={[...platforms]}
        branches={branchesQuery.data}
        busy={startEnabledMutation.isPending}
        onConfirm={() =>
          startEnabledMutation.mutate(undefined, {
            onSuccess: () => {
              setConfirmingStartEnabled(false);
              onClose();
            },
          })
        }
        onCancel={() => setConfirmingStartEnabled(false)}
      />
    </>
  );
}
