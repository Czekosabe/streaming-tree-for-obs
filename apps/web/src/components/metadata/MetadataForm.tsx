import { Check, RotateCcw, Save } from 'lucide-react';
import { useState, type FormEvent } from 'react';

import {
  validateMetadata,
  type MetadataErrors,
  type MetadataFieldName,
} from '@/models/metadata-schema';
import type { PlatformMetadata, StreamPlatform } from '@/models/platform';

import { Button } from '../ui/Button';
import { FormField } from '../ui/FormField';
import { SelectInput } from '../ui/SelectInput';
import { TagInput } from '../ui/TagInput';
import { TextArea, TextInput } from '../ui/TextInput';
import { ToggleSwitch } from '../ui/ToggleSwitch';

type MetadataFormProps = {
  platform: StreamPlatform;
  onSave: (metadata: PlatformMetadata) => void;
};

/** Human labels for the capability summary shown under the form. */
const FIELD_LABELS: Record<keyof StreamPlatform['capabilities'], string> = {
  title: 'Title',
  description: 'Description',
  category: 'Category',
  tags: 'Tags',
  language: 'Language',
  visibility: 'Visibility',
  matureContent: 'Mature content',
  dvr: 'DVR',
  latencyMode: 'Latency mode',
};

/**
 * Capability-driven metadata form.
 *
 * Only fields enabled in the platform's capability table are rendered, and
 * validation is produced by the Zod schema built from the same table. Saving
 * writes to the in-memory DEMO store only - no platform API is contacted.
 *
 * The component is remounted per platform (`key` in `MetadataEditor`), so the
 * draft state always starts from the selected platform's stored metadata.
 */
export function MetadataForm({ platform, onSave }: MetadataFormProps) {
  const { capabilities, limits, options } = platform;

  const [draft, setDraft] = useState<PlatformMetadata>(platform.metadata);
  const [errors, setErrors] = useState<MetadataErrors>({});
  const [saved, setSaved] = useState(false);

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
    const result = validateMetadata(platform, draft);
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

  const unsupported = (
    Object.keys(FIELD_LABELS) as (keyof StreamPlatform['capabilities'])[]
  ).filter((field) => !capabilities[field]);

  return (
    <form onSubmit={handleSubmit} noValidate className="space-y-5">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {capabilities.title && (
          <FormField
            label="Title"
            error={errors.title}
            counter={`${draft.title.length} / ${limits.titleMaxLength}`}
            className="lg:col-span-2"
          >
            {({ inputId, describedBy }) => (
              <TextInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.title !== undefined}
                value={draft.title}
                maxLength={limits.titleMaxLength}
                placeholder="What are you streaming?"
                onChange={(event) => patch('title', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.description && (
          <FormField
            label="Description"
            error={errors.description}
            counter={`${draft.description.length} / ${limits.descriptionMaxLength}`}
            className="lg:col-span-2"
          >
            {({ inputId, describedBy }) => (
              <TextArea
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.description !== undefined}
                value={draft.description}
                maxLength={limits.descriptionMaxLength}
                placeholder="Describe the stream"
                onChange={(event) => patch('description', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.category && (
          <FormField label={options.categoryLabel} error={errors.category}>
            {({ inputId, describedBy }) => (
              <TextInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.category !== undefined}
                value={draft.category}
                placeholder={options.categoryPlaceholder}
                onChange={(event) => patch('category', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.language && (
          <FormField label="Language" error={errors.language}>
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
          <FormField label="Visibility" error={errors.visibility}>
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.visibility !== undefined}
                options={options.visibility}
                value={draft.visibility}
                onChange={(event) => patch('visibility', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.latencyMode && (
          <FormField
            label="Latency mode"
            hint="Affects how quickly viewers receive the stream."
            error={errors.latencyMode}
          >
            {({ inputId, describedBy }) => (
              <SelectInput
                id={inputId}
                aria-describedby={describedBy}
                aria-invalid={errors.latencyMode !== undefined}
                options={options.latencyModes}
                value={draft.latencyMode}
                onChange={(event) => patch('latencyMode', event.target.value)}
              />
            )}
          </FormField>
        )}

        {capabilities.tags && (
          <FormField label="Tags" error={errors.tags} className="lg:col-span-2">
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
              label="Mature content"
              description="Flag the stream as intended for adult audiences."
              checked={draft.matureContent}
              onCheckedChange={(checked) => patch('matureContent', checked)}
            />
          )}
          {capabilities.dvr && (
            <ToggleSwitch
              label="DVR"
              description="Allow viewers to rewind the live stream."
              checked={draft.dvr}
              onCheckedChange={(checked) => patch('dvr', checked)}
            />
          )}
        </div>
      )}

      {unsupported.length > 0 && (
        <p className="text-[11px] leading-relaxed text-ink-faint">
          <span className="font-medium text-ink-muted">
            Not available for {platform.name}:
          </span>{' '}
          {unsupported.map((field) => FIELD_LABELS[field]).join(', ')}. Fields are driven by the
          platform capability table, so unsupported fields are not rendered at all.
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2 border-t border-line pt-4">
        <Button type="submit" variant="primary" icon={<Save className="size-3.5" />}>
          Save metadata
        </Button>
        <Button type="button" onClick={handleReset} icon={<RotateCcw className="size-3.5" />}>
          Reset
        </Button>

        <p aria-live="polite" className="ml-auto text-[11px] text-ink-faint">
          {saved ? (
            <span className="inline-flex items-center gap-1 text-status-live">
              <Check aria-hidden="true" className="size-3" />
              Saved to local demo state only
            </span>
          ) : (
            'Changes are kept in memory - nothing is sent to any platform.'
          )}
        </p>
      </div>
    </form>
  );
}
