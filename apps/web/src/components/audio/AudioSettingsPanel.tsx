import type { TFunction } from 'i18next';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { AudioSettings, AudioSettingsInput } from '@/api/audio-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { RemoteOverlayPanel } from '@/components/overlays/RemoteOverlayPanel';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TagInput } from '@/components/ui/TagInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  useAudioCapabilitiesQuery,
  useAudioSettingsQuery,
  useAudioVoicesQuery,
  useRotateAudioPublicSlugMutation,
  useUpdateAudioSettingsMutation,
} from '@/hooks/use-audio';
import { ApiError } from '@/lib/api-client';
import {
  AUDIO_PROVIDER_IDS,
  AUDIO_PROVIDER_MODES,
  AUDIO_SPEAKABLE_EVENT_TYPES,
  isValidBlockedWords,
  isValidGlobalCooldownSeconds,
  isValidLanguage,
  isValidMaxTextLength,
  isValidMinimumBits,
  isValidPerUserCooldownSeconds,
  isValidQueueCapacity,
  isValidSpeed,
  isValidThresholdAmount,
  isValidVoiceId,
  isValidVolume,
  normalizeCurrencyCode,
} from '@/models/audio';

function errorMessage(t: TFunction<'audio'>, error: unknown): string {
  if (error instanceof ApiError && error.code !== null) {
    const key = `errors.${error.code}` as never;
    const translated = t(key, { defaultValue: '' });
    return translated === '' ? t('errors.generic') : translated;
  }
  return t('errors.generic');
}

function draftFromSettings(settings: AudioSettings): AudioSettingsInput {
  return {
    enabled: settings.enabled,
    providerMode: settings.providerMode,
    enabledEventTypes: settings.enabledEventTypes,
    enabledProviderIds: settings.enabledProviderIds,
    enabledSourceIds: settings.enabledSourceIds,
    supporterOnlyMode: settings.supporterOnlyMode,
    thresholdCurrency: settings.thresholdCurrency ?? '',
    thresholdMinimumAmountMicros: settings.thresholdMinimumAmountMicros ?? null,
    minimumBits: settings.minimumBits ?? null,
    maxTextLengthCodePoints: settings.maxTextLengthCodePoints,
    perUserCooldownSeconds: settings.perUserCooldownSeconds,
    globalCooldownSeconds: settings.globalCooldownSeconds,
    blockedWords: settings.blockedWords,
    removeUrls: settings.removeUrls,
    normalizeRepeatedChars: settings.normalizeRepeatedChars,
    suppressCommands: settings.suppressCommands,
    queueCapacity: settings.queueCapacity,
    manualApproval: settings.manualApproval,
    voiceId: settings.voiceId,
    language: settings.language,
    speed: settings.speed,
    volume: settings.volume,
  };
}

function resolveBrowserSourceUrl(publicSlug: string): string {
  return `${window.location.origin}/overlay/audio/${publicSlug}`;
}

/**
 * The Stage 17A settings form: enable/provider/voice selection, event/
 * provider/source filters, exact-currency threshold, Bits threshold,
 * text preprocessing controls, cooldowns, queue capacity, manual
 * approval, and the Browser Source URL - copy/open/rotate.
 */
export function AudioSettingsPanel() {
  const { t } = useTranslation('audio');
  const settingsQuery = useAudioSettingsQuery();
  const capabilitiesQuery = useAudioCapabilitiesQuery();
  const voicesQuery = useAudioVoicesQuery();
  const updateMutation = useUpdateAudioSettingsMutation();
  const rotateMutation = useRotateAudioPublicSlugMutation();

  const [draft, setDraft] = useState<AudioSettingsInput | null>(null);
  const [copied, setCopied] = useState(false);
  const [rotateConfirmOpen, setRotateConfirmOpen] = useState(false);

  useEffect(() => {
    if (settingsQuery.data !== undefined && draft === null) {
      setDraft(draftFromSettings(settingsQuery.data));
    }
  }, [settingsQuery.data, draft]);

  if (settingsQuery.data === undefined || draft === null) {
    return null;
  }

  const settings = settingsQuery.data;

  const formValid =
    isValidMaxTextLength(draft.maxTextLengthCodePoints) &&
    isValidPerUserCooldownSeconds(draft.perUserCooldownSeconds) &&
    isValidGlobalCooldownSeconds(draft.globalCooldownSeconds) &&
    isValidQueueCapacity(draft.queueCapacity) &&
    isValidBlockedWords(draft.blockedWords) &&
    isValidVoiceId(draft.voiceId) &&
    isValidLanguage(draft.language) &&
    isValidSpeed(draft.speed) &&
    isValidVolume(draft.volume) &&
    isValidThresholdAmount(draft.thresholdCurrency, draft.thresholdMinimumAmountMicros ?? null) &&
    isValidMinimumBits(draft.minimumBits ?? null);

  const dirty = JSON.stringify(draft) !== JSON.stringify(draftFromSettings(settings));

  const toggleEventType = (type: string) => {
    setDraft((d) => {
      if (d === null) return d;
      const has = d.enabledEventTypes.includes(type);
      return {
        ...d,
        enabledEventTypes: has
          ? d.enabledEventTypes.filter((v) => v !== type)
          : [...d.enabledEventTypes, type],
      };
    });
  };

  const toggleProviderId = (id: string) => {
    setDraft((d) => {
      if (d === null) return d;
      const has = d.enabledProviderIds.includes(id);
      return {
        ...d,
        enabledProviderIds: has
          ? d.enabledProviderIds.filter((v) => v !== id)
          : [...d.enabledProviderIds, id],
      };
    });
  };

  const handleCopy = () => {
    void navigator.clipboard.writeText(resolveBrowserSourceUrl(settings.publicSlug));
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-4">
      <Panel>
        <PanelHeader title={t('capabilities.title')} />
        <PanelBody>
          {capabilitiesQuery.data !== undefined && (
            <p className="text-sm text-ink">
              {capabilitiesQuery.data.systemProviderAvailable
                ? t('capabilities.available')
                : t('capabilities.unavailable')}
              {!capabilitiesQuery.data.systemProviderAvailable &&
                capabilitiesQuery.data.systemProviderReason !== undefined && (
                  <span className="ml-2 text-ink-faint">
                    {t('capabilities.unavailableReason', {
                      reason: capabilitiesQuery.data.systemProviderReason,
                    })}
                  </span>
                )}
            </p>
          )}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelHeader title={t('settings.title')} />
        <PanelBody className="space-y-4">
          <ToggleSwitch
            label={t('settings.fields.enabled')}
            description={t('settings.fields.enabledHint')}
            checked={draft.enabled}
            onCheckedChange={(v) => setDraft((d) => (d === null ? d : { ...d, enabled: v }))}
          />

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <FormField label={t('settings.fields.providerMode')}>
              {({ inputId }) => (
                <SelectInput
                  id={inputId}
                  options={AUDIO_PROVIDER_MODES.map((v) => ({
                    value: v,
                    label: t(`settings.providerMode.${v}`),
                  }))}
                  value={draft.providerMode}
                  onChange={(e) =>
                    setDraft((d) =>
                      d === null
                        ? d
                        : { ...d, providerMode: e.target.value as AudioSettingsInput['providerMode'] },
                    )
                  }
                />
              )}
            </FormField>

            <FormField label={t('settings.fields.voice')}>
              {({ inputId }) => (
                <SelectInput
                  id={inputId}
                  options={[
                    { value: '', label: t('settings.fields.voiceSystemDefault') },
                    ...(voicesQuery.data ?? []).map((v) => ({ value: v.id, label: v.name ?? v.id })),
                  ]}
                  value={draft.voiceId}
                  onChange={(e) => setDraft((d) => (d === null ? d : { ...d, voiceId: e.target.value }))}
                />
              )}
            </FormField>
          </div>

          <ToggleSwitch
            label={t('settings.fields.supporterOnlyMode')}
            description={t('settings.fields.supporterOnlyModeHint')}
            checked={draft.supporterOnlyMode}
            onCheckedChange={(v) => setDraft((d) => (d === null ? d : { ...d, supporterOnlyMode: v }))}
          />

          <FormField label={t('settings.fields.eventTypes')} hint={t('settings.fields.eventTypesHint')}>
            {() => (
              <div className="flex flex-wrap gap-2">
                {AUDIO_SPEAKABLE_EVENT_TYPES.map((type) => (
                  <label
                    key={type}
                    className="flex cursor-pointer items-center gap-1.5 rounded-md border border-line bg-surface-sunken px-2 py-1 text-xs text-ink"
                  >
                    <input
                      type="checkbox"
                      checked={draft.enabledEventTypes.includes(type)}
                      onChange={() => toggleEventType(type)}
                      className="size-3.5 accent-accent"
                    />
                    {type}
                  </label>
                ))}
              </div>
            )}
          </FormField>

          <FormField label={t('settings.fields.providers')} hint={t('settings.fields.providersHint')}>
            {() => (
              <div className="flex flex-wrap gap-2">
                {AUDIO_PROVIDER_IDS.map((id) => (
                  <label
                    key={id}
                    className="flex cursor-pointer items-center gap-1.5 rounded-md border border-line bg-surface-sunken px-2 py-1 text-xs text-ink"
                  >
                    <input
                      type="checkbox"
                      checked={draft.enabledProviderIds.includes(id)}
                      onChange={() => toggleProviderId(id)}
                      className="size-3.5 accent-accent"
                    />
                    {id}
                  </label>
                ))}
              </div>
            )}
          </FormField>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <FormField label={t('settings.fields.thresholdCurrency')} hint={t('settings.fields.thresholdHint')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  value={draft.thresholdCurrency}
                  onChange={(e) =>
                    setDraft((d) =>
                      d === null ? d : { ...d, thresholdCurrency: normalizeCurrencyCode(e.target.value) },
                    )
                  }
                />
              )}
            </FormField>
            <FormField label={t('settings.fields.thresholdAmount')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  type="number"
                  value={
                    draft.thresholdMinimumAmountMicros === null || draft.thresholdMinimumAmountMicros === undefined
                      ? ''
                      : draft.thresholdMinimumAmountMicros / 1_000_000
                  }
                  onChange={(e) => {
                    const raw = e.target.value.trim();
                    setDraft((d) =>
                      d === null
                        ? d
                        : {
                            ...d,
                            thresholdMinimumAmountMicros: raw === '' ? null : Math.round(Number(raw) * 1_000_000),
                          },
                    );
                  }}
                />
              )}
            </FormField>
          </div>

          <FormField label={t('settings.fields.minimumBits')}>
            {({ inputId }) => (
              <TextInput
                id={inputId}
                type="number"
                value={draft.minimumBits === null || draft.minimumBits === undefined ? '' : draft.minimumBits}
                onChange={(e) => {
                  const raw = e.target.value.trim();
                  setDraft((d) => (d === null ? d : { ...d, minimumBits: raw === '' ? null : Number(raw) }));
                }}
              />
            )}
          </FormField>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <FormField label={t('settings.fields.maxTextLength')} hint={t('settings.fields.maxTextLengthHint')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  type="number"
                  value={draft.maxTextLengthCodePoints}
                  onChange={(e) =>
                    setDraft((d) => (d === null ? d : { ...d, maxTextLengthCodePoints: Number(e.target.value) }))
                  }
                />
              )}
            </FormField>
            <FormField label={t('settings.fields.perUserCooldown')} hint={t('settings.fields.perUserCooldownHint')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  type="number"
                  value={draft.perUserCooldownSeconds}
                  onChange={(e) =>
                    setDraft((d) => (d === null ? d : { ...d, perUserCooldownSeconds: Number(e.target.value) }))
                  }
                />
              )}
            </FormField>
            <FormField label={t('settings.fields.globalCooldown')} hint={t('settings.fields.globalCooldownHint')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  type="number"
                  value={draft.globalCooldownSeconds}
                  onChange={(e) =>
                    setDraft((d) => (d === null ? d : { ...d, globalCooldownSeconds: Number(e.target.value) }))
                  }
                />
              )}
            </FormField>
          </div>

          <FormField label={t('settings.fields.blockedWords')}>
            {({ inputId, describedBy }) => (
              <TagInput
                inputId={inputId}
                describedBy={describedBy}
                tags={draft.blockedWords}
                maxTags={200}
                invalid={!isValidBlockedWords(draft.blockedWords)}
                onChange={(tags) => setDraft((d) => (d === null ? d : { ...d, blockedWords: tags }))}
              />
            )}
          </FormField>

          <ToggleSwitch
            label={t('settings.fields.removeUrls')}
            checked={draft.removeUrls}
            onCheckedChange={(v) => setDraft((d) => (d === null ? d : { ...d, removeUrls: v }))}
          />
          <ToggleSwitch
            label={t('settings.fields.normalizeRepeatedChars')}
            description={t('settings.fields.normalizeRepeatedCharsHint')}
            checked={draft.normalizeRepeatedChars}
            onCheckedChange={(v) => setDraft((d) => (d === null ? d : { ...d, normalizeRepeatedChars: v }))}
          />
          <ToggleSwitch
            label={t('settings.fields.suppressCommands')}
            description={t('settings.fields.suppressCommandsHint')}
            checked={draft.suppressCommands}
            onCheckedChange={(v) => setDraft((d) => (d === null ? d : { ...d, suppressCommands: v }))}
          />

          <FormField label={t('settings.fields.queueCapacity')} hint={t('settings.fields.queueCapacityHint')}>
            {({ inputId }) => (
              <TextInput
                id={inputId}
                type="number"
                value={draft.queueCapacity}
                onChange={(e) => setDraft((d) => (d === null ? d : { ...d, queueCapacity: Number(e.target.value) }))}
              />
            )}
          </FormField>

          <ToggleSwitch
            label={t('settings.fields.manualApproval')}
            description={t('settings.fields.manualApprovalHint')}
            checked={draft.manualApproval}
            onCheckedChange={(v) => setDraft((d) => (d === null ? d : { ...d, manualApproval: v }))}
          />

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <FormField label={t('settings.fields.language')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  value={draft.language}
                  onChange={(e) => setDraft((d) => (d === null ? d : { ...d, language: e.target.value }))}
                />
              )}
            </FormField>
            <FormField label={t('settings.fields.speed')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  type="number"
                  step="0.1"
                  value={draft.speed}
                  onChange={(e) => setDraft((d) => (d === null ? d : { ...d, speed: Number(e.target.value) }))}
                />
              )}
            </FormField>
            <FormField label={t('settings.fields.volume')}>
              {({ inputId }) => (
                <TextInput
                  id={inputId}
                  type="number"
                  step="0.05"
                  value={draft.volume}
                  onChange={(e) => setDraft((d) => (d === null ? d : { ...d, volume: Number(e.target.value) }))}
                />
              )}
            </FormField>
          </div>

          {updateMutation.isError && (
            <p role="alert" className="text-sm text-status-error">
              {errorMessage(t, updateMutation.error)}
            </p>
          )}

          <div className="flex justify-end">
            <Button
              variant="primary"
              disabled={!formValid || !dirty || updateMutation.isPending}
              onClick={() => updateMutation.mutate(draft)}
            >
              {t('common.save')}
            </Button>
          </div>
        </PanelBody>
      </Panel>

      <Panel>
        <PanelHeader title={t('browserSource.title')} />
        <PanelBody className="space-y-3">
          <p className="text-xs text-ink-faint">{t('browserSource.hint')}</p>
          <FormField label={t('browserSource.url')}>
            {({ inputId }) => <TextInput id={inputId} readOnly value={resolveBrowserSourceUrl(settings.publicSlug)} />}
          </FormField>
          <div className="flex gap-2">
            <Button onClick={handleCopy}>{copied ? t('browserSource.copied') : t('browserSource.copyUrl')}</Button>
            <Button onClick={() => window.open(resolveBrowserSourceUrl(settings.publicSlug), '_blank')}>
              {t('browserSource.openUrl')}
            </Button>
            <Button variant="danger" onClick={() => setRotateConfirmOpen(true)}>
              {t('browserSource.rotateAction')}
            </Button>
          </div>
        </PanelBody>
      </Panel>

      <RemoteOverlayPanel domain="audio" localSlug={settings.publicSlug} />

      <ConfirmDialog
        open={rotateConfirmOpen}
        title={t('browserSource.rotateConfirmTitle')}
        message={t('browserSource.rotateConfirmMessage')}
        confirmLabel={t('browserSource.rotateAction')}
        destructive
        busy={rotateMutation.isPending}
        onCancel={() => setRotateConfirmOpen(false)}
        onConfirm={() => rotateMutation.mutate(undefined, { onSuccess: () => setRotateConfirmOpen(false) })}
      />
    </div>
  );
}
