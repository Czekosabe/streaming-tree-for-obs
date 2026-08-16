import type { VisualDesignDocument } from '@/api/visualdesign-schemas';
import {
  VISUAL_TEMPLATE_FORMAT,
  type VisualTemplate,
  type VisualTemplateAlertAudio,
  type VisualTemplateFile,
} from '@/api/visualtemplate-schemas';

/**
 * Frontend-only helpers for Stage 14A's visual-template library - see
 * docs/visual-templates.md for the canonical contract.
 */

/** Mirrors the backend's own safeTemplateExportFilename
 * (internal/httpapi/visualtemplate.go) closely enough for a consistent
 * client-suggested filename - the backend's own Content-Disposition
 * header remains authoritative for a raw download; this is used only
 * when the frontend constructs the download client-side from an
 * already-fetched, schema-validated VisualTemplateFile. */
export function safeTemplateExportFilename(name: string): string {
  let safe = '';
  for (const ch of name) {
    if (ch === '/' || ch === '\\' || ch === ':') {
      safe += '-';
    } else if (/\p{Cc}/u.test(ch)) {
      // drop control characters entirely
    } else {
      safe += ch;
    }
  }
  safe = safe.trim();
  if (safe.length > 80) safe = safe.slice(0, 80);
  if (safe === '') safe = 'template';
  return `${safe}.streaming-tree-template.json`;
}

/** Triggers a real browser download of file - the one place this
 * package touches the DOM directly, so every call site is easy to find
 * and audit. */
export function downloadVisualTemplateFile(file: VisualTemplateFile, suggestedName: string): void {
  const blob = new Blob([JSON.stringify(file, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  try {
    const a = document.createElement('a');
    a.href = url;
    a.download = safeTemplateExportFilename(suggestedName);
    a.click();
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** Triggers a real browser download of an already-fetched Blob (Stage
 * 14B package export) - mirrors downloadVisualTemplateFile's own
 * pattern for the JSON case, the one place this file touches the DOM
 * directly for a binary download. */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  try {
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** Reports whether doc references at least one Stage 14B managed asset
 * (an image/video layer, or a custom-font reference on a text-capable
 * layer) - determines whether the JSON export/import path is even
 * offered (docs/visual-template-packages.md §21: a standalone JSON file
 * cannot carry asset bytes, so an asset-backed template must be
 * exported/imported as a package instead). */
export function templateHasAssets(doc: VisualDesignDocument): boolean {
  return doc.layers.some(
    (l) =>
      l.image !== undefined ||
      l.video !== undefined ||
      (l.text?.fontAssetId ?? '') !== '' ||
      (l.messageFragments?.fontAssetId ?? '') !== '',
  );
}

/** Reports whether audio carries any actual configured audio - mirrors
 * the backend's own `visualtemplate.RuleAudioPreset.HasAudio()` exactly
 * (docs/alert-audio.md §10.7: "sound or TTS, either forces package
 * export"). Also gates the JSON export/import path, alongside
 * templateHasAssets above - a template with only a TTS preset (no sound
 * asset at all) still cannot be represented in the Stage 14A JSON
 * schema, which never gains an audio field. */
export function templateHasAudio(audio: VisualTemplateAlertAudio | undefined): boolean {
  return audio !== undefined && (audio.soundEnabled || audio.ttsEnabled);
}

/** Converts a management-shape VisualTemplate into the portable file
 * shape a re-import would accept - used only for round-trip testing and
 * for building an import-preview request from a freshly parsed file;
 * the real export endpoint already returns this shape directly. */
export function toVisualTemplateFile(t: VisualTemplate): VisualTemplateFile {
  return {
    format: VISUAL_TEMPLATE_FORMAT,
    schemaVersion: t.templateSchemaVersion,
    target: t.target,
    name: t.name,
    description: t.description,
    author: t.author,
    license: t.license,
    visualDesign: t.document,
  };
}
