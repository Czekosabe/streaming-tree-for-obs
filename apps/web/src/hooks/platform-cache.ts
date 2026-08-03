import type { ConfiguredPlatform, PlatformMetadata } from '@/api/platform-schemas';

/**
 * Pure cache transformations used by the platform mutations.
 *
 * Extracted from the hooks so the update rules can be tested directly, without
 * rendering a component or driving a query client.
 */

/** Replaces one platform, leaving the rest and their order untouched. */
export function replacePlatform(
  platforms: ConfiguredPlatform[] | undefined,
  updated: ConfiguredPlatform,
): ConfiguredPlatform[] | undefined {
  return platforms?.map((platform) => (platform.id === updated.id ? updated : platform));
}

/** Removes one platform by id. */
export function removePlatform(
  platforms: ConfiguredPlatform[] | undefined,
  id: string,
): ConfiguredPlatform[] | undefined {
  return platforms?.filter((platform) => platform.id !== id);
}

/** Replaces the metadata of one platform, keeping its configuration fields. */
export function replaceMetadata(
  platforms: ConfiguredPlatform[] | undefined,
  id: string,
  metadata: PlatformMetadata,
): ConfiguredPlatform[] | undefined {
  return platforms?.map((platform) =>
    platform.id === id ? { ...platform, metadata } : platform,
  );
}
