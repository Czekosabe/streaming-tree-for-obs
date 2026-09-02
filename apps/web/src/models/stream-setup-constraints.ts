/**
 * Client-side mirrors of a few backend limits.
 *
 * These exist only to give immediate feedback and to set `maxLength` on inputs.
 * The backend remains the authority: every value is validated again server-side,
 * and a rejection there is displayed per field. Keep these in sync with
 * `apps/server/internal/domain/streamsetup/validation.go`.
 */

/** Matches `NameMaxLength` in the domain package. */
export const NAME_MAX_LENGTH = 100;

/** Matches `NoteMaxLength` in the domain package. */
export const NOTE_MAX_LENGTH = 280;

/** Matches `MaxProfiles` in the domain package. */
export const MAX_STREAM_SETUPS = 200;
