package updatersettings

import "errors"

// ErrStorage wraps any unexpected persistence failure - the HTTP layer
// maps it to a generic 500, never surfacing the underlying driver error
// to a client.
var ErrStorage = errors.New("storage failure")
