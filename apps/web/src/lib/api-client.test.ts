import { afterEach, describe, expect, it, vi } from 'vitest';
import { z } from 'zod';

import { ApiError, apiDelete, apiGet, apiPost, setCSRFToken } from './api-client';

/** Builds a fetch stub returning one response. */
function mockFetch(status: number, body: unknown, ok = status < 400) {
  const payload = typeof body === 'string' ? body : JSON.stringify(body);
  return vi.fn().mockResolvedValue({
    ok,
    status,
    statusText: String(status),
    json: async () => JSON.parse(payload),
    text: async () => payload,
  } as unknown as Response);
}

const schema = z.object({ value: z.string() });

afterEach(() => {
  vi.unstubAllGlobals();
  setCSRFToken(null);
});

describe('Stage 20D2B CSRF header attachment', () => {
  it('attaches X-CSRF-Token on POST when a token is set', async () => {
    const fetchMock = mockFetch(200, { value: 'ok' });
    vi.stubGlobal('fetch', fetchMock);
    setCSRFToken('a-csrf-token');

    await apiPost('/api/test', {}, schema);

    const [, init] = fetchMock.mock.calls[0] ?? [];
    expect(init?.headers).toMatchObject({ 'X-CSRF-Token': 'a-csrf-token' });
  });

  it('attaches X-CSRF-Token on DELETE when a token is set', async () => {
    const fetchMock = mockFetch(204, '');
    vi.stubGlobal('fetch', fetchMock);
    setCSRFToken('a-csrf-token');

    await apiDelete('/api/test');

    const [, init] = fetchMock.mock.calls[0] ?? [];
    expect(init?.headers).toMatchObject({ 'X-CSRF-Token': 'a-csrf-token' });
  });

  it('does not attach X-CSRF-Token on GET even when a token is set', async () => {
    const fetchMock = mockFetch(200, { value: 'ok' });
    vi.stubGlobal('fetch', fetchMock);
    setCSRFToken('a-csrf-token');

    await apiGet('/api/test', schema);

    const [, init] = fetchMock.mock.calls[0] ?? [];
    const headers = (init?.headers ?? {}) as Record<string, string>;
    expect(headers['X-CSRF-Token']).toBeUndefined();
  });

  it('does not attach X-CSRF-Token when no token has been set', async () => {
    const fetchMock = mockFetch(201, { value: 'created' });
    vi.stubGlobal('fetch', fetchMock);

    await apiPost('/api/test', {}, schema);

    const [, init] = fetchMock.mock.calls[0] ?? [];
    const headers = (init?.headers ?? {}) as Record<string, string>;
    expect(headers['X-CSRF-Token']).toBeUndefined();
  });
});

describe('successful requests', () => {
  it('validates and returns the parsed body', async () => {
    vi.stubGlobal('fetch', mockFetch(200, { value: 'ok' }));

    await expect(apiGet('/api/test', schema)).resolves.toEqual({ value: 'ok' });
  });

  it('sends a JSON body on POST', async () => {
    const fetchMock = mockFetch(201, { value: 'created' });
    vi.stubGlobal('fetch', fetchMock);

    await apiPost('/api/test', { name: 'x' }, schema);

    const [, init] = fetchMock.mock.calls[0] ?? [];
    expect(init?.method).toBe('POST');
    expect(init?.body).toBe(JSON.stringify({ name: 'x' }));
    expect(init?.headers).toMatchObject({ 'Content-Type': 'application/json' });
  });

  it('does not parse a body for DELETE', async () => {
    vi.stubGlobal('fetch', mockFetch(204, ''));

    await expect(apiDelete('/api/test/1')).resolves.toBeUndefined();
  });
});

describe('failure classification', () => {
  it('reports an unreachable backend as a network failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('failed to fetch')));

    await expect(apiGet('/api/test', schema)).rejects.toMatchObject({ kind: 'network' });
  });

  it('reports a 404 as not-found', async () => {
    vi.stubGlobal('fetch', mockFetch(404, { error: 'not_found', message: 'Missing.' }));

    const error = await apiGet('/api/test', schema).catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).kind).toBe('not-found');
    expect((error as ApiError).code).toBe('not_found');
  });

  it('reports a 500 as a server failure', async () => {
    vi.stubGlobal('fetch', mockFetch(500, { error: 'internal_error', message: 'Boom.' }));

    await expect(apiGet('/api/test', schema)).rejects.toMatchObject({ kind: 'server' });
  });

  it('reports a malformed response shape as a parse failure', async () => {
    // The backend answered 200 but with a payload the contract does not allow.
    vi.stubGlobal('fetch', mockFetch(200, { unexpected: true }));

    await expect(apiGet('/api/test', schema)).rejects.toMatchObject({ kind: 'parse' });
  });
});

describe('validation error envelope', () => {
  it('captures field messages and localization details', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetch(422, {
        error: 'validation_failed',
        message: 'Validation failed',
        fields: { title: 'Title cannot exceed 140 characters.' },
        details: { title: { rule: 'too_long', params: { max: 140 } } },
      }),
    );

    const error = (await apiPost('/api/test', {}, schema).catch(
      (caught: unknown) => caught,
    )) as ApiError;

    expect(error.kind).toBe('validation');
    expect(error.isValidation).toBe(true);
    expect(error.fields.title).toBe('Title cannot exceed 140 characters.');
    expect(error.details.title).toEqual({ rule: 'too_long', params: { max: 140 } });
  });

  it('treats an unknown provider rejection as a validation failure', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetch(422, { error: 'unknown_provider', message: 'Not supported.' }),
    );

    const error = (await apiPost('/api/test', {}, schema).catch(
      (caught: unknown) => caught,
    )) as ApiError;

    expect(error.isValidation).toBe(true);
    expect(error.code).toBe('unknown_provider');
  });

  it('survives an error response that does not follow the envelope', async () => {
    // Reporting an error must never throw a second error.
    vi.stubGlobal('fetch', mockFetch(500, 'plain text failure', false));

    const error = (await apiGet('/api/test', schema).catch((caught: unknown) => caught)) as ApiError;

    expect(error).toBeInstanceOf(ApiError);
    expect(error.code).toBeNull();
    expect(error.fields).toEqual({});
  });
});
