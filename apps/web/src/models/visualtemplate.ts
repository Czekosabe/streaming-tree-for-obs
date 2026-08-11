import { VISUAL_TEMPLATE_FORMAT, type VisualTemplate, type VisualTemplateFile } from '@/api/visualtemplate-schemas';

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
