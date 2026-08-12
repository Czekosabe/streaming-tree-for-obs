import { Loader2, Plus, Save, Trash2 } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { DonationSource } from '@/api/donationsource-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  useCreateDonationSourceMutation,
  useDeleteDonationSourceMutation,
  useDonationSourceEngagementQuery,
  useDonationSourcesQuery,
  useReplaceDonationSourceCredentialMutation,
  useRestartDonationEngagementMutation,
  useSetDonationEngagementMutation,
  useUpdateDonationSourceMutation,
} from '@/hooks/use-donationsources';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import {
  donationConnectorStateKey,
  donationConnectorStateTone,
} from '@/models/donationsource-presentation';

/** Bounds mirroring apps/server/internal/domain/donationsource/
 * validation.go exactly - client-side hints only, the backend is the real
 * authority and is re-validated on every submit regardless. */
const LABEL_MAX_LENGTH = 80;
const REMOTE_CHANNEL_ID_MAX_LENGTH = 128;
const CREDENTIAL_MAX_BYTES = 8 * 1024;

/**
 * The Stage 16A external-donation-source management surface: an
 * "add source" form plus one card per configured source (metadata edit,
 * credential replacement, enable/disable, connection status, delete).
 *
 * Unlike TwitchConnectorCard/YouTubeConnectorCard, a donation source is not
 * an OAuth-linked ConnectedAccount - there is no separate Settings-page
 * linking step, so creation and status/management live together here (see
 * docs/provider-integrations/external-donations.md's own persistence
 * section for why donation sources are a deliberately separate domain).
 */
export function StreamElementsConnectorCard() {
  const { t } = useTranslation('engagement');
  const sourcesQuery = useDonationSourcesQuery();
  const sources = sourcesQuery.data ?? [];

  return (
    <Panel>
      <PanelHeader
        title={t('streamElementsConnector.title')}
        description={t('streamElementsConnector.description')}
      />
      <PanelBody className="space-y-4">
        {sourcesQuery.isLoading && (
          <p className="text-xs text-ink-faint">{t('streamElementsConnector.loading')}</p>
        )}

        {sources.map((source) => (
          <DonationSourceCard key={source.id} source={source} />
        ))}

        <AddDonationSourceForm />
      </PanelBody>
    </Panel>
  );
}

function AddDonationSourceForm() {
  const { t } = useTranslation(['engagement', 'errors']);
  const tErrors = useTranslation('errors').t;
  const createMutation = useCreateDonationSourceMutation();

  const [label, setLabel] = useState('');
  const [remoteChannelId, setRemoteChannelId] = useState('');
  const [token, setToken] = useState('');
  const [submitError, setSubmitError] = useState<string | null>(null);

  const busy = createMutation.isPending;
  const canSubmit =
    label.trim() !== '' &&
    label.length <= LABEL_MAX_LENGTH &&
    remoteChannelId.trim() !== '' &&
    remoteChannelId.length <= REMOTE_CHANNEL_ID_MAX_LENGTH &&
    token !== '' &&
    token.length <= CREDENTIAL_MAX_BYTES;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canSubmit || busy) return;

    setSubmitError(null);
    createMutation.mutate(
      { providerId: 'streamelements', label: label.trim(), remoteChannelId: remoteChannelId.trim(), token },
      {
        onSuccess: () => {
          setLabel('');
          setRemoteChannelId('');
          setToken('');
        },
        onError: (error) => {
          setSubmitError(resolveApiErrorMessage(tErrors, error));
        },
        onSettled: () => {
          // The JWT must not linger in the mutation cache - mirrors
          // StreamKeySection.tsx's own reset-on-settle reasoning.
          createMutation.reset();
        },
      },
    );
  }

  return (
    <form
      onSubmit={handleSubmit}
      noValidate
      className="space-y-3 rounded-lg border border-line bg-surface-sunken p-3"
    >
      <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {t('streamElementsConnector.addTitle')}
      </p>
      <p className="text-[11px] text-ink-faint">{t('streamElementsConnector.addExplanation')}</p>

      {submitError !== null && (
        <p
          role="alert"
          className="rounded-md border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-[11px] text-status-error"
        >
          {submitError}
        </p>
      )}

      <FormField label={t('streamElementsConnector.labelField')}>
        {({ inputId }) => (
          <TextInput
            id={inputId}
            value={label}
            maxLength={LABEL_MAX_LENGTH}
            placeholder={t('streamElementsConnector.labelPlaceholder')}
            disabled={busy}
            onChange={(event) => setLabel(event.target.value)}
          />
        )}
      </FormField>

      <FormField label={t('streamElementsConnector.remoteChannelIdField')} hint={t('streamElementsConnector.remoteChannelIdHint')}>
        {({ inputId, describedBy }) => (
          <TextInput
            id={inputId}
            aria-describedby={describedBy}
            value={remoteChannelId}
            maxLength={REMOTE_CHANNEL_ID_MAX_LENGTH}
            placeholder={t('streamElementsConnector.remoteChannelIdPlaceholder')}
            disabled={busy}
            onChange={(event) => setRemoteChannelId(event.target.value)}
          />
        )}
      </FormField>

      <FormField label={t('streamElementsConnector.tokenField')} hint={t('streamElementsConnector.tokenHint')}>
        {({ inputId, describedBy }) => (
          <TextInput
            id={inputId}
            type="password"
            autoComplete="off"
            aria-describedby={describedBy}
            value={token}
            maxLength={CREDENTIAL_MAX_BYTES}
            placeholder={t('streamElementsConnector.tokenPlaceholder')}
            disabled={busy}
            onChange={(event) => setToken(event.target.value)}
          />
        )}
      </FormField>

      <Button
        type="submit"
        variant="primary"
        size="sm"
        disabled={!canSubmit || busy}
        icon={busy ? <Loader2 className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />}
      >
        {busy ? t('streamElementsConnector.adding') : t('streamElementsConnector.addAction')}
      </Button>
    </form>
  );
}

function DonationSourceCard({ source }: { source: DonationSource }) {
  const { t } = useTranslation(['engagement', 'errors']);
  const tErrors = useTranslation('errors').t;

  const engagementQuery = useDonationSourceEngagementQuery(source.id);
  const setEngagement = useSetDonationEngagementMutation();
  const restart = useRestartDonationEngagementMutation();
  const updateMutation = useUpdateDonationSourceMutation();
  const replaceCredentialMutation = useReplaceDonationSourceCredentialMutation();
  const deleteMutation = useDeleteDonationSourceMutation();

  const [labelDraft, setLabelDraft] = useState(source.label);
  const [remoteChannelIdDraft, setRemoteChannelIdDraft] = useState(source.remoteChannelId);
  const [metadataError, setMetadataError] = useState<string | null>(null);
  const [metadataSaved, setMetadataSaved] = useState(false);

  const [newToken, setNewToken] = useState('');
  const [credentialError, setCredentialError] = useState<string | null>(null);
  const [credentialSaved, setCredentialSaved] = useState(false);

  const [confirmingDelete, setConfirmingDelete] = useState(false);

  const data = engagementQuery.data;
  const tone = data === undefined ? donationConnectorStateTone('disabled') : donationConnectorStateTone(data.state);
  const stateLabel = data === undefined ? t('streamElementsConnector.loading') : t(donationConnectorStateKey(data.state));

  const metadataChanged = labelDraft !== source.label || remoteChannelIdDraft !== source.remoteChannelId;
  const metadataValid =
    labelDraft.trim() !== '' &&
    labelDraft.length <= LABEL_MAX_LENGTH &&
    remoteChannelIdDraft.trim() !== '' &&
    remoteChannelIdDraft.length <= REMOTE_CHANNEL_ID_MAX_LENGTH;

  function handleToggle(nextEnabled: boolean) {
    setEngagement.mutate({ id: source.id, input: { enabled: nextEnabled } });
  }

  function handleSaveMetadata(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!metadataValid || updateMutation.isPending) return;
    setMetadataError(null);
    setMetadataSaved(false);
    updateMutation.mutate(
      { id: source.id, input: { label: labelDraft.trim(), remoteChannelId: remoteChannelIdDraft.trim() } },
      {
        onSuccess: () => setMetadataSaved(true),
        onError: (error) => setMetadataError(resolveApiErrorMessage(tErrors, error)),
      },
    );
  }

  function handleReplaceCredential(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (newToken === '' || newToken.length > CREDENTIAL_MAX_BYTES || replaceCredentialMutation.isPending) return;
    setCredentialError(null);
    setCredentialSaved(false);
    replaceCredentialMutation.mutate(
      { id: source.id, input: { token: newToken } },
      {
        onSuccess: () => {
          setNewToken('');
          setCredentialSaved(true);
        },
        onError: (error) => setCredentialError(resolveApiErrorMessage(tErrors, error)),
        onSettled: () => replaceCredentialMutation.reset(),
      },
    );
  }

  function handleDelete() {
    deleteMutation.mutate(source.id, { onSuccess: () => setConfirmingDelete(false) });
  }

  return (
    <div className="space-y-3 rounded-lg border border-line bg-surface-sunken p-3">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-ink">{source.label}</p>
          <p className="text-[11px] text-ink-faint">{t('streamElementsConnector.subtitle', { room: source.remoteChannelId })}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <StatusBadge status={tone} label={stateLabel} />
        </div>
      </div>

      <ToggleSwitch
        label={t('streamElementsConnector.enableLabel')}
        description={t('streamElementsConnector.enableDescription')}
        checked={source.enabled}
        onCheckedChange={handleToggle}
        disabled={setEngagement.isPending}
      />

      {data !== undefined && (
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-[11px] text-ink-muted">
          <dt>{t('connector.reconnectCount')}</dt>
          <dd className="text-right text-ink">{data.reconnectCount}</dd>
          {data.lastEventAt !== undefined && (
            <>
              <dt>{t('connector.lastEventAt')}</dt>
              <dd className="text-right text-ink">{new Date(data.lastEventAt).toLocaleTimeString()}</dd>
            </>
          )}
          {data.lastDataGapAt !== undefined && (
            <>
              <dt>{t('connector.lastDataGapAt')}</dt>
              <dd className="text-right text-status-starting">{new Date(data.lastDataGapAt).toLocaleTimeString()}</dd>
            </>
          )}
          {data.possibleGapCount > 0 && (
            <>
              <dt>{t('streamElementsConnector.possibleGapCount')}</dt>
              <dd className="text-right text-status-starting">{data.possibleGapCount}</dd>
            </>
          )}
          {data.lastError !== undefined && (
            <>
              <dt>{t('connector.lastError')}</dt>
              <dd className="text-right text-status-error">{data.lastError}</dd>
            </>
          )}
        </dl>
      )}

      {data !== undefined && (data.state === 'error' || data.state === 'reconnect_required') && (
        <Button size="sm" onClick={() => restart.mutate(source.id)} disabled={restart.isPending}>
          {t('connector.restartAction')}
        </Button>
      )}

      <details className="rounded-md border border-line/60 bg-surface p-2">
        <summary className="cursor-pointer text-[11px] font-medium text-ink-muted">
          {t('streamElementsConnector.manageDetails')}
        </summary>
        <div className="mt-2 space-y-4">
          <form onSubmit={handleSaveMetadata} noValidate className="space-y-2">
            {metadataError !== null && (
              <p role="alert" className="text-[11px] text-status-error">
                {metadataError}
              </p>
            )}
            <FormField label={t('streamElementsConnector.labelField')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  value={labelDraft}
                  maxLength={LABEL_MAX_LENGTH}
                  onChange={(event) => {
                    setLabelDraft(event.target.value);
                    setMetadataSaved(false);
                  }}
                />
              )}
            </FormField>
            <FormField label={t('streamElementsConnector.remoteChannelIdField')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  value={remoteChannelIdDraft}
                  maxLength={REMOTE_CHANNEL_ID_MAX_LENGTH}
                  onChange={(event) => {
                    setRemoteChannelIdDraft(event.target.value);
                    setMetadataSaved(false);
                  }}
                />
              )}
            </FormField>
            <Button
              type="submit"
              size="sm"
              disabled={!metadataChanged || !metadataValid || updateMutation.isPending}
              icon={<Save className="size-3.5" />}
            >
              {t('streamElementsConnector.saveMetadata')}
            </Button>
            {metadataSaved && <span className="ml-2 text-[11px] text-status-live">{t('streamElementsConnector.saved')}</span>}
          </form>

          <form onSubmit={handleReplaceCredential} noValidate className="space-y-2">
            <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('streamElementsConnector.credentialSectionTitle')}
            </p>
            <p className="text-[11px] text-ink-faint">
              {source.credentialConfigured
                ? t('streamElementsConnector.credentialConfigured')
                : t('streamElementsConnector.credentialMissing')}
            </p>
            {credentialError !== null && (
              <p role="alert" className="text-[11px] text-status-error">
                {credentialError}
              </p>
            )}
            <FormField label={t('streamElementsConnector.tokenField')} hint={t('streamElementsConnector.tokenHint')}>
              {({ inputId, describedBy }) => (
                <TextInput
                  id={inputId}
                  type="password"
                  autoComplete="off"
                  aria-describedby={describedBy}
                  value={newToken}
                  maxLength={CREDENTIAL_MAX_BYTES}
                  placeholder={t('streamElementsConnector.tokenPlaceholder')}
                  onChange={(event) => {
                    setNewToken(event.target.value);
                    setCredentialSaved(false);
                  }}
                />
              )}
            </FormField>
            <Button
              type="submit"
              size="sm"
              disabled={newToken === '' || replaceCredentialMutation.isPending}
              icon={
                replaceCredentialMutation.isPending ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Save className="size-3.5" />
                )
              }
            >
              {t('streamElementsConnector.replaceCredential')}
            </Button>
            {credentialSaved && <span className="ml-2 text-[11px] text-status-live">{t('streamElementsConnector.saved')}</span>}
          </form>

          <Button
            type="button"
            variant="danger"
            size="sm"
            icon={<Trash2 className="size-3.5" />}
            onClick={() => setConfirmingDelete(true)}
          >
            {t('streamElementsConnector.delete')}
          </Button>
        </div>
      </details>

      <ConfirmDialog
        open={confirmingDelete}
        title={t('streamElementsConnector.deleteDialog.title')}
        message={t('streamElementsConnector.deleteDialog.message', { label: source.label })}
        confirmLabel={t('streamElementsConnector.delete')}
        destructive
        busy={deleteMutation.isPending}
        onConfirm={handleDelete}
        onCancel={() => setConfirmingDelete(false)}
      />
    </div>
  );
}
