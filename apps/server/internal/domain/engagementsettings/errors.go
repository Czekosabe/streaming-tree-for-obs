package engagementsettings

import "errors"

// ErrStorage wraps any unexpected persistence failure.
var ErrStorage = errors.New("storage failure")
