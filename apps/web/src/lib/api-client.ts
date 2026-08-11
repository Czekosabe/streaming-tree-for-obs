import { z } from 'zod';

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

/**
 * How a request failed. The dashboard distinguishes these so it can tell a
 * stopped backend apart from a rejected payload or a broken contract.
 */
export type ApiErrorKind =
  | 'network'
  | 'timeout'
  | 'http'
  | 'parse'
  | 'validation'
  | 'not-found'
  | 'server';

/** Localization payload for one rejected field, mirroring the backend. */
export type FieldDetail = {
  /** Stable rule identifier, e.g. "too_long". */
  rule: string;
  /** Values a localized message needs, e.g. `{ max: 140 }`. */
  params: Record<string, string | number>;
};

/** Zod shape of the backend error envelope. */
const errorEnvelopeSchema = z.object({
  error: z.string(),
  message: z.string(),
  fields: z.record(z.string(), z.string()).optional(),
  details: z
    .record(
      z.string(),
      z.object({
        rule: z.string(),
        params: z.record(z.string(), z.union([z.string(), z.number()])).optional(),
      }),
    )
    .optional(),
});

export class ApiError extends Error {
  readonly kind: ApiErrorKind;
  readonly status: number | null;
  /**
   * Stable machine-readable code from the backend error payload, e.g.
   * "not_found". The UI maps it to a localized message; it is an identifier,
   * not display text, so it is never translated itself.
   */
  readonly code: string | null;
  /**
   * The backend's own English message. Used as a last-resort fallback when no
   * localized mapping exists for `code`.
   */
  readonly serverMessage: string | null;
  /** English fallback message per rejected field. */
  readonly fields: Record<string, string>;
  /** Stable rule and parameters per rejected field, used for localization. */
  readonly details: Record<string, FieldDetail>;

  constructor(
    kind: ApiErrorKind,
    message: string,
    options: {
      status?: number | null;
      code?: string | null;
      serverMessage?: string | null;
      fields?: Record<string, string>;
      details?: Record<string, FieldDetail>;
    } = {},
  ) {
    super(message);
    this.name = 'ApiError';
    this.kind = kind;
    this.status = options.status ?? null;
    this.code = options.code ?? null;
    this.serverMessage = options.serverMessage ?? null;
    this.fields = options.fields ?? {};
    this.details = options.details ?? {};
  }

  /** True when the backend rejected the payload rather than failing. */
  get isValidation(): boolean {
    return this.kind === 'validation';
  }
}

type ErrorEnvelope = {
  code: string | null;
  serverMessage: string | null;
  fields: Record<string, string>;
  details: Record<string, FieldDetail>;
};

const EMPTY_ENVELOPE: ErrorEnvelope = {
  code: null,
  serverMessage: null,
  fields: {},
  details: {},
};

/**
 * Best-effort extraction of the backend's error envelope.
 *
 * Returns empty values for any response that does not follow it - an error path
 * must never throw while reporting another error.
 */
export async function readErrorEnvelope(response: Response): Promise<ErrorEnvelope> {
  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    return EMPTY_ENVELOPE;
  }

  const parsed = errorEnvelopeSchema.safeParse(payload);
  if (!parsed.success) {
    return EMPTY_ENVELOPE;
  }

  const details: Record<string, FieldDetail> = {};
  for (const [field, detail] of Object.entries(parsed.data.details ?? {})) {
    details[field] = { rule: detail.rule, params: detail.params ?? {} };
  }

  return {
    code: parsed.data.error,
    serverMessage: parsed.data.message,
    fields: parsed.data.fields ?? {},
    details,
  };
}

/** Classifies an HTTP failure so the UI can react to it appropriately. */
export function kindForStatus(status: number, code: string | null): ApiErrorKind {
  if (status === 404) return 'not-found';
  if (status === 422 || code === 'validation_failed' || code === 'unknown_provider') {
    return 'validation';
  }
  if (status >= 500) return 'server';
  return 'http';
}

export function resolveUrl(path: string): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? '';
  if (base === '') return path;
  return `${base.replace(/\/$/, '')}${path}`;
}

type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE';
  body?: unknown;
  timeoutMs?: number;
  signal?: AbortSignal | undefined;
};

/**
 * Performs one request and returns the raw response, normalising every failure
 * into an `ApiError`.
 */
async function send(path: string, options: RequestOptions): Promise<Response> {
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const method = options.method ?? 'GET';

  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), timeoutMs);

  // Forward an externally supplied abort signal (TanStack Query provides one).
  const onExternalAbort = () => controller.abort();
  options.signal?.addEventListener('abort', onExternalAbort);

  const headers: Record<string, string> = { Accept: 'application/json' };
  let payload: string | undefined;
  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(options.body);
  }

  let response: Response;
  try {
    response = await fetch(resolveUrl(path), {
      method,
      headers,
      ...(payload === undefined ? {} : { body: payload }),
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
    const envelope = await readErrorEnvelope(response);
    throw new ApiError(
      kindForStatus(response.status, envelope.code),
      `Backend responded with ${response.status} ${response.statusText}.`,
      { status: response.status, ...envelope },
    );
  }

  return response;
}

/** Reads and validates a JSON response body. */
async function parseBody<TSchema extends z.ZodType>(
  response: Response,
  path: string,
  schema: TSchema,
): Promise<z.infer<TSchema>> {
  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    throw new ApiError('parse', `Backend response for ${path} was not valid JSON.`);
  }

  const parsed = schema.safeParse(payload);
  if (!parsed.success) {
    // A shape mismatch means frontend and backend disagree on the contract.
    // It is reported as its own kind so the UI can say so rather than
    // pretending the backend is down.
    throw new ApiError('parse', `Backend response for ${path} did not match the expected shape.`);
  }

  return parsed.data;
}

export async function apiGet<TSchema extends z.ZodType>(
  path: string,
  schema: TSchema,
  options: { timeoutMs?: number; signal?: AbortSignal | undefined } = {},
): Promise<z.infer<TSchema>> {
  const response = await send(path, { method: 'GET', ...options });
  return parseBody(response, path, schema);
}

export async function apiPost<TSchema extends z.ZodType>(
  path: string,
  body: unknown,
  schema: TSchema,
): Promise<z.infer<TSchema>> {
  const response = await send(path, { method: 'POST', body });
  return parseBody(response, path, schema);
}

export async function apiPut<TSchema extends z.ZodType>(
  path: string,
  body: unknown,
  schema: TSchema,
): Promise<z.infer<TSchema>> {
  const response = await send(path, { method: 'PUT', body });
  return parseBody(response, path, schema);
}

/** DELETE returns 204 with no body, so there is nothing to validate. */
export async function apiDelete(path: string): Promise<void> {
  await send(path, { method: 'DELETE' });
}
