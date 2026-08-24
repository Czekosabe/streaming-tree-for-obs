import { SlidersHorizontal, Tv } from 'lucide-react';

import { PlaceholderPage } from './PlaceholderPage';

/**
 * Placeholder routes.
 *
 * Grouped in one file because each is a single call with different copy; they
 * will be replaced by real pages one stage at a time.
 */

export function PlatformsPage() {
  return (
    <PlaceholderPage
      titleKey="platforms.title"
      descriptionKey="platforms.description"
      icon={Tv}
      plannedKeys={[
        'platforms.planned.manageBranches',
        'platforms.planned.oauth',
        'platforms.planned.credentials',
        'platforms.planned.encoding',
      ]}
    />
  );
}

// StreamsPage is no longer a placeholder: it shows the real local ingest state
// and lives in its own file, `StreamsPage.tsx`.

export function MetadataPage() {
  return (
    <PlaceholderPage
      titleKey="metadata.title"
      descriptionKey="metadata.description"
      icon={SlidersHorizontal}
      plannedKeys={[
        'metadata.planned.presets',
        'metadata.planned.overrides',
        'metadata.planned.push',
        'metadata.planned.history',
      ]}
    />
  );
}

// SettingsPage is no longer a placeholder: it shows real connected-account
// management and lives in its own file, `SettingsPage.tsx`.

// LogsPage is no longer a placeholder: it shows the real Stage 20E
// diagnostics API and lives in its own file, `LogsPage.tsx`.
