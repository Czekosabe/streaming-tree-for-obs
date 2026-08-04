/**
 * Client-side mirrors of a few backend limits.
 *
 * These exist only to give immediate feedback and to set `maxLength` on inputs.
 * The backend remains the authority: every value is validated again server-side,
 * and a rejection there is displayed per field. Keep these in sync with
 * `apps/server/internal/domain/platform/validation.go`.
 */

/** Matches `DisplayNameMaxLength` in the domain package. */
export const DISPLAY_NAME_MAX_LENGTH = 80;

/** Matches `SortOrderMax` in the domain package. */
export const SORT_ORDER_MAX = 10_000;

/**
 * Matches `MaxStreamKeyBytes` in `internal/domain/credential`. The backend
 * limit is a byte count (UTF-8); this is a character count, so it is a close
 * approximation for immediate feedback only - the backend is still the
 * authority and validates the actual encoded size.
 */
export const STREAM_KEY_MAX_LENGTH = 4096;
