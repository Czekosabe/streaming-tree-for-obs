import { describe, expect, it } from 'vitest';

import { ApiError } from './api-client';
import { localizeFieldErrors, messageForViolation } from './field-error-rules';

/** Records which key and parameters the mapping asked to render. */
const echoTranslate = (key: string, params: Record<string, string | number>) =>
  `${key}${Object.keys(params).length === 0 ? '' : ` ${JSON.stringify(params)}`}`;

function validationError(
  fields: Record<string, string>,
  details: Record<string, { rule: string; params?: Record<string, string | number> }>,
): ApiError {
  return new ApiError('validation', 'Validation failed', {
    status: 422,
    code: 'validation_failed',
    fields,
    details: Object.fromEntries(
      Object.entries(details).map(([field, detail]) => [
        field,
        { rule: detail.rule, params: detail.params ?? {} },
      ]),
    ),
  });
}

describe('violation to message mapping', () => {
  it('maps a field-specific rule', () => {
    const message = messageForViolation('title', { rule: 'too_long', params: { max: 140 } });

    expect(message).toEqual({ key: 'metadata:validation.titleMaxLength', params: { max: 140 } });
  });

  it('maps the shared unsupported-field rule for any field', () => {
    for (const field of ['tags', 'dvr', 'visibility']) {
      expect(messageForViolation(field, { rule: 'not_supported_by_provider', params: {} })).toEqual(
        { key: 'metadata:validation.notSupported', params: {} },
      );
    }
  });

  it('maps platform configuration rules', () => {
    expect(messageForViolation('displayName', { rule: 'required', params: {} })?.key).toBe(
      'platforms:validation.displayNameRequired',
    );
    expect(messageForViolation('providerId', { rule: 'unsupported', params: {} })?.key).toBe(
      'platforms:validation.providerUnsupported',
    );
  });

  it('returns null for a rule this build does not know', () => {
    expect(messageForViolation('title', { rule: 'invented_later', params: {} })).toBeNull();
  });
});

describe('localizing a failed request', () => {
  it('localizes every field that has a known rule', () => {
    const error = validationError(
      { title: 'Title cannot exceed 140 characters.' },
      { title: { rule: 'too_long', params: { max: 140 } } },
    );

    expect(localizeFieldErrors(error, echoTranslate)).toEqual({
      title: 'metadata:validation.titleMaxLength {"max":140}',
    });
  });

  it('falls back to the English backend sentence for an unmapped rule', () => {
    // The user must always see a sentence, never a rule identifier.
    const error = validationError(
      { title: 'Something the backend explained in English.' },
      { title: { rule: 'invented_later' } },
    );

    expect(localizeFieldErrors(error, echoTranslate)).toEqual({
      title: 'Something the backend explained in English.',
    });
  });

  it('falls back when the backend sent no details at all', () => {
    const error = validationError({ tags: 'Tags are not supported here.' }, {});

    expect(localizeFieldErrors(error, echoTranslate)).toEqual({
      tags: 'Tags are not supported here.',
    });
  });

  it('maps several fields at once', () => {
    const error = validationError(
      { title: 'a', tags: 'b' },
      { title: { rule: 'required' }, tags: { rule: 'duplicate' } },
    );

    expect(Object.keys(localizeFieldErrors(error, echoTranslate)).sort()).toEqual(['tags', 'title']);
  });

  it('returns nothing for a failure that is not a validation rejection', () => {
    const networkError = new ApiError('network', 'Cannot reach the backend.');

    expect(localizeFieldErrors(networkError, echoTranslate)).toEqual({});
  });

  it('returns nothing for a non-ApiError value', () => {
    expect(localizeFieldErrors(new Error('boom'), echoTranslate)).toEqual({});
    expect(localizeFieldErrors(null, echoTranslate)).toEqual({});
  });
});
