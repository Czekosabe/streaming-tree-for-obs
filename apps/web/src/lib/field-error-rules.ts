import type { ParseKeys } from 'i18next';

import { ApiError, type FieldDetail } from './api-client';

/**
 * Any key of the two namespaces validation messages live in. Typing the tables
 * with it means a renamed translation key fails the build here.
 */
export type FieldMessageKey = ParseKeys<['metadata', 'platforms']>;

/**
 * Maps a backend field violation onto a local message.
 *
 * The backend sends a stable rule identifier plus parameters (`too_long`,
 * `{max: 140}`). This module turns `field + rule` into a message key and its
 * interpolation values; the caller supplies the actual translation function.
 *
 * Kept free of React and i18next so the mapping rules can be tested directly.
 */

/** A message to render: a namespaced key plus its interpolation values. */
export type LocalizedFieldMessage = {
  key: FieldMessageKey;
  params: Record<string, string | number>;
};

/** Rules shared by every field: whichever field it was, the message is one. */
const SHARED_RULE_KEYS: Record<string, FieldMessageKey> = {
  not_supported_by_provider: 'metadata:validation.notSupported',
};

/** `field:rule` pairs with a dedicated message. */
const FIELD_RULE_KEYS: Record<string, FieldMessageKey> = {
  'title:required': 'metadata:validation.titleRequired',
  'title:too_long': 'metadata:validation.titleMaxLength',
  'description:too_long': 'metadata:validation.descriptionMaxLength',
  'category:required': 'metadata:validation.categoryRequired',
  'category:too_long': 'metadata:validation.categoryMaxLength',
  'tags:too_short': 'metadata:validation.tagMinLength',
  'tags:too_long': 'metadata:validation.tagMaxLength',
  'tags:too_many': 'metadata:validation.tagsMaxCount',
  'tags:duplicate': 'metadata:validation.tagsUnique',
  'tags:invalid': 'metadata:validation.tagPattern',
  'language:unsupported': 'metadata:validation.languageUnsupported',
  'visibility:unsupported': 'metadata:validation.visibilityUnsupported',
  'latencyMode:unsupported': 'metadata:validation.latencyModeUnsupported',
  'displayName:required': 'platforms:validation.displayNameRequired',
  'displayName:too_long': 'platforms:validation.displayNameTooLong',
  'providerId:unsupported': 'platforms:validation.providerUnsupported',
  'sortOrder:invalid': 'platforms:validation.sortOrderInvalid',
};

/**
 * Resolves the message for one violation.
 *
 * Returns `null` when this build has no mapping - a newer backend rule, say -
 * so the caller falls back to the English sentence the backend already sent.
 * A user must always see a sentence, never a rule identifier.
 */
export function messageForViolation(
  field: string,
  detail: FieldDetail,
): LocalizedFieldMessage | null {
  const specific = FIELD_RULE_KEYS[`${field}:${detail.rule}`];
  if (specific !== undefined) {
    return { key: specific, params: detail.params };
  }

  const shared = SHARED_RULE_KEYS[detail.rule];
  if (shared !== undefined) {
    return { key: shared, params: detail.params };
  }

  return null;
}

/**
 * Turns a failed request into localized per-field messages.
 *
 * `translate` receives a namespaced key and its parameters. Anything that is
 * not a validation rejection yields an empty map, so callers can render field
 * errors and a general failure message independently.
 */
export function localizeFieldErrors(
  error: unknown,
  translate: (key: FieldMessageKey, params: Record<string, string | number>) => string,
): Record<string, string> {
  if (!(error instanceof ApiError) || !error.isValidation) {
    return {};
  }

  const localized: Record<string, string> = {};

  for (const [field, fallback] of Object.entries(error.fields)) {
    const detail = error.details[field];
    if (detail === undefined) {
      localized[field] = fallback;
      continue;
    }

    const message = messageForViolation(field, detail);
    localized[field] = message === null ? fallback : translate(message.key, message.params);
  }

  return localized;
}
