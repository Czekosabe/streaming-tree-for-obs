// Package buildinfo exposes static identity information about the binary.
package buildinfo

// ServiceName is reported by the health endpoint and used in log lines.
const ServiceName = "streaming-tree-server"

// Version is the application version. It is kept in sync with the web app's
// package.json version by hand for now; a later stage can inject it at build
// time with -ldflags.
const Version = "0.1.0"
