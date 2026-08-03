import { QueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api-client';

/**
 * Shared TanStack Query client.
 *
 * A locally running backend either answers immediately or is simply not up, so
 * retries are limited: one retry for transient network hiccups, none for
 * responses that were received but rejected (HTTP errors, schema mismatches).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      refetchOnWindowFocus: true,
      retry: (failureCount, error) => {
        if (error instanceof ApiError && (error.kind === 'http' || error.kind === 'parse')) {
          return false;
        }
        return failureCount < 1;
      },
      retryDelay: 1_000,
    },
  },
});
