import { FileText, Play, RefreshCw, Square } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { StartEnabledConfirmDialog } from '@/components/runtime/StartEnabledConfirmDialog';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import {
  branchKeys,
  useBranchRuntimeQuery,
  useStartEnabledBranchesMutation,
  useStopAllBranchesMutation,
} from '@/hooks/use-branches';
import { platformKeys } from '@/hooks/use-platforms';
import { branchFor } from '@/models/branch-presentation';

/**
 * Real, canonical destination actions surfaced on the Dashboard - never a
 * second implementation of branch start/stop/refresh logic. Start-enabled
 * and stop-all reuse the exact same mutations and confirmation dialogs
 * `StreamsPage` already uses; "Refresh status" invalidates the same real
 * queries every other Dashboard card already reads from. No action is
 * shown here that this application cannot actually perform - there is no
 * bulk "restart all" endpoint, so no such button is offered.
 */
export function QuickActionsCard({ platforms }: { platforms: readonly ConfiguredPlatform[] }) {
  const { t } = useTranslation(['dashboard', 'runtime']);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const branchesQuery = useBranchRuntimeQuery();
  const startEnabledMutation = useStartEnabledBranchesMutation();
  const stopAllMutation = useStopAllBranchesMutation();

  const [confirmingStartEnabled, setConfirmingStartEnabled] = useState(false);
  const [confirmingStopAll, setConfirmingStopAll] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const platformList = [...platforms];
  const bulkBusy = startEnabledMutation.isPending || stopAllMutation.isPending;
  const liveBranchCount = platformList.filter(
    (platform) => branchFor(branchesQuery.data, platform.id)?.state === 'live',
  ).length;

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: platformKeys.platforms }),
        queryClient.invalidateQueries({ queryKey: branchKeys.branches }),
      ]);
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <Panel>
      <PanelHeader title={t('dashboard:quickActions.heading')} headingLevel={3} />
      <PanelBody className="space-y-1.5">
        <Button
          className="w-full justify-start"
          disabled={bulkBusy || platformList.length === 0}
          icon={<Play className="size-3.5" />}
          onClick={() => setConfirmingStartEnabled(true)}
        >
          {t('dashboard:quickActions.startEnabled')}
        </Button>
        <Button
          className="w-full justify-start"
          disabled={bulkBusy}
          icon={<Square className="size-3.5" />}
          onClick={() => {
            // Only interrupt something a viewer would notice - matches
            // StreamsPage's own confirmation policy for this exact action.
            if (liveBranchCount > 0) {
              setConfirmingStopAll(true);
            } else {
              stopAllMutation.mutate();
            }
          }}
        >
          {t('dashboard:quickActions.stopAll')}
        </Button>
        <Button
          className="w-full justify-start"
          disabled={refreshing}
          icon={<RefreshCw className={refreshing ? 'size-3.5 animate-spin' : 'size-3.5'} />}
          onClick={() => void handleRefresh()}
        >
          {t('dashboard:quickActions.refresh')}
        </Button>
        <Button
          className="w-full justify-start"
          icon={<FileText className="size-3.5" />}
          onClick={() => void navigate('/logs')}
        >
          {t('dashboard:quickActions.openLogs')}
        </Button>
      </PanelBody>

      <StartEnabledConfirmDialog
        open={confirmingStartEnabled}
        platforms={platformList}
        branches={branchesQuery.data}
        busy={startEnabledMutation.isPending}
        onConfirm={() =>
          startEnabledMutation.mutate(undefined, {
            onSuccess: () => setConfirmingStartEnabled(false),
          })
        }
        onCancel={() => setConfirmingStartEnabled(false)}
      />

      <ConfirmDialog
        open={confirmingStopAll}
        title={t('runtime:branch.confirmStopAll.title')}
        message={t('runtime:branch.confirmStopAll.body', { count: liveBranchCount })}
        confirmLabel={t('runtime:branch.confirmStopAll.confirm')}
        destructive
        busy={stopAllMutation.isPending}
        onConfirm={() =>
          stopAllMutation.mutate(undefined, {
            onSuccess: () => setConfirmingStopAll(false),
          })
        }
        onCancel={() => setConfirmingStopAll(false)}
      />
    </Panel>
  );
}
