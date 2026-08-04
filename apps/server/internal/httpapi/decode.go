package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxRequestBodyBytes caps write payloads. Platform configuration is small;
// anything larger is a mistake or an attempt to exhaust memory.
const maxRequestBodyBytes = 64 * 1024

// decodeError distinguishes the ways a request body can be unusable, so the
// handler can answer 400 for malformed JSON and 413 for an oversized body
// instead of collapsing both into one status.
type decodeError struct {
	status  int
	code    string
	message string
}

func (e *decodeError) Error() string { return e.message }

// hasRequestBody reports whether the client sent a non-empty body.
//
// Used by command endpoints that document no body, so a client cannot smuggle
// parameters past an endpoint that never reads them.
func hasRequestBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil {
		return false
	}

	limited := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	buffer := make([]byte, 1)
	read, _ := limited.Read(buffer)
	return read > 0
}

// decodeJSON reads a JSON request body strictly.
//
// Unknown fields are rejected so a client typo ("displayname") fails loudly
// instead of being silently dropped, and only one JSON value is accepted so
// trailing garbage cannot be smuggled past validation.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	contentType := r.Header.Get("Content-Type")
	if contentType != "" {
		mediaType := strings.TrimSpace(strings.Split(contentType, ";")[0])
		if !strings.EqualFold(mediaType, "application/json") {
			return &decodeError{
				status:  http.StatusUnsupportedMediaType,
				code:    "unsupported_media_type",
				message: "Request body must be application/json.",
			}
		}
	}

	limited := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return translateDecodeError(err)
	}

	// A second value in the same body means the client sent something we did
	// not fully understand.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &decodeError{
			status:  http.StatusBadRequest,
			code:    "malformed_json",
			message: "Request body must contain exactly one JSON object.",
		}
	}

	return nil
}

func translateDecodeError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return &decodeError{
			status:  http.StatusRequestEntityTooLarge,
			code:    "request_too_large",
			message: fmt.Sprintf("Request body must not exceed %d bytes.", maxRequestBodyBytes),
		}
	}

	var unknownField *json.UnmarshalTypeError
	if errors.As(err, &unknownField) {
		return &decodeError{
			status:  http.StatusBadRequest,
			code:    "malformed_json",
			message: fmt.Sprintf("Field %q has the wrong type.", unknownField.Field),
		}
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return &decodeError{
			status:  http.StatusBadRequest,
			code:    "malformed_json",
			message: fmt.Sprintf("Request body is not valid JSON (offset %d).", syntaxErr.Offset),
		}
	}

	if errors.Is(err, io.EOF) {
		return &decodeError{
			status:  http.StatusBadRequest,
			code:    "malformed_json",
			message: "Request body is empty.",
		}
	}

	// DisallowUnknownFields reports a plain error whose text starts with this
	// prefix; there is no typed error for it in encoding/json.
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		field := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return &decodeError{
			status:  http.StatusBadRequest,
			code:    "unknown_field",
			message: fmt.Sprintf("Request body contains an unknown field %s.", field),
		}
	}

	return &decodeError{
		status:  http.StatusBadRequest,
		code:    "malformed_json",
		message: "Request body could not be read as JSON.",
	}
}
