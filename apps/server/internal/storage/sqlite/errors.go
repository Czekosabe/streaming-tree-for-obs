package sqlite

import (
	"errors"
	"strings"

	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"
)

// isUniqueViolation reports whether the driver rejected a write because of a
// UNIQUE or PRIMARY KEY constraint.
//
// The typed driver error is checked first; the string match is a fallback for
// wrapped errors, so a driver change cannot silently turn a conflict into a
// generic 500.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var sqliteErr *sqlite3.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		if code == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE ||
			code == sqlite3lib.SQLITE_CONSTRAINT_PRIMARYKEY {
			return true
		}
	}

	return strings.Contains(strings.ToUpper(err.Error()), "UNIQUE CONSTRAINT FAILED")
}

// isForeignKeyViolation reports whether the driver rejected a write because
// it referenced a row that does not exist (a platform or account id that is
// not really there).
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}

	var sqliteErr *sqlite3.Error
	if errors.As(err, &sqliteErr) {
		if sqliteErr.Code() == sqlite3lib.SQLITE_CONSTRAINT_FOREIGNKEY {
			return true
		}
	}

	return strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY CONSTRAINT FAILED")
}
