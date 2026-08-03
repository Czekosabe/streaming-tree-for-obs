import type { z } from 'zod';

/**
 * Minimal typed fetch wrapper around the local Go backend.
 *
 * Responsibilities kept deliberately small at this stage:
 *  - resolve the base URL (empty by default, so requests stay same-origin and
 *    are proxied by the Vite dev server),
 *  - apply a request timeout so an unreachable backend fails fast instead of
 *    hanging the UI,
 *  - normalise every failure into `ApiError` with a message a human can read,
 *  - validate the payload with the caller's Zod schema.
 */

const DEFAULT_TIMEOUT_MS = 5_000;

export type ApiErrorKind = 'network' | 'timeout' | 'http' | 'parse';

export class ApiError extends Error {
  readonly kind: ApiErrorKind;
  readonly status: number | null;

  constructor(kind: ApiErrorKind, message: string, status: number | null = null) {
    super(message);
    this.name = 'ApiError';
    this.kind = kind;
    this.status = status;
  }
}

function resolveUrl(path: string): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  if (base === '') return path;
  return `${base.replace(/\/$/, '')}${path}`;
}

export async function apiGet<TSchema extends z.ZodType>(
  path: string,
  schema: TSchema,
  options: { timeoutMs?: number; signal?: AbortSignal } = {},
): Promise<z.infer<TSchema>> {
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), timeoutMs);

  // Forward an externally supplied abort signal (TanStack Query provides one).
  const onExternalAbort = () => controller.abort();
  options.signal?.addEventListener('abort', onExternalAbort);

  let response: Response;
  try {
    response = await fetch(resolveUrl(path), {
      method: 'GET',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError('timeout', `Request to ${path} timed out after ${timeoutMs} ms.`);
    }
    throw new ApiError('network', `Cannot reach the backend at ${resolveUrl(path)}.`);
  } finally {
    window.clearTimeout(timeoutId);
    options.signal?.removeEventListener('abort', onExternalAbort);
  }

  if (!response.ok) {
    throw new ApiError(
      'http',
      `Backend responded with ${response.status} ${response.statusText}.`,
      response.status,
    );
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    throw new ApiError('parse', `Backend response for ${path} was not valid JSON.`);
  }

  const parsed = schema.safeParse(payload);
  if (!parsed.success) {
    throw new ApiError('parse', `Backend response for ${path} did not match the expected shape.`);
  }

  return parsed.data;
}
