// Package config loads the server configuration from environment variables.
//
// No configuration value is a secret at this stage. When stream keys and OAuth
// tokens arrive they will NOT be read from the environment or from files in the
// repository, but from the operating system credential store.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every tunable of the HTTP server.
type Config struct {
	// Host is the interface to bind to. Defaults to 127.0.0.1 so a development
	// build is not exposed to the local network by accident.
	Host string

	// Port is the TCP port of the REST API.
	Port int

	// AllowedOrigins lists the browser origins accepted by the CORS middleware.
	AllowedOrigins []string

	// ReadHeaderTimeout guards against slow-header attacks.
	ReadHeaderTimeout time.Duration

	// ShutdownTimeout bounds how long in-flight requests may finish during a
	// graceful shutdown.
	ShutdownTimeout time.Duration
}

const (
	defaultHost              = "127.0.0.1"
	defaultPort              = 8080
	defaultReadHeaderTimeout = 5 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
)

// defaultAllowedOrigins covers the Vite dev server on both loopback spellings.
var defaultAllowedOrigins = []string{
	"http://localhost:5173",
	"http://127.0.0.1:5173",
}

// Load reads the configuration from the environment, applying defaults for
// anything not set. It returns an error when a variable is present but invalid,
// so a typo fails loudly at startup instead of silently falling back.
func Load() (Config, error) {
	cfg := Config{
		Host:              defaultHost,
		Port:              defaultPort,
		AllowedOrigins:    defaultAllowedOrigins,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ShutdownTimeout:   defaultShutdownTimeout,
	}

	if raw, ok := lookup("STREAMING_TREE_HOST"); ok {
		cfg.Host = raw
	}

	if raw, ok := lookup("STREAMING_TREE_PORT"); ok {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("STREAMING_TREE_PORT: %q is not a number", raw)
		}
		if port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("STREAMING_TREE_PORT: %d is outside the range 1-65535", port)
		}
		cfg.Port = port
	}

	if raw, ok := lookup("STREAMING_TREE_ALLOWED_ORIGINS"); ok {
		origins := splitAndTrim(raw)
		if len(origins) == 0 {
			return Config{}, fmt.Errorf("STREAMING_TREE_ALLOWED_ORIGINS: no origin left after parsing %q", raw)
		}
		cfg.AllowedOrigins = origins
	}

	return cfg, nil
}

// Address returns the host:port string accepted by net.Listen.
func (c Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// lookup returns the trimmed value of an environment variable, treating an
// empty or whitespace-only value as "not set".
func lookup(key string) (string, bool) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
