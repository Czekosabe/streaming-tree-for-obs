package engagement

import "errors"

// ErrInvalidEvent means Validate found the event structurally unacceptable
// (a missing required field, an unknown type, an oversized ProviderExtra
// bag). Wrapped with more detail via fmt.Errorf("%w: ...", ErrInvalidEvent).
var ErrInvalidEvent = errors.New("invalid engagement event")
