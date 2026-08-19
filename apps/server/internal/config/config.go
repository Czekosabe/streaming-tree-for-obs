// Package config loads the server configuration from environment variables.
//
// No configuration value is a secret at this stage. When stream keys and OAuth
// tokens arrive they will NOT be read from the environment or from files in the
// repository, but from the operating system credential store.
package config

import (
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
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

	// DatabasePath is the resolved absolute path of the SQLite file. It is safe
	// to log: the application stores no credentials anywhere.
	DatabasePath string

	// DataDir is the per-user application data directory. It holds the managed
	// MediaMTX installation and the generated runtime configuration.
	DataDir string

	// MediaMTX groups the local ingest runtime settings.
	MediaMTX MediaMTXConfig

	// FFmpeg groups the destination-branch runtime settings.
	FFmpeg FFmpegConfig

	// TwitchClientID is the environment override for the Twitch Client ID.
	// Empty means no override - see internal/domain/account.SourceEnvironment
	// and internal/domain/account.SourceDatabase.
	TwitchClientID string

	// YouTubeClientID is the environment override for the YouTube (Google
	// Desktop OAuth client) Client ID. Empty means no override - same
	// precedence rule as TwitchClientID.
	YouTubeClientID string

	// EngagementBufferSize is the Engagement Event Bus's in-memory ring
	// buffer capacity - see internal/engagement.Bus. Never persisted; reset
	// to this configured value on every backend start, since the buffer
	// itself always starts empty.
	EngagementBufferSize int

	// OperatorChatBufferSize is the Stage 9 unified-operator-chat
	// projection's in-memory ring buffer capacity - see
	// internal/operatorchat.Projection. Independent of
	// EngagementBufferSize (the projection's own capacity, not the Event
	// Bus's) and never persisted, for the same reason.
	OperatorChatBufferSize int

	// TestNoUI suppresses packaged-mode OS-level side effects (opening the
	// default browser, showing a native fatal-startup-error dialog) that
	// are unsafe/undesirable to trigger for real during an automated test
	// (docs/windows-packaging.md §30). It has no effect at all unless the
	// binary was also built packaged (buildinfo.Packaged()) - a
	// development build never reads this differently, and it can never be
	// set by an incoming HTTP request, only by the process's own
	// environment at startup.
	TestNoUI bool

	// RemoteManagement groups the Stage 20D2B remote-management
	// deployment-security settings (docs/remote-management.md §4) -
	// read once, here, from the process environment at startup, and
	// never mutable through any HTTP route: no handler in this codebase
	// ever re-invokes config.Load() or writes to process environment.
	RemoteManagement RemoteManagementConfig
}

// RemoteManagementConfig groups Stage 20D2B's security-critical
// deployment configuration (docs/remote-management.md §4) - never
// persisted as an ordinary, remotely-editable application setting.
type RemoteManagementConfig struct {
	// Enabled is the explicit --remote-management opt-in
	// (docs/remote-management.md §3). Never inferred from --headless
	// alone; checked against it explicitly at startup (only valid when
	// Headless is also true).
	Enabled bool

	// ExternalOrigin is the one canonical external HTTPS origin
	// browser clients use (docs/remote-management.md §6), e.g.
	// "https://stream.example.com". Empty unless Enabled.
	ExternalOrigin string
}

// FFmpegConfig configures FFmpeg executable resolution for destination
// branches.
type FFmpegConfig struct {
	// ExecutablePath is an explicit override for the FFmpeg binary. Empty
	// means "search the bundled location, then PATH" - see
	// internal/runtime/ffmpeg.Resolver.
	ExecutablePath string
}

// MediaMTXConfig configures the local MediaMTX ingest service.
//
// Both listeners are restricted to loopback in this local version: MediaMTX
// accepts an unauthenticated publisher on the configured path, and the Control
// API can change its configuration, so neither may be reachable from the
// network.
type MediaMTXConfig struct {
	// ExecutablePath is an explicit override for the MediaMTX binary. Empty
	// means "use the application-managed installation".
	ExecutablePath string

	// AutoStart starts MediaMTX when the backend starts.
	AutoStart bool

	// AutoRestart restarts MediaMTX after an unexpected exit.
	AutoRestart bool

	// RTMPAddress is the loopback address OBS publishes to.
	RTMPAddress string

	// APIAddress is the loopback address of the MediaMTX Control API. Only the
	// Go backend ever talks to it; it is never exposed to the browser.
	APIAddress string

	// IngestPath is the single MediaMTX path publishing is allowed on. It is a
	// route identifier, not a secret.
	IngestPath string
}

const (
	defaultHost              = "127.0.0.1"
	defaultPort              = 8080
	defaultReadHeaderTimeout = 5 * time.Second
	defaultShutdownTimeout   = 10 * time.Second

	// DatabaseFileName is the SQLite file created inside the data directory.
	DatabaseFileName = "streaming-tree.db"

	// AppDirName is the per-user folder holding application data.
	AppDirName = "StreamingTree"

	// DefaultRTMPAddress is the loopback address OBS publishes to.
	DefaultRTMPAddress = "127.0.0.1:1935"

	// DefaultAPIAddress is the loopback address of the MediaMTX Control API.
	DefaultAPIAddress = "127.0.0.1:9997"

	// DefaultIngestPath is the single MediaMTX path publishing is allowed on.
	// It is a route identifier, not a secret.
	DefaultIngestPath = "live"

	// defaultEngagementBufferSize, minEngagementBufferSize and
	// maxEngagementBufferSize mirror internal/engagement's
	// DefaultCapacity/MinCapacity/MaxCapacity exactly - duplicated here
	// (rather than importing that package from this low-level one, which
	// every other package already depends on) and kept in sync by
	// TestEngagementBufferSizeConstantsMatchBusPackage.
	defaultEngagementBufferSize = 1000
	minEngagementBufferSize     = 100
	maxEngagementBufferSize     = 10000

	// defaultOperatorChatBufferSize, minOperatorChatBufferSize and
	// maxOperatorChatBufferSize mirror internal/operatorchat's own
	// DefaultCapacity/MinCapacity/MaxCapacity exactly - duplicated here for
	// the same reason as the Engagement Event Bus's constants above, kept
	// in sync by TestOperatorChatBufferSizeConstantsMatchProjectionPackage.
	defaultOperatorChatBufferSize = 500
	minOperatorChatBufferSize     = 100
	maxOperatorChatBufferSize     = 5000
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
		Host:                   defaultHost,
		Port:                   defaultPort,
		AllowedOrigins:         defaultAllowedOrigins,
		ReadHeaderTimeout:      defaultReadHeaderTimeout,
		ShutdownTimeout:        defaultShutdownTimeout,
		EngagementBufferSize:   defaultEngagementBufferSize,
		OperatorChatBufferSize: defaultOperatorChatBufferSize,
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

	dataDir, err := resolveDataDir()
	if err != nil {
		return Config{}, err
	}
	cfg.DataDir = dataDir

	dbPath, err := resolveDatabasePath(dataDir)
	if err != nil {
		return Config{}, err
	}
	cfg.DatabasePath = dbPath

	mediaMTX, err := loadMediaMTX()
	if err != nil {
		return Config{}, err
	}
	cfg.MediaMTX = mediaMTX

	ffmpegCfg, err := loadFFmpeg()
	if err != nil {
		return Config{}, err
	}
	cfg.FFmpeg = ffmpegCfg

	if raw, ok := lookup("STREAMING_TREE_TWITCH_CLIENT_ID"); ok {
		cfg.TwitchClientID = strings.TrimSpace(raw)
	}

	if raw, ok := lookup("STREAMING_TREE_YOUTUBE_CLIENT_ID"); ok {
		cfg.YouTubeClientID = strings.TrimSpace(raw)
	}

	if raw, ok := lookup("STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE"); ok {
		size, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE: %q is not a number", raw)
		}
		if size < minEngagementBufferSize || size > maxEngagementBufferSize {
			return Config{}, fmt.Errorf(
				"STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE: %d is outside the range %d-%d",
				size, minEngagementBufferSize, maxEngagementBufferSize)
		}
		cfg.EngagementBufferSize = size
	}

	if raw, ok := lookup("STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE"); ok {
		size, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE: %q is not a number", raw)
		}
		if size < minOperatorChatBufferSize || size > maxOperatorChatBufferSize {
			return Config{}, fmt.Errorf(
				"STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE: %d is outside the range %d-%d",
				size, minOperatorChatBufferSize, maxOperatorChatBufferSize)
		}
		cfg.OperatorChatBufferSize = size
	}

	testNoUI, err := lookupBool("STREAMING_TREE_TEST_NO_UI", false)
	if err != nil {
		return Config{}, err
	}
	cfg.TestNoUI = testNoUI

	remoteManagement, err := loadRemoteManagement()
	if err != nil {
		return Config{}, err
	}
	cfg.RemoteManagement = remoteManagement

	return cfg, nil
}

// loadRemoteManagement reads the Stage 20D2B remote-management
// deployment settings. The external origin is read but not validated
// here - net/url parsing/validation happens in
// ValidateRemoteManagementOrigin, called explicitly by run() only
// when Enabled, so a desktop/D2A-only build that never sets these
// variables pays no validation cost and cannot be affected by a
// malformed value it does not use.
func loadRemoteManagement() (RemoteManagementConfig, error) {
	var cfg RemoteManagementConfig

	enabled, err := lookupBool("STREAMING_TREE_REMOTE_MANAGEMENT", false)
	if err != nil {
		return RemoteManagementConfig{}, err
	}
	cfg.Enabled = enabled

	if raw, ok := lookup("STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN"); ok {
		cfg.ExternalOrigin = raw
	}

	return cfg, nil
}

// loadFFmpeg reads the FFmpeg executable override, if any.
func loadFFmpeg() (FFmpegConfig, error) {
	var cfg FFmpegConfig

	if raw, ok := lookup("STREAMING_TREE_FFMPEG_PATH"); ok {
		absolute, err := filepath.Abs(raw)
		if err != nil {
			return FFmpegConfig{}, fmt.Errorf(
				"STREAMING_TREE_FFMPEG_PATH: %q is not a usable path: %w", raw, err)
		}
		cfg.ExecutablePath = absolute
	}

	return cfg, nil
}

// resolveDataDir decides where application data lives.
//
// STREAMING_TREE_DATA_DIR wins; otherwise the per-user configuration directory
// plus "StreamingTree" is used, which keeps runtime data out of the working
// copy. os.UserConfigDir resolves to %AppData% on Windows,
// ~/Library/Application Support on macOS and $XDG_CONFIG_HOME (or ~/.config)
// on Linux.
func resolveDataDir() (string, error) {
	if raw, ok := lookup("STREAMING_TREE_DATA_DIR"); ok {
		absolute, err := filepath.Abs(raw)
		if err != nil {
			return "", fmt.Errorf("STREAMING_TREE_DATA_DIR: %q is not a usable path: %w", raw, err)
		}
		return absolute, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf(
			"cannot determine the per-user configuration directory; "+
				"set STREAMING_TREE_DATA_DIR or STREAMING_TREE_DB_PATH: %w", err)
	}

	return filepath.Join(base, AppDirName), nil
}

// loadMediaMTX reads and validates the local ingest settings.
func loadMediaMTX() (MediaMTXConfig, error) {
	cfg := MediaMTXConfig{
		AutoStart:   true,
		AutoRestart: true,
		RTMPAddress: DefaultRTMPAddress,
		APIAddress:  DefaultAPIAddress,
		IngestPath:  DefaultIngestPath,
	}

	if raw, ok := lookup("STREAMING_TREE_MEDIAMTX_PATH"); ok {
		absolute, err := filepath.Abs(raw)
		if err != nil {
			return MediaMTXConfig{}, fmt.Errorf(
				"STREAMING_TREE_MEDIAMTX_PATH: %q is not a usable path: %w", raw, err)
		}
		cfg.ExecutablePath = absolute
	}

	autoStart, err := lookupBool("STREAMING_TREE_MEDIAMTX_AUTOSTART", cfg.AutoStart)
	if err != nil {
		return MediaMTXConfig{}, err
	}
	cfg.AutoStart = autoStart

	autoRestart, err := lookupBool("STREAMING_TREE_MEDIAMTX_AUTO_RESTART", cfg.AutoRestart)
	if err != nil {
		return MediaMTXConfig{}, err
	}
	cfg.AutoRestart = autoRestart

	if raw, ok := lookup("STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS"); ok {
		cfg.RTMPAddress = raw
	}
	if err := validateLoopbackAddress("STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS", cfg.RTMPAddress); err != nil {
		return MediaMTXConfig{}, err
	}

	if raw, ok := lookup("STREAMING_TREE_MEDIAMTX_API_ADDRESS"); ok {
		cfg.APIAddress = raw
	}
	if err := validateLoopbackAddress("STREAMING_TREE_MEDIAMTX_API_ADDRESS", cfg.APIAddress); err != nil {
		return MediaMTXConfig{}, err
	}

	if cfg.RTMPAddress == cfg.APIAddress {
		return MediaMTXConfig{}, fmt.Errorf(
			"the MediaMTX RTMP and Control API addresses must differ, both are %q", cfg.RTMPAddress)
	}

	if raw, ok := lookup("STREAMING_TREE_INGEST_PATH"); ok {
		cfg.IngestPath = raw
	}
	if err := ValidateIngestPath(cfg.IngestPath); err != nil {
		return MediaMTXConfig{}, err
	}

	return cfg, nil
}

// PublishURL is the full RTMP URL a publisher such as OBS connects to.
func (m MediaMTXConfig) PublishURL() string {
	return m.ServerURL() + "/" + m.IngestPath
}

// ServerURL is the value that goes in the OBS "Server" field.
func (m MediaMTXConfig) ServerURL() string {
	return "rtmp://" + m.RTMPAddress
}

// APIBaseURL is the loopback base URL of the MediaMTX Control API.
func (m MediaMTXConfig) APIBaseURL() string {
	return "http://" + m.APIAddress
}

// resolveDatabasePath decides where the SQLite file lives.
//
// STREAMING_TREE_DB_PATH names the file directly; otherwise the default
// filename is placed inside the already-resolved data directory.
func resolveDatabasePath(dataDir string) (string, error) {
	if raw, ok := lookup("STREAMING_TREE_DB_PATH"); ok {
		absolute, err := filepath.Abs(raw)
		if err != nil {
			return "", fmt.Errorf("STREAMING_TREE_DB_PATH: %q is not a usable path: %w", raw, err)
		}
		return absolute, nil
	}

	return filepath.Join(dataDir, DatabaseFileName), nil
}

// lookupBool reads a boolean environment variable.
//
// Only the spellings strconv.ParseBool understands are accepted; a typo such as
// "yes" is an error rather than a silent false, because silently disabling
// autostart would look like a bug in the application.
func lookupBool(key string, fallback bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf(
			"%s: %q is not a boolean; use true/false, 1/0, t/f", key, raw)
	}
	return parsed, nil
}

// ValidateHeadlessListenAddress rejects a management HTTP listener
// address that is not loopback-only (docs/linux-headless-server.md
// §6). Unlike the MediaMTX addresses (already unconditionally
// loopback-validated by loadMediaMTX on every platform/mode), the
// management listener has no such restriction by default - a desktop
// deployment may legitimately want a different local address in the
// future, but a headless service (no interactive user to notice a
// misconfiguration) must fail closed rather than silently bind
// somewhere reachable off the local machine. Call only when headless
// mode is explicitly requested; never call this for a desktop/dev
// build.
func ValidateHeadlessListenAddress(cfg Config) error {
	return validateLoopbackAddress("STREAMING_TREE_HOST/STREAMING_TREE_PORT (headless mode)", cfg.Address())
}

// ValidateRemoteManagementOrigin strictly validates the configured
// canonical external management origin (docs/remote-management.md
// §6): scheme must be exactly "https" (no production insecure
// fallback), a host is required, an explicit port is allowed, and no
// userinfo/path/query/fragment is permitted - this is an origin
// identity, never a URL with a meaningful path. Call only when
// RemoteManagement.Enabled is true.
func ValidateRemoteManagementOrigin(origin string) error {
	const key = "STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN"

	if origin == "" {
		return fmt.Errorf("%s: must be set when remote management is enabled", key)
	}

	parsed, err := neturl.Parse(origin)
	if err != nil {
		return fmt.Errorf("%s: %q is not a valid URL: %w", key, origin, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%s: %q must use the https scheme (no insecure remote management)", key, origin)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s: %q must not contain a username or password", key, origin)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s: %q must include a host", key, origin)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("%s: %q must not contain a path", key, origin)
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("%s: %q must not contain a query string", key, origin)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("%s: %q must not contain a fragment", key, origin)
	}
	if strings.Contains(origin, ",") {
		return fmt.Errorf("%s: %q must be exactly one origin, not a list", key, origin)
	}
	if strings.TrimSpace(origin) == "*" {
		return fmt.Errorf("%s: a wildcard origin is not permitted", key)
	}

	return nil
}

// CanonicalRemoteManagementOrigin returns origin normalized to
// scheme://host[:port] with no trailing slash - the exact form every
// Origin/forwarded-header comparison in internal/httpapi compares
// against. Call only after ValidateRemoteManagementOrigin has already
// accepted origin.
func CanonicalRemoteManagementOrigin(origin string) string {
	parsed, err := neturl.Parse(origin)
	if err != nil {
		return origin
	}
	return parsed.Scheme + "://" + parsed.Host
}

// validateLoopbackAddress rejects anything that is not a loopback host:port.
//
// MediaMTX is configured to accept an unauthenticated publisher and to expose a
// Control API that can rewrite its own configuration. Binding either to a
// routable interface would put both on the network, so a non-loopback address
// is refused outright rather than warned about.
func validateLoopbackAddress(key, address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s: %q must be in host:port form: %w", key, address, err)
	}

	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("%s: %q has a non-numeric port", key, address)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s: port %d is outside the range 1-65535", key, port)
	}

	if host == "localhost" {
		return nil
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf(
			"%s: %q must be a loopback address such as 127.0.0.1 or localhost", key, host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf(
			"%s: %q is not a loopback address; MediaMTX must not be reachable from the network",
			key, host)
	}

	return nil
}

// ValidateIngestPath checks the MediaMTX path publishing is allowed on.
//
// The value ends up both in the generated MediaMTX configuration and in an RTMP
// URL, so it is restricted to a conservative character set: no slashes, no
// query strings, no relative segments.
func ValidateIngestPath(path string) error {
	const key = "STREAMING_TREE_INGEST_PATH"

	if path == "" {
		return fmt.Errorf("%s: must not be empty", key)
	}
	if len(path) > 64 {
		return fmt.Errorf("%s: %q is longer than 64 characters", key, path)
	}
	if path == "." || path == ".." {
		return fmt.Errorf("%s: %q is a relative path segment", key, path)
	}
	if !ingestPathPattern.MatchString(path) {
		return fmt.Errorf(
			"%s: %q may only contain letters, digits, '-' and '_'", key, path)
	}
	// "all" and "all_others" are MediaMTX wildcard path names; using one would
	// silently widen publishing to every path.
	if path == "all" || path == "all_others" {
		return fmt.Errorf("%s: %q is a MediaMTX wildcard name and cannot be used", key, path)
	}

	return nil
}

// ingestPathPattern deliberately excludes '/', '.', '?' and '#'.
var ingestPathPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Address returns the host:port string accepted by net.Listen.
//
// net.JoinHostPort (not a bare "+") is required here: a literal IPv6
// host such as "::1" must be bracketed ("[::1]:8080") or
// net.SplitHostPort/net.Listen reject the whole string as ambiguous
// ("too many colons in address") - a real, pre-existing gap this
// package's own Stage 20D2A headless-mode listener validation surfaced
// (docs/linux-headless-server.md §6), since STREAMING_TREE_HOST=::1 is
// a legitimate loopback value MediaMTX's own address fields already
// accept when given as one whole string.
func (c Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
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
