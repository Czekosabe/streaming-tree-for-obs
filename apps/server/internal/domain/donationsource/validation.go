package donationsource

import (
	"fmt"
	"unicode/utf8"
)

// LabelMaxLength bounds the operator-chosen label, mirroring
// platform.DisplayNameMaxLength.
const LabelMaxLength = 80

// RemoteChannelIDMaxLength bounds the safe remote channel identifier -
// generous for any provider's own id format, still a defensive bound
// against an unbounded paste.
const RemoteChannelIDMaxLength = 128

// MaxCredentialBytes bounds the accepted credential length - a real
// StreamElements JWT is a few hundred bytes; this is a conservative
// ceiling against an accidental paste of something much larger, never a
// claim about StreamElements' own JWT format.
const MaxCredentialBytes = 8 * 1024

func validateLabel(label string) error {
	if label == "" {
		return fmt.Errorf("%w: label is required", ErrInvalidLabel)
	}
	if utf8.RuneCountInString(label) > LabelMaxLength {
		return fmt.Errorf("%w: label exceeds %d characters", ErrInvalidLabel, LabelMaxLength)
	}
	return nil
}

func validateRemoteChannelID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: remote channel id is required", ErrInvalidRemoteChannelID)
	}
	if utf8.RuneCountInString(id) > RemoteChannelIDMaxLength {
		return fmt.Errorf("%w: remote channel id exceeds %d characters", ErrInvalidRemoteChannelID, RemoteChannelIDMaxLength)
	}
	return nil
}

func validateProvider(p ProviderID) error {
	if !p.valid() {
		return fmt.Errorf("%w: %q", ErrInvalidProvider, p)
	}
	return nil
}

func validateCredential(token string) error {
	if token == "" {
		return ErrCredentialRequired
	}
	if len(token) > MaxCredentialBytes {
		return ErrCredentialTooLong
	}
	return nil
}
