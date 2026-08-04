// Package credential holds the credential domain: validating and storing a
// destination platform's stream key through the secrets.SecretStore
// abstraction, and reporting its status without ever exposing its value.
//
// Nothing in this package, and nothing it returns to the HTTP layer, carries
// a secret value. The one deliberate exception is
// Service.RetrieveForProcessStart, reserved for the future FFmpeg stage and
// not part of the interface the HTTP layer is given - see service.go.
package credential

// Status is the safe-to-return view of one credential slot: never the
// secret, never its length, never a hash or masked form of it.
type Status struct {
	Configured bool
}

// StoreStatus reports whether the OS credential store could be reached at
// all, independent of any single credential's configured state.
type StoreStatus struct {
	Available bool
}
