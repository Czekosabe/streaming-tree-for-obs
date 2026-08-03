import type { ConfiguredPlatform, SaveMetadataInput } from '@/api/platform-schemas';

/**
 * Draft handling for the metadata form.
 *
 * Pure so the unsaved-changes rule can be tested without rendering: it decides
 * whether switching tabs needs a confirmation, so getting it wrong either
 * nags the user constantly or silently discards their edits.
 */

/** Builds an editable draft from the stored metadata of a platform. */
export function toDraft(platform: ConfiguredPlatform): SaveMetadataInput {
  const { metadata } = platform;
  return {
    title: metadata.title,
    description: metadata.description,
    category: metadata.category,
    // Copied so editing the draft cannot mutate the query cache.
    tags: [...metadata.tags],
    language: metadata.language,
    visibility: metadata.visibility,
    matureContent: metadata.matureContent,
    dvr: metadata.dvr,
    latencyMode: metadata.latencyMode,
  };
}

/**
 * Whether a draft differs from what is stored.
 *
 * Tag order counts as a change, because order is user-visible and persisted.
 */
export function isDirty(draft: SaveMetadataInput, stored: SaveMetadataInput): boolean {
  return (
    draft.title !== stored.title ||
    draft.description !== stored.description ||
    draft.category !== stored.category ||
    draft.language !== stored.language ||
    draft.visibility !== stored.visibility ||
    draft.matureContent !== stored.matureContent ||
    draft.dvr !== stored.dvr ||
    draft.latencyMode !== stored.latencyMode ||
    draft.tags.length !== stored.tags.length ||
    draft.tags.some((tag, index) => tag !== stored.tags[index])
  );
}
