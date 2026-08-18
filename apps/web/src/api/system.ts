import { apiPostNoContent } from '@/lib/api-client';

/**
 * Requests graceful application shutdown - the packaged application's real
 * "Quit Streaming Tree" action (docs/windows-packaging.md §8). The backend
 * requires this exact body shape; there is no generic action/command
 * parameter.
 */
export async function shutdownApplication(): Promise<void> {
  await apiPostNoContent('/api/system/shutdown', { confirm: true });
}
