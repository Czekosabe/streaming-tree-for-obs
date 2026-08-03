import { DISPLAY_NAME_MAX_LENGTH } from '@/models/platform-constraints';

/**
 * Client-side validation for the Add Platform form.
 *
 * Kept as a pure function so it can be tested without rendering, and so the
 * dialog stays about presentation. The backend validates the same rules again
 * and remains the authority.
 */

export type AddPlatformDraft = {
  providerId: string;
  displayName: string;
};

/** Which rule failed, as an identifier the caller maps to a message. */
export type AddPlatformViolation =
  | 'provider-required'
  | 'display-name-required'
  | 'display-name-too-long';

export type AddPlatformValidation = {
  valid: boolean;
  violation: AddPlatformViolation | null;
  /** The trimmed name that should be submitted. */
  displayName: string;
};

export function validateAddPlatform(draft: AddPlatformDraft): AddPlatformValidation {
  const displayName = draft.displayName.trim();

  if (displayName === '') {
    return { valid: false, violation: 'display-name-required', displayName };
  }
  if (displayName.length > DISPLAY_NAME_MAX_LENGTH) {
    return { valid: false, violation: 'display-name-too-long', displayName };
  }
  if (draft.providerId === '') {
    return { valid: false, violation: 'provider-required', displayName };
  }

  return { valid: true, violation: null, displayName };
}
