import { useMutation, type UseMutationResult } from '@tanstack/react-query';

import { shutdownApplication } from '@/api/system';

/** Requests graceful application shutdown. See api/system.ts's own doc comment. */
export function useShutdownMutation(): UseMutationResult<void, Error, void> {
  return useMutation({
    mutationFn: shutdownApplication,
  });
}
