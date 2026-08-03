import type { ParseKeys } from 'i18next';

/**
 * DEMO DATA - static placeholder system metrics.
 *
 * The backend does not report host resource usage yet. These numbers are fixed
 * constants so the panel never pretends to be a live readout; they are rendered
 * behind an explicit "Demo data" badge.
 *
 * Labels are translation keys rather than text: the data layer stays free of
 * display language.
 */

type DashboardKey = ParseKeys<'dashboard'>;

export type ResourceMetric = {
  id: 'cpu' | 'memory' | 'disk';
  labelKey: DashboardKey;
  /** Percentage 0-100. */
  usagePercent: number;
  detailKey: DashboardKey;
};

export type NetworkStatus = {
  statusKey: DashboardKey;
  detailKey: DashboardKey;
  /** Aggregated upload bitrate that would be pushed to all branches. */
  uploadMbps: number;
};

export const DEMO_RESOURCE_METRICS: readonly ResourceMetric[] = [
  {
    id: 'cpu',
    labelKey: 'resources.cpu',
    usagePercent: 38,
    detailKey: 'resources.cpuDetail',
  },
  {
    id: 'memory',
    labelKey: 'resources.memory',
    usagePercent: 54,
    detailKey: 'resources.memoryDetail',
  },
  {
    id: 'disk',
    labelKey: 'resources.disk',
    usagePercent: 71,
    detailKey: 'resources.diskDetail',
  },
];

export const DEMO_NETWORK_STATUS: NetworkStatus = {
  statusKey: 'resources.networkStable',
  detailKey: 'resources.networkDetail',
  uploadMbps: 18.4,
};

/**
 * DEMO connection state of OBS towards the (not yet existing) local ingest.
 * Hard-coded to "waiting" because nothing is listening on the RTMP port.
 */
export const DEMO_OBS_CONNECTION = {
  state: 'waiting' as const,
  labelKey: 'obs.waiting',
  detailKey: 'obs.waitingDetail',
} as const;
