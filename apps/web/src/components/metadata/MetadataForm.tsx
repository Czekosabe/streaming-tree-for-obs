import type { ParseKeys } from 'i18next';
import { Check, RotateCcw, Save } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { useLanguage } from '@/i18n/use-language';
import {
  validateMetadata,
  type MetadataErrors,
  type MetadataFieldName,
} from '@/models/metadata-schema';
import type {
  PlatformCapabilities,
  PlatformMetadata,
  SelectOption,
  StreamPlatform,
  TranslatedOption,
} from '@/models/platform';

import { Button } from '../ui/Button';
import { FormField } from '../ui/FormField';
import { SelectInput } from '../ui/SelectInput';
import { TagInput } from '../ui/TagInput';
import { TextArea, TextInput } from '../ui/TextInput';
import { ToggleSwitch } from '../ui/ToggleSwitch';
import { useValidationMessages } from './use-validation-messages';

type MetadataFormProps = {
  platform: StreamPlatform;
  onSave: (metadata: PlatformMetadata) => void;
};

/** Field labels used by the capability summary under the form. */
const FIELD_LABEL_KEYS: Record<keyof PlatformCapabilities, ParseKeys<'metadata'>> = {
  title: 'fields.title',
  description: 'fields.description',
  category: 'fields.category',
  tags: 'fields.tags',
  language: 'fields.language',
  visibility: 'fields.visibility',
  matureContent: 'fields.matureContent',
  dvr: 'fields.dvr',
  latencyMode: 'fields.latencyMode',
};

const CAPABILITY_FIELDS = Object.keys(FIELD_LABEL_KEYS) as (keyof PlatformCapabilities)[];

/**
 * Capability-driven metadata form.
 *
 * Only fields enabled in the platform's capability table are rendered, and
 * validation is produced by the Zod schema built from the same table. Saving
 * writes to the in-memory DEMO store only - no platform API is contacted.
 *
 * Values typed here (title, description, tags, category) are user content and
 * are never translated - only the surrounding labels are.
 *
 * The component is remounted per platform (`key` in `MetadataEditor`), so the
 * draft state always starts from the selected platform's stored metadata.
 */
export function MetadataForm({ platform, onSave }: MetadataFormProps) {
  const { t } = useTranslation(['metadata', 'platforms']);
  const { locale } = useLanguage();
  const messages = useValidationMessages();
  const { capabilities, limits, options } = platform;

  const [draft, setDraft] = useState<PlatformMetadata>(platform.metadata);
  const [errors, setErrors] = useState<MetadataErrors>({});
  const [saved, setSaved] = useState(false);

  const categoryLabel = t(`platforms:${options.categoryLabelKey}` as const);

  /** Resolves translated option labels for the native select. */
  const resolveOptions = (source: readonly TranslatedOption[]): SelectOption[] =>
    source.map((option) => ({
      value: option.value,
      label: t(`platforms:${option.labelKey}` as const),
    }));

  const patch = <K extends MetadataFieldName>(field: K, value: PlatformMetadata[K]) => {
    setDraft((current) => ({ ...current, [field]: value }));
    setSaved(false);
    // Clear the error for the field being edited so the message does not linger.
    setErrors((current) => {
      if (current[field] === undefined) return current;
      const next = { ...current };
      delete next[field];
      return next;
    });
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const result = validateMetadata(platform, draft, { messages, categoryLabel });
    setErrors(result.errors);

    if (!result.success) {
      setSaved(false);
      return;
    }

    onSave(draft);
    setSaved(true);
  };

  const handleReset = () => {
    setDraft(platform.metadata);
    setErrors({});
    setSaved(false);
  };

  const unsupported = CAPABILITY_FIELDS.filter((field) => !capabilities[field]);
  // `Intl.ListFormat` joins the list the way the active language does, instead
  // of hard-coding a separator.
  const unsupportedList = new Intl.ListFormat(locale, {
    style: 'long',
    type: 'conjunction',
  }).format(unsupported.map((field) => t(FIELD_LABEL_KEYS[field])));

  return (
    <form onSubmit={handleSubmit} noValidate className="space-y-5">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {capabilities.title && (
          <FormField
            label={t('metadata:fields.title')}
            error={errors.title}
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
                aria-invalid={errors.title !== undefined}
                value={draft.title}
                maxLength={limits.titleMaxLength}
                placeholder={t('metadata:placeholders.title')}
                onChange={(event) => patch('title', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.description && (
          <FormField
            label={t('metadata:fields.description')}
            error={errors.description}
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
                aria-invalid={errors.description !== undefined}
                value={draft.description}
                maxLength={limits.descriptionMaxLength}
                placeholder={t('metadata:placeholders.description')}
                onChange={(event) => patch('description', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.category && (
          <FormField label={categoryLabel} error={errors.category}>
            {({ inputId, describedBy }) => (
              <TextInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.category !== undefined}
                value={draft.category}
                placeholder={t(`platforms:${options.categoryPlaceholderKey}` as const)}
                onChange={(event) => patch('category', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.language && (
          <FormField label={t('metadata:fields.language')} error={errors.language}>
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.language !== undefined}
                options={options.languages}
                value={draft.language}
                onChange={(event) => patch('language', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.visibility && (
          <FormField label={t('metadata:fields.visibility')} error={errors.visibility}>
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.visibility !== undefined}
                options={resolveOptions(options.visibility)}
                value={draft.visibility}
                onChange={(event) => patch('visibility', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.latencyMode && (
          <FormField
            label={t('metadata:fields.latencyMode')}
            hint={t('metadata:hints.latencyMode')}
            error={errors.latencyMode}
          >
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.latencyMode !== undefined}
                options={resolveOptions(options.latencyModes)}
                value={draft.latencyMode}
                onChange={(event) => patch('latencyMode', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.tags && (
          <FormField label={t('metadata:fields.tags')} error={errors.tags} className="lg:col-span-2">
            {({ inputId, describedBy }) => (
              <TagInput
                inputId={inputId}
                describedBy={describedBy}
                invalid={errors.tags !== undefined}
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
          {t('metadata:unsupported', { platform: platform.name, fields: unsupportedList })}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2 border-t border-line pt-4">
        <Button type="submit" variant="primary" icon={<Save className="size-3.5" />}>
          {t('metadata:actions.save')}
        </Button>
        <Button type="button" onClick={handleReset} icon={<RotateCcw className="size-3.5" />}>
          {t('metadata:actions.reset')}
        </Button>

        <p aria-live="polite" className="ml-auto text-[11px] text-ink-faint">
          {saved ? (
            <span className="inline-flex items-center gap-1 text-status-live">
              <Check aria-hidden="true" className="size-3" />
              {t('metadata:status.saved')}
            </span>
          ) : (
            t('metadata:status.idle')
          )}
        </p>
      </div>
    </form>
  );
}
