import { FileText, Radio, Settings, SlidersHorizontal, Tv } from 'lucide-react';

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
      title="Platforms"
      description="Connect and configure destinations for the streaming tree."
      icon={Tv}
      plannedFor={[
        'Adding and removing platform branches',
        'OAuth sign-in per platform',
        'Stream keys stored in the OS credential store, never in the browser',
        'Per-branch encoding profile (bitrate, resolution, keyframe interval)',
      ]}
    />
  );
}

export function StreamsPage() {
  return (
    <PlaceholderPage
      title="Streams"
      description="Live control of the ingest and of every outgoing branch."
      icon={Radio}
      plannedFor={[
        'MediaMTX ingest state and OBS connection details',
        'Start/stop of individual FFmpeg processes per branch',
        'Live bitrate, dropped frames and reconnect counters',
        'Automatic restart policy for a failed branch',
      ]}
    />
  );
}

export function MetadataPage() {
  return (
    <PlaceholderPage
      title="Metadata"
      description="Stream metadata presets shared across platforms."
      icon={SlidersHorizontal}
      plannedFor={[
        'Reusable presets applied to several platforms at once',
        'Per-platform overrides driven by the capability model',
        'Pushing metadata to platform APIs before going live',
        'History of previously used titles and categories',
      ]}
    />
  );
}

export function SettingsPage() {
  return (
    <PlaceholderPage
      title="Settings"
      description="Global application and router configuration."
      icon={Settings}
      plannedFor={[
        'Local ingest configuration (RTMP port, path, authentication)',
        'Paths to the MediaMTX and FFmpeg binaries',
        'Backend address, for the future remote-router deployment',
        'Credential store management',
      ]}
    />
  );
}

export function LogsPage() {
  return (
    <PlaceholderPage
      title="Logs"
      description="Diagnostics from the router and from every branch."
      icon={FileText}
      plannedFor={[
        'Structured backend logs streamed over SSE or WebSocket',
        'Per-branch FFmpeg output with severity filtering',
        'Export of a diagnostic bundle with secrets stripped',
      ]}
    />
  );
}
