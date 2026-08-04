import { STREAM_KEY_MAX_LENGTH } from '@/models/platform-constraints';

/**
 * Client-side validation for the stream-key input.
 *
 * Kept as a pure function so it can be tested without rendering, and so the
 * dialog stays about presentation - mirrors `add-platform-validation.ts`.
 * The backend validates the same rules again and remains the authority; this
 * exists only to give immediate feedback without a round trip.
 */

/** Which rule failed, as an identifier the caller maps to a message. */
export type StreamKeyViolation = 'stream-key-required' | 'stream-key-too-long' | 'stream-key-invalid';

export type StreamKeyValidation = {
  valid: boolean;
  violation: StreamKeyViolation | null;
  /** The trimmed value that should be submitted. */
  streamKey: string;
};

// Every Unicode control character, including every line-break form -
// mirrors the backend's unicode.IsControl check.
const CONTROL_CHARACTER_PATTERN = /[\p{Cc}]/u;

export function validateStreamKeyDraft(raw: string): StreamKeyValidation {
  const streamKey = raw.trim();

  if (streamKey === '') {
    return { valid: false, violation: 'stream-key-required', streamKey };
  }
  if (streamKey.length > STREAM_KEY_MAX_LENGTH) {
    return { valid: false, violation: 'stream-key-too-long', streamKey };
  }
  if (CONTROL_CHARACTER_PATTERN.test(streamKey)) {
    return { valid: false, violation: 'stream-key-invalid', streamKey };
  }

  return { valid: true, violation: null, streamKey };
}
