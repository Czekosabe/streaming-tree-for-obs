import { Bookmark, SlidersHorizontal } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { Button } from '../ui/Button';
import { ConfirmDialog } from '../ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '../ui/Panel';
import { MetadataForm } from './MetadataForm';
import { PlatformTabs } from './PlatformTabs';
import { ManagePresetsDialog } from './presets/ManagePresetsDialog';

type MetadataEditorProps = {
  platforms: readonly ConfiguredPlatform[];
  activeId: string | null;
  onSelect: (id: string) => void;
  /** Whether the currently open tab has unsaved edits right now - lifted
   * to the caller so a Stage 25 stream setup apply (triggered from
   * outside this component entirely) can also warn before it would
   * overwrite them (docs/stream-setup-profiles.md §19). */
  dirty: boolean;
  onDirtyChange: (dirty: boolean) => void;
};

/**
 * Tabbed metadata editor, one tab per configured destination.
 *
 * The form is keyed by platform id so switching tabs starts a fresh draft from
 * that platform's stored metadata. Switching away with unsaved edits is
 * confirmed first, so work is never discarded silently.
 */
export function MetadataEditor({ platforms, activeId, onSelect, dirty, onDirtyChange }: MetadataEditorProps) {
  const { t } = useTranslation('metadata');
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [managePresetsOpen, setManagePresetsOpen] = useState(false);
  // Bumped only when an apply-preset write touches the currently open tab,
  // to remount MetadataForm and pick up the freshly applied metadata - its
  // own draft state otherwise only resets when the platform id itself
  // changes, never on a metadata change arriving from elsewhere.
  const [applyRemountToken, setApplyRemountToken] = useState(0);

  const activePlatform = platforms.find((platform) => platform.id === activeId);

  const handleApplied = useCallback(
    (appliedIds: string[]) => {
      if (activeId !== null && appliedIds.includes(activeId)) {
        setApplyRemountToken((n) => n + 1);
        onDirtyChange(false);
      }
    },
    [activeId, onDirtyChange],
  );

  const requestSelect = (id: string) => {
    if (id === activeId) return;
    if (dirty) {
      setPendingId(id);
      return;
    }
    onSelect(id);
  };

  const confirmDiscard = () => {
    if (pendingId !== null) {
      onDirtyChange(false);
      onSelect(pendingId);
      setPendingId(null);
    }
  };

  const hasTabs = platforms.length > 0 && activeId !== null;

  return (
    <>
      <Panel>
        <PanelHeader
          title={t('editor.heading')}
          description={t('editor.description')}
          icon={<SlidersHorizontal className="size-4" />}
          actions={
            <Button
              type="button"
              onClick={() => setManagePresetsOpen(true)}
              icon={<Bookmark className="size-3.5" />}
            >
              {t('editor.presets')}
            </Button>
          }
        />

        {/*
         * A dedicated provider-switching column at desktop width (lg+),
         * rather than a thin strip squeezed above a wide form - the same
         * destination-switching behaviour, given real visual weight of its
         * own. Below `lg` it simply stacks above the form as a full-width
         * vertical list instead, which is exactly as usable on a narrow
         * viewport as a horizontal strip would have been.
         */}
        <div className={hasTabs ? 'lg:flex' : undefined}>
          {hasTabs && activeId !== null && (
            <div className="border-b border-line lg:w-56 lg:shrink-0 lg:border-r lg:border-b-0">
              <PlatformTabs
                platforms={platforms}
                activeId={activeId}
                onSelect={requestSelect}
                orientation="vertical"
              />
            </div>
          )}

          <PanelBody className="min-w-0 flex-1">
            {activePlatform === undefined ? (
              <p className="text-sm text-ink-muted">{t('editor.empty')}</p>
            ) : (
              <div
                role="tabpanel"
                id={`metadata-panel-${activePlatform.id}`}
                aria-labelledby={`metadata-tab-${activePlatform.id}`}
                tabIndex={0}
                className="animate-fade-rise"
              >
                <MetadataForm
                  key={`${activePlatform.id}-${applyRemountToken}`}
                  platform={activePlatform}
                  onDirtyChange={onDirtyChange}
                />
              </div>
            )}
          </PanelBody>
        </div>
      </Panel>

      <ConfirmDialog
        open={pendingId !== null}
        title={t('unsaved.title')}
        message={t('unsaved.message')}
        confirmLabel={t('unsaved.discard')}
        destructive
        onConfirm={confirmDiscard}
        onCancel={() => setPendingId(null)}
      />

      <ManagePresetsDialog
        open={managePresetsOpen}
        onClose={() => setManagePresetsOpen(false)}
        platforms={platforms}
        activeId={activeId}
        activeDirty={dirty}
        onApplied={handleApplied}
      />
    </>
  );
}
