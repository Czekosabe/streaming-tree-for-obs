package operatorchat

import "errors"

// ErrClosed is returned by Subscribe once the projection has been shut
// down.
var ErrClosed = errors.New("operator chat projection is closed")
