import { Check, Loader2, RotateCcw, Save } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform, SaveMetadataInput } from '@/api/platform-schemas';
import { useApiFieldErrors } from '@/hooks/use-api-field-errors';
import { useUpdatePlatformMetadataMutation } from '@/hooks/use-platforms';
import { useLanguage } from '@/i18n/use-language';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import {
  validateMetadata,
  type MetadataErrors,
  type MetadataFieldName,
} from '@/models/metadata-schema';
import {
  categoryFieldLabelKey,
  categoryPlaceholderKey,
  languageLabel,
  latencyLabelKey,
  visibilityLabelKey,
} from '@/models/provider-labels';
import type { SelectOption } from '@/models/platform';

import { Button } from '../ui/Button';
import { FormField } from '../ui/FormField';
import { SelectInput } from '../ui/SelectInput';
import { TagInput } from '../ui/TagInput';
import { TextArea, TextInput } from '../ui/TextInput';
import { ToggleSwitch } from '../ui/ToggleSwitch';
import { CategoryPicker } from './CategoryPicker';
import { isDirty, toDraft } from './metadata-draft';
import { PublishPanel } from './PublishPanel';
import { useValidationMessages } from './use-validation-messages';

type MetadataFormProps = {
  platform: ConfiguredPlatform;
  /** Reports whether the draft differs from what is stored. */
  onDirtyChange: (dirty: boolean) => void;
};

/** Field labels shown in the capability summary under the form. */
const CAPABILITY_FIELDS = [
  'title',
  'description',
  'category',
  'tags',
  'language',
  'visibility',
  'matureContent',
  'dvr',
  'latencyMode',
] as const;

type CapabilityField = (typeof CAPABILITY_FIELDS)[number];

/**
 * Capability-driven metadata form backed by the API.
 *
 * Capabilities, limits and option lists all come from the backend's provider
 * definition; the frontend keeps no competing table. Only supported fields are
 * rendered, and saving performs a real PUT that replaces metadata and tags
 * atomically.
 *
 * Values typed here are user content: stored exactly as entered and never
 * translated.
 */
export function MetadataForm({ platform, onDirtyChange }: MetadataFormProps) {
  const { t } = useTranslation(['metadata', 'platforms', 'errors']);
  const tErrors = useTranslation('errors').t;
  const { locale } = useLanguage();
  const messages = useValidationMessages();
  const fieldErrorsOf = useApiFieldErrors();
  const saveMetadata = useUpdatePlatformMetadataMutation();

  const [draft, setDraft] = useState<SaveMetadataInput>(() => toDraft(platform));
  const [errors, setErrors] = useState<MetadataErrors>({});
  const [saved, setSaved] = useState(false);

  const dirty = isDirty(draft, toDraft(platform));

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);

  const provider = platform.provider;
  if (provider === undefined) {
    return (
      <p className="rounded-lg border border-status-warning/30 bg-status-warning/10 px-3 py-2 text-xs text-status-warning">
        {t('platforms:card.unknownProvider', { providerId: platform.providerId })}
      </p>
    );
  }

  const { capabilities, limits } = provider;
  const serverFieldErrors = fieldErrorsOf(saveMetadata.error);
  const busy = saveMetadata.isPending;

  const categoryKey = categoryFieldLabelKey(provider.categoryFieldType);
  const categoryLabel = categoryKey === null ? t('metadata:fields.category') : t(`platforms:${categoryKey}` as const);
  const placeholderKey = categoryPlaceholderKey(provider.id);

  /** Resolves option identifiers to labels, keeping unknown ones visible. */
  const toOptions = (
    identifiers: readonly string[],
    resolve: (identifier: string) => string | null,
  ): SelectOption[] =>
    identifiers.map((value) => ({ value, label: resolve(value) ?? value }));

  const visibilityOptions = toOptions(provider.visibilityOptions, (id) => {
    const key = visibilityLabelKey(id);
    return key === null ? null : t(`platforms:${key}` as const);
  });
  const latencyOptions = toOptions(provider.latencyOptions, (id) => {
    const key = latencyLabelKey(id);
    return key === null ? null : t(`platforms:${key}` as const);
  });
  const languageOptions = toOptions(provider.languageOptions, languageLabel);

  const patch = <K extends keyof SaveMetadataInput>(field: K, value: SaveMetadataInput[K]) => {
    setDraft((current) => ({ ...current, [field]: value }));
    setSaved(false);
    setErrors((current) => {
      const key = field as MetadataFieldName;
      if (current[key] === undefined) return current;
      const next = { ...current };
      delete next[key];
      return next;
    });
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) return;

    // Client-side validation first, so obvious mistakes never reach the
    // network. The backend validates again and is the authority.
    const result = validateMetadata(
      { capabilities, limits, categoryLabel, visibilityOptions: provider.visibilityOptions, latencyOptions: provider.latencyOptions, languageOptions: provider.languageOptions },
      draft,
      messages,
    );
    setErrors(result.errors);
    if (!result.success) {
      setSaved(false);
      return;
    }

    saveMetadata.mutate(
      { id: platform.id, input: draft },
      { onSuccess: () => setSaved(true) },
    );
  };

  const handleReset = () => {
    setDraft(toDraft(platform));
    setErrors({});
    setSaved(false);
    saveMetadata.reset();
  };

  const unsupported = CAPABILITY_FIELDS.filter((field: CapabilityField) => !capabilities[field]);
  const unsupportedList = new Intl.ListFormat(locale, {
    style: 'long',
    type: 'conjunction',
  }).format(unsupported.map((field) => t(`metadata:fields.${field}` as const)));

  const generalError =
    saveMetadata.error !== null && Object.keys(serverFieldErrors).length === 0
      ? resolveApiErrorMessage(tErrors, saveMetadata.error)
      : null;

  const errorFor = (field: MetadataFieldName): string | undefined =>
    errors[field] ?? serverFieldErrors[field];

  return (
    <form onSubmit={handleSubmit} noValidate className="space-y-5">
      {generalError !== null && (
        <p
          role="alert"
          className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error"
        >
          {generalError}
        </p>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {capabilities.title && (
          <FormField
            label={t('metadata:fields.title')}
            error={errorFor('title')}
            counter={t('metadata:counter', {
              current: draft.title.length,
              max: limits.titleMaxLength,
            })}
            className="lg:col-span-2"
          >
            {({ inputId, describedBy }) => (
              <TextInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errorFor('title') !== undefined}
                value={draft.title}
                maxLength={limits.titleMaxLength}
                disabled={busy}
                placeholder={t('metadata:placeholders.title')}
                onChange={(event) => patch('title', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.description && (
          <FormField
            label={t('metadata:fields.description')}
            error={errorFor('description')}
            counter={t('metadata:counter', {
              current: draft.description.length,
              max: limits.descriptionMaxLength,
            })}
            className="lg:col-span-2"
          >
            {({ inputId, describedBy }) => (
              <TextArea
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errorFor('description') !== undefined}
                value={draft.description}
                maxLength={limits.descriptionMaxLength}
                disabled={busy}
                placeholder={t('metadata:placeholders.description')}
                onChange={(event) => patch('description', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.category && (
          <FormField label={categoryLabel} error={errorFor('category')}>
            {({ inputId, describedBy }) =>
              provider.categoryRequiresRemoteId ? (
                <CategoryPicker
                  platformId={platform.id}
                  providerId={platform.providerId}
                  value={draft.category}
                  categoryId={draft.categoryId}
                  disabled={busy}
                  invalid={errorFor('category') !== undefined}
                  onChange={(category, categoryId) => {
                    patch('category', category);
                    patch('categoryId', categoryId);
                  }}
                />
              ) : (
                <TextInput
                  id={inputId}
                  aria-describedby={describedBy}
                  aria-invalid={errorFor('category') !== undefined}
                  value={draft.category}
                  disabled={busy}
                  placeholder={
                    placeholderKey === null ? undefined : t(`platforms:${placeholderKey}` as const)
                  }
                  onChange={(event) => patch('category', event.target.value)}
                />
              )
            }
          </FormField>
        )}

        {capabilities.language && (
          <FormField label={t('metadata:fields.language')} error={errorFor('language')}>
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errorFor('language') !== undefined}
                options={languageOptions}
                value={draft.language}
                disabled={busy}
                onChange={(event) => patch('language', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.visibility && (
          <FormField label={t('metadata:fields.visibility')} error={errorFor('visibility')}>
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errorFor('visibility') !== undefined}
                options={visibilityOptions}
                value={draft.visibility}
                disabled={busy}
                onChange={(event) => patch('visibility', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.latencyMode && (
          <FormField
            label={t('metadata:fields.latencyMode')}
            hint={t('metadata:hints.latencyMode')}
            error={errorFor('latencyMode')}
          >
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errorFor('latencyMode') !== undefined}
                options={latencyOptions}
                value={draft.latencyMode}
                disabled={busy}
                onChange={(event) => patch('latencyMode', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.tags && (
          <FormField label={t('metadata:fields.tags')} error={errorFor('tags')} className="lg:col-span-2">
            {({ inputId, describedBy }) => (
              <TagInput
                inputId={inputId}
                describedBy={describedBy}
                invalid={errorFor('tags') !== undefined}
                tags={draft.tags}
                maxTags={limits.maxTags}
                onChange={(tags) => patch('tags', tags)}
              />
            )}
          </FormField>
        )}
      </div>

      {(capabilities.matureContent || capabilities.dvr) && (
        <div className="grid grid-cols-1 gap-3 rounded-lg border border-line bg-surface-sunken p-3 sm:grid-cols-2">
          {capabilities.matureContent && (
            <ToggleSwitch
              label={t('metadata:fields.matureContent')}
              description={t('metadata:toggles.matureContentDescription')}
              checked={draft.matureContent}
              onCheckedChange={(checked) => patch('matureContent', checked)}
            />
          )}
          {capabilities.dvr && (
            <ToggleSwitch
              label={t('metadata:fields.dvr')}
              description={t('metadata:toggles.dvrDescription')}
              checked={draft.dvr}
              onCheckedChange={(checked) => patch('dvr', checked)}
            />
          )}
        </div>
      )}

      {unsupported.length > 0 && (
        <p className="text-[11px] leading-relaxed text-ink-faint">
          {t('metadata:unsupported', {
            platform: provider.brandName,
            fields: unsupportedList,
          })}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2 border-t border-line pt-4">
        <Button
          type="submit"
          variant="primary"
          disabled={busy}
          icon={
            busy ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />
          }
        >
          {busy ? t('metadata:actions.saving') : t('metadata:actions.save')}
        </Button>
        <Button
          type="button"
          onClick={handleReset}
          disabled={busy || !dirty}
          icon={<RotateCcw className="size-3.5" />}
        >
          {t('metadata:actions.reset')}
        </Button>

        <p aria-live="polite" className="ml-auto text-[11px] text-ink-faint">
          {saved && !dirty ? (
            <span className="inline-flex items-center gap-1 text-status-live">
              <Check aria-hidden="true" className="size-3" />
              {t('metadata:status.saved')}
            </span>
          ) : dirty ? (
            <span className="text-status-warning">{t('metadata:status.unsaved')}</span>
          ) : (
            t('metadata:status.idle')
          )}
        </p>
      </div>

      <PublishPanel platform={platform} dirty={dirty} />
    </form>
  );
}
