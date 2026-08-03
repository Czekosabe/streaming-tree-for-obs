package mediamtx

import (
	"encoding/json"
	"runtime"
)

// currentGOOS and currentGOARCH exist so tests can reason about the platform
// without importing runtime everywhere.
func currentGOOS() string   { return runtime.GOOS }
func currentGOARCH() string { return runtime.GOARCH }

// marshalIndent produces readable JSON for on-disk metadata.
func marshalIndent(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
