import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import {
  createVisualTemplate,
  deleteVisualTemplate,
  exportVisualTemplate,
  fetchVisualTemplates,
  importVisualTemplate,
  previewVisualTemplateImport,
  updateVisualTemplateMetadata,
} from '@/api/visualtemplate';
import type { VisualDesignDocument } from '@/api/visualdesign-schemas';
import type { VisualTemplate, VisualTemplateFile, VisualTemplateTarget } from '@/api/visualtemplate-schemas';
import {
  cancelVisualTemplatePackagePreview,
  exportVisualTemplatePackage,
  importVisualTemplatePackage,
  previewVisualTemplatePackageImport,
} from '@/api/visualpackage';
import type { VisualTemplatePackagePreview } from '@/api/visualpackage-schemas';

/** Shared by both Designers (Stage 14A task Part 32) - one gallery
 * implementation, scoped by an optional real owner instance so the
 * backend can additionally return per-template compatibility
 * (docs/visual-templates.md §9). */
export const visualTemplateKeys = {
  list: (ownerContext?: { target: VisualTemplateTarget; ownerId: string }) =>
    ['visual-templates', ownerContext?.target ?? null, ownerContext?.ownerId ?? null] as const,
};

export function useVisualTemplatesQuery(
  ownerContext?: { target: VisualTemplateTarget; ownerId: string },
  options: { enabled?: boolean } = {},
): UseQueryResult<VisualTemplate[], Error> {
  return useQuery({
    queryKey: visualTemplateKeys.list(ownerContext),
    queryFn: ({ signal }) => fetchVisualTemplates(ownerContext, signal),
    enabled: options.enabled ?? true,
  });
}

export function useCreateVisualTemplateMutation(): UseMutationResult<
  VisualTemplate,
  Error,
  { target: VisualTemplateTarget; name: string; description: string; author: string; license: string; document: VisualDesignDocument }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input) => createVisualTemplate(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['visual-templates'] });
    },
  });
}

export function useUpdateVisualTemplateMetadataMutation(
  id: string,
): UseMutationResult<VisualTemplate, Error, { name: string; description: string; author: string; license: string }> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input) => updateVisualTemplateMetadata(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['visual-templates'] });
    },
  });
}

export function useDeleteVisualTemplateMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id) => deleteVisualTemplate(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['visual-templates'] });
    },
  });
}

/** Never persists anything - see previewVisualTemplateImport's own doc
 * comment. Not cached by React Query (a preview is a one-shot action,
 * not read state). */
export function useImportVisualTemplatePreviewMutation(): UseMutationResult<
  VisualTemplate,
  Error,
  { file: VisualTemplateFile; ownerContext?: { target: VisualTemplateTarget; ownerId: string } }
> {
  return useMutation({
    mutationFn: ({ file, ownerContext }) => previewVisualTemplateImport(file, ownerContext),
  });
}

export function useImportVisualTemplateMutation(): UseMutationResult<VisualTemplate, Error, VisualTemplateFile> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file) => importVisualTemplate(file),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['visual-templates'] });
    },
  });
}

export function useExportVisualTemplateMutation(): UseMutationResult<VisualTemplateFile, Error, string> {
  return useMutation({
    mutationFn: (id) => exportVisualTemplate(id),
  });
}

// --- Stage 14B: portable package import/preview/export -------------------

/** Never persists anything (docs/visual-template-packages.md §43) - not
 * cached by React Query, exactly like useImportVisualTemplatePreviewMutation. */
export function useImportVisualTemplatePackagePreviewMutation(): UseMutationResult<VisualTemplatePackagePreview, Error, File> {
  return useMutation({
    mutationFn: (file) => previewVisualTemplatePackageImport(file),
  });
}

/** Best-effort - see cancelVisualTemplatePackagePreview's own doc
 * comment. Never surfaces an error to the caller. */
export function useCancelVisualTemplatePackagePreviewMutation(): UseMutationResult<void, Error, string> {
  return useMutation({
    mutationFn: (token) => cancelVisualTemplatePackagePreview(token),
  });
}

export function useImportVisualTemplatePackageMutation(): UseMutationResult<VisualTemplate, Error, File> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file) => importVisualTemplatePackage(file),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['visual-templates'] });
    },
  });
}

export function useExportVisualTemplatePackageMutation(): UseMutationResult<{ blob: Blob; filename: string }, Error, string> {
  return useMutation({
    mutationFn: (id) => exportVisualTemplatePackage(id),
  });
}
