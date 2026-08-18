package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Parse decodes raw strictly into a Manifest: unknown fields are
// rejected (mirroring internal/httpapi/decode.go's own convention) and
// exactly one JSON value is required. Parse does not validate field
// values - call Validate on the result before trusting it.
func Parse(raw []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var m Manifest
	if err := decoder.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}

	if err := decoder.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return Manifest{}, fmt.Errorf("%w: manifest must contain exactly one JSON object", ErrInvalid)
	}

	return m, nil
}

// MustMarshal encodes m as indented JSON, panicking on failure - used
// only by the release-build tool (cmd/releasemanifest) and tests, never
// by the runtime updater, which only ever parses a manifest it did not
// author.
func MustMarshal(m Manifest) []byte {
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("manifest: MustMarshal: %v", err))
	}
	return out
}
