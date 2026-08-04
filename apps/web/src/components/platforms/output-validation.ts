/**
 * Client-side validation for the destination server-address input.
 *
 * Kept as a pure function so it can be tested without rendering - mirrors
 * `credential-validation.ts`. The backend validates the same rules again and
 * remains the authority; this exists only for immediate feedback.
 */

export type ServerUrlViolation = 'server-url-invalid-scheme' | 'server-url-missing-host';

export type ServerUrlValidation = {
  valid: boolean;
  violation: ServerUrlViolation | null;
  /** The trimmed value that should be submitted. Empty is valid: it means
   * "not configured yet", a legitimate state, not an error. */
  serverUrl: string;
};

export function validateServerUrlDraft(raw: string): ServerUrlValidation {
  const serverUrl = raw.trim();

  if (serverUrl === '') {
    return { valid: true, violation: null, serverUrl };
  }

  let parsed: URL;
  try {
    parsed = new URL(serverUrl);
  } catch {
    return { valid: false, violation: 'server-url-invalid-scheme', serverUrl };
  }

  if (parsed.protocol !== 'rtmp:' && parsed.protocol !== 'rtmps:') {
    return { valid: false, violation: 'server-url-invalid-scheme', serverUrl };
  }
  if (parsed.hostname === '') {
    return { valid: false, violation: 'server-url-missing-host', serverUrl };
  }

  return { valid: true, violation: null, serverUrl };
}
