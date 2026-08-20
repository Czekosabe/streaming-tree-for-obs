package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// clearEnv removes every variable Load reads, so one test cannot leak into
// another through the process environment.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"STREAMING_TREE_HOST",
		"STREAMING_TREE_PORT",
		"STREAMING_TREE_ALLOWED_ORIGINS",
		"STREAMING_TREE_DATA_DIR",
		"STREAMING_TREE_DB_PATH",
		"STREAMING_TREE_MEDIAMTX_PATH",
		"STREAMING_TREE_MEDIAMTX_AUTOSTART",
		"STREAMING_TREE_MEDIAMTX_AUTO_RESTART",
		"STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS",
		"STREAMING_TREE_MEDIAMTX_API_ADDRESS",
		"STREAMING_TREE_INGEST_PATH",
		"STREAMING_TREE_FFMPEG_PATH",
		"STREAMING_TREE_TWITCH_CLIENT_ID",
		"STREAMING_TREE_YOUTUBE_CLIENT_ID",
		"STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE",
		"STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE",
		"STREAMING_TREE_REMOTE_MANAGEMENT",
		"STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN",
		"STREAMING_TREE_REMOTE_INGEST",
		"STREAMING_TREE_REMOTE_INGEST_RTMPS_ADDRESS",
		"STREAMING_TREE_REMOTE_INGEST_TLS_KEY_PATH",
		"STREAMING_TREE_REMOTE_INGEST_TLS_CERT_PATH",
		"STREAMING_TREE_REMOTE_INGEST_OVERLAY_ORIGIN",
	} {
		t.Setenv(key, "")
	}
}

func TestMediaMTXDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	if !cfg.MediaMTX.AutoStart {
		t.Error("AutoStart = false, want true by default")
	}
	if !cfg.MediaMTX.AutoRestart {
		t.Error("AutoRestart = false, want true by default")
	}
	if cfg.MediaMTX.RTMPAddress != DefaultRTMPAddress {
		t.Errorf("RTMPAddress = %q, want %q", cfg.MediaMTX.RTMPAddress, DefaultRTMPAddress)
	}
	if cfg.MediaMTX.APIAddress != DefaultAPIAddress {
		t.Errorf("APIAddress = %q, want %q", cfg.MediaMTX.APIAddress, DefaultAPIAddress)
	}
	if cfg.MediaMTX.IngestPath != DefaultIngestPath {
		t.Errorf("IngestPath = %q, want %q", cfg.MediaMTX.IngestPath, DefaultIngestPath)
	}
	if cfg.MediaMTX.ExecutablePath != "" {
		t.Errorf("ExecutablePath = %q, want empty by default", cfg.MediaMTX.ExecutablePath)
	}
}

func TestMediaMTXDerivedURLs(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	// The OBS server field, the stream key and the publish URL must all derive
	// from the same configured values rather than being separate constants.
	if got := cfg.MediaMTX.ServerURL(); got != "rtmp://127.0.0.1:1935" {
		t.Errorf("ServerURL() = %q, want rtmp://127.0.0.1:1935", got)
	}
	if got := cfg.MediaMTX.PublishURL(); got != "rtmp://127.0.0.1:1935/live" {
		t.Errorf("PublishURL() = %q, want rtmp://127.0.0.1:1935/live", got)
	}
	if got := cfg.MediaMTX.APIBaseURL(); got != "http://127.0.0.1:9997" {
		t.Errorf("APIBaseURL() = %q, want http://127.0.0.1:9997", got)
	}
}

func TestMediaMTXExplicitOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_MEDIAMTX_AUTOSTART", "false")
	t.Setenv("STREAMING_TREE_MEDIAMTX_AUTO_RESTART", "0")
	t.Setenv("STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS", "127.0.0.1:2935")
	t.Setenv("STREAMING_TREE_MEDIAMTX_API_ADDRESS", "localhost:19997")
	t.Setenv("STREAMING_TREE_INGEST_PATH", "obs_input")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	if cfg.MediaMTX.AutoStart || cfg.MediaMTX.AutoRestart {
		t.Error("the boolean overrides were not applied")
	}
	if cfg.MediaMTX.RTMPAddress != "127.0.0.1:2935" {
		t.Errorf("RTMPAddress = %q", cfg.MediaMTX.RTMPAddress)
	}
	if cfg.MediaMTX.IngestPath != "obs_input" {
		t.Errorf("IngestPath = %q", cfg.MediaMTX.IngestPath)
	}
}

func TestMediaMTXPathIsMadeAbsolute(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_MEDIAMTX_PATH", filepath.Join("relative", "mediamtx"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if !filepath.IsAbs(cfg.MediaMTX.ExecutablePath) {
		t.Errorf("ExecutablePath = %q, want an absolute path", cfg.MediaMTX.ExecutablePath)
	}
}

func TestFFmpegPathDefaultsToEmpty(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.FFmpeg.ExecutablePath != "" {
		t.Errorf("ExecutablePath = %q, want empty by default", cfg.FFmpeg.ExecutablePath)
	}
}

func TestFFmpegPathIsMadeAbsolute(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_FFMPEG_PATH", filepath.Join("relative", "ffmpeg"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if !filepath.IsAbs(cfg.FFmpeg.ExecutablePath) {
		t.Errorf("ExecutablePath = %q, want an absolute path", cfg.FFmpeg.ExecutablePath)
	}
}

func TestInvalidBooleanIsRejected(t *testing.T) {
	// A typo must fail loudly: silently disabling autostart would look like a
	// bug in the application rather than a configuration mistake.
	for _, value := range []string{"yes", "on", "maybe", "2"} {
		t.Run(value, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("STREAMING_TREE_MEDIAMTX_AUTOSTART", value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted the invalid boolean %q", value)
			}
		})
	}
}

func TestNonLoopbackAddressesAreRejected(t *testing.T) {
	cases := map[string]string{
		"all interfaces":   "0.0.0.0:1935",
		"empty host":       ":1935",
		"routable IPv4":    "192.168.1.10:1935",
		"public IPv4":      "8.8.8.8:1935",
		"hostname":         "example.com:1935",
		"IPv6 unspecified": "[::]:1935",
	}

	for name, address := range cases {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS", address)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted the non-loopback address %q", address)
			}
		})
	}
}

func TestLoopbackAddressesAreAccepted(t *testing.T) {
	for _, address := range []string{"127.0.0.1:1935", "localhost:1935", "[::1]:1935", "127.0.0.5:1935"} {
		t.Run(address, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS", address)

			if _, err := Load(); err != nil {
				t.Fatalf("Load() rejected the loopback address %q: %v", address, err)
			}
		})
	}
}

func TestValidateHeadlessListenAddressRejectsNonLoopback(t *testing.T) {
	cases := map[string]string{
		"all interfaces":   "0.0.0.0",
		"routable IPv4":    "192.168.1.10",
		"public IPv4":      "8.8.8.8",
		"IPv6 unspecified": "::",
	}

	for name, host := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Host: host, Port: defaultPort}
			if err := ValidateHeadlessListenAddress(cfg); err == nil {
				t.Fatalf("ValidateHeadlessListenAddress() accepted the non-loopback host %q", host)
			}
		})
	}
}

func TestValidateHeadlessListenAddressAcceptsLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "::1", "127.0.0.5"} {
		t.Run(host, func(t *testing.T) {
			cfg := Config{Host: host, Port: defaultPort}
			if err := ValidateHeadlessListenAddress(cfg); err != nil {
				t.Fatalf("ValidateHeadlessListenAddress() rejected the loopback host %q: %v", host, err)
			}
		})
	}
}

func TestInvalidMediaMTXPortsAreRejected(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "127.0.0.1:70000", "127.0.0.1:abc", "127.0.0.1"} {
		t.Run(address, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS", address)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted the invalid address %q", address)
			}
		})
	}
}

func TestIdenticalRTMPAndAPIAddressesAreRejected(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS", "127.0.0.1:9000")
	t.Setenv("STREAMING_TREE_MEDIAMTX_API_ADDRESS", "127.0.0.1:9000")

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted the same address for RTMP and the Control API")
	}
}

func TestIngestPathValidation(t *testing.T) {
	valid := []string{"live", "obs", "obs_input", "stream-1", "A1"}
	for _, path := range valid {
		if err := ValidateIngestPath(path); err != nil {
			t.Errorf("ValidateIngestPath(%q) returned an error: %v", path, err)
		}
	}

	invalid := map[string]string{
		"empty":         "",
		"slash":         "live/stream",
		"traversal":     "..",
		"dot":           ".",
		"parent":        "../etc",
		"query string":  "live?token=1",
		"fragment":      "live#x",
		"space":         "live stream",
		"wildcard":      "all",
		"wildcard rest": "all_others",
		"backslash":     `live\stream`,
	}
	for name, path := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateIngestPath(path); err == nil {
				t.Fatalf("ValidateIngestPath(%q) accepted an unsafe path", path)
			}
		})
	}
}

func TestDataDirDefaultsBesideTheDatabase(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("STREAMING_TREE_DATA_DIR", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	if cfg.DatabasePath != filepath.Join(dir, DatabaseFileName) {
		t.Errorf("DatabasePath = %q, want it inside the data directory", cfg.DatabasePath)
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if !filepath.IsAbs(cfg.DatabasePath) {
		t.Errorf("DatabasePath = %q, want an absolute path", cfg.DatabasePath)
	}
	if filepath.Base(cfg.DatabasePath) != DatabaseFileName {
		t.Errorf("database file = %q, want %q", filepath.Base(cfg.DatabasePath), DatabaseFileName)
	}
	// The default must never land inside the repository working copy.
	if !strings.Contains(cfg.DatabasePath, AppDirName) {
		t.Errorf("DatabasePath = %q, want it inside a %q directory", cfg.DatabasePath, AppDirName)
	}
}

func TestDataDirOverride(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("STREAMING_TREE_DATA_DIR", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	want := filepath.Join(dir, DatabaseFileName)
	if cfg.DatabasePath != want {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

func TestDBPathTakesPrecedenceOverDataDir(t *testing.T) {
	clearEnv(t)
	dataDir := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "nested", "custom.db")

	t.Setenv("STREAMING_TREE_DATA_DIR", dataDir)
	t.Setenv("STREAMING_TREE_DB_PATH", explicit)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	if cfg.DatabasePath != explicit {
		t.Errorf("DatabasePath = %q, want the explicit path %q", cfg.DatabasePath, explicit)
	}
}

func TestRelativeDatabasePathIsMadeAbsolute(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_DB_PATH", filepath.Join("relative", "streaming.db"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	if !filepath.IsAbs(cfg.DatabasePath) {
		t.Errorf("DatabasePath = %q, want an absolute path", cfg.DatabasePath)
	}
}

func TestInvalidPortIsRejected(t *testing.T) {
	tests := map[string]string{
		"not a number": "abc",
		"zero":         "0",
		"too large":    "70000",
		"negative":     "-1",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("STREAMING_TREE_PORT", value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted the invalid port %q", value)
			}
		})
	}
}

func TestAllowedOriginsOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_ALLOWED_ORIGINS", " http://localhost:3000 , http://127.0.0.1:4000 ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	want := []string{"http://localhost:3000", "http://127.0.0.1:4000"}
	if len(cfg.AllowedOrigins) != len(want) {
		t.Fatalf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, want)
	}
	for i, origin := range want {
		if cfg.AllowedOrigins[i] != origin {
			t.Errorf("AllowedOrigins[%d] = %q, want %q", i, cfg.AllowedOrigins[i], origin)
		}
	}
}

func TestBlankEnvironmentValueFallsBackToDefault(t *testing.T) {
	clearEnv(t)
	// An empty variable is treated as "not set" rather than as an empty host.
	t.Setenv("STREAMING_TREE_HOST", "   ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want the default 127.0.0.1", cfg.Host)
	}
}

func TestTwitchAndYouTubeClientIDsAreIndependentEnvironmentOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_TWITCH_CLIENT_ID", "  twitch-cid  ")
	t.Setenv("STREAMING_TREE_YOUTUBE_CLIENT_ID", "youtube-cid")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.TwitchClientID != "twitch-cid" {
		t.Errorf("TwitchClientID = %q, want trimmed twitch-cid", cfg.TwitchClientID)
	}
	if cfg.YouTubeClientID != "youtube-cid" {
		t.Errorf("YouTubeClientID = %q, want youtube-cid", cfg.YouTubeClientID)
	}
}

func TestClientIDsDefaultToEmptyWhenUnset(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.TwitchClientID != "" || cfg.YouTubeClientID != "" {
		t.Errorf("TwitchClientID/YouTubeClientID = %q/%q, want both empty when unset", cfg.TwitchClientID, cfg.YouTubeClientID)
	}
}

func TestEngagementBufferSizeDefault(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.EngagementBufferSize != 1000 {
		t.Errorf("EngagementBufferSize = %d, want default 1000", cfg.EngagementBufferSize)
	}
}

func TestEngagementBufferSizeOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE", "2500")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.EngagementBufferSize != 2500 {
		t.Errorf("EngagementBufferSize = %d, want 2500", cfg.EngagementBufferSize)
	}
}

func TestEngagementBufferSizeRejectsOutOfRangeValues(t *testing.T) {
	for _, raw := range []string{"0", "99", "10001", "not-a-number", "-5"} {
		t.Run(raw, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE", raw)
			if _, err := Load(); err == nil {
				t.Errorf("expected Load() to reject STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE=%q", raw)
			}
		})
	}
}

func TestEngagementBufferSizeConstantsMatchBusPackage(t *testing.T) {
	// internal/engagement.DefaultCapacity/MinCapacity/MaxCapacity must stay
	// numerically identical to this package's own duplicated constants (see
	// their doc comment for why they are duplicated rather than imported).
	// This package cannot import internal/engagement directly without
	// creating a dependency a low-level config package should not have, so
	// the two are pinned together with literal values here instead - if a
	// future change updates one without the other, this test catches it.
	if defaultEngagementBufferSize != 1000 {
		t.Errorf("defaultEngagementBufferSize = %d, want 1000 (must match internal/engagement.DefaultCapacity)", defaultEngagementBufferSize)
	}
	if minEngagementBufferSize != 100 {
		t.Errorf("minEngagementBufferSize = %d, want 100 (must match internal/engagement.MinCapacity)", minEngagementBufferSize)
	}
	if maxEngagementBufferSize != 10000 {
		t.Errorf("maxEngagementBufferSize = %d, want 10000 (must match internal/engagement.MaxCapacity)", maxEngagementBufferSize)
	}
}

func TestOperatorChatBufferSizeDefault(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.OperatorChatBufferSize != 500 {
		t.Errorf("OperatorChatBufferSize = %d, want default 500", cfg.OperatorChatBufferSize)
	}
}

func TestOperatorChatBufferSizeOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE", "1200")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.OperatorChatBufferSize != 1200 {
		t.Errorf("OperatorChatBufferSize = %d, want 1200", cfg.OperatorChatBufferSize)
	}
}

func TestOperatorChatBufferSizeRejectsOutOfRangeValues(t *testing.T) {
	for _, raw := range []string{"0", "99", "5001", "not-a-number", "-5"} {
		t.Run(raw, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE", raw)
			if _, err := Load(); err == nil {
				t.Errorf("expected Load() to reject STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE=%q", raw)
			}
		})
	}
}

func TestOperatorChatBufferSizeConstantsMatchProjectionPackage(t *testing.T) {
	// internal/operatorchat.DefaultCapacity/MinCapacity/MaxCapacity must
	// stay numerically identical to this package's own duplicated
	// constants - see defaultOperatorChatBufferSize's own doc comment for
	// why they are duplicated rather than imported (this package cannot
	// import internal/operatorchat without creating a dependency a
	// low-level config package should not have).
	if defaultOperatorChatBufferSize != 500 {
		t.Errorf("defaultOperatorChatBufferSize = %d, want 500 (must match internal/operatorchat.DefaultCapacity)", defaultOperatorChatBufferSize)
	}
	if minOperatorChatBufferSize != 100 {
		t.Errorf("minOperatorChatBufferSize = %d, want 100 (must match internal/operatorchat.MinCapacity)", minOperatorChatBufferSize)
	}
	if maxOperatorChatBufferSize != 5000 {
		t.Errorf("maxOperatorChatBufferSize = %d, want 5000 (must match internal/operatorchat.MaxCapacity)", maxOperatorChatBufferSize)
	}
}

func TestRemoteManagementDefaultsDisabled(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.RemoteManagement.Enabled {
		t.Error("RemoteManagement.Enabled = true by default, want false")
	}
	if cfg.RemoteManagement.ExternalOrigin != "" {
		t.Errorf("RemoteManagement.ExternalOrigin = %q by default, want empty", cfg.RemoteManagement.ExternalOrigin)
	}
}

func TestRemoteManagementEnabledFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_REMOTE_MANAGEMENT", "true")
	t.Setenv("STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN", "https://stream.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if !cfg.RemoteManagement.Enabled {
		t.Error("RemoteManagement.Enabled = false, want true")
	}
	if cfg.RemoteManagement.ExternalOrigin != "https://stream.example.com" {
		t.Errorf("RemoteManagement.ExternalOrigin = %q, want %q",
			cfg.RemoteManagement.ExternalOrigin, "https://stream.example.com")
	}
}

func TestRemoteManagementRejectsNonBooleanFlag(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_REMOTE_MANAGEMENT", "yes-please")

	if _, err := Load(); err == nil {
		t.Error("Load() with a non-boolean STREAMING_TREE_REMOTE_MANAGEMENT succeeded, want an error")
	}
}

func TestValidateRemoteManagementOriginAcceptsValidHTTPS(t *testing.T) {
	cases := []string{
		"https://stream.example.com",
		"https://stream.example.com:8443",
		"https://sub.stream.example.com",
	}
	for _, origin := range cases {
		if err := ValidateRemoteManagementOrigin(origin); err != nil {
			t.Errorf("ValidateRemoteManagementOrigin(%q) = %v, want nil", origin, err)
		}
	}
}

func TestValidateRemoteManagementOriginRejectsInvalid(t *testing.T) {
	cases := []string{
		"",
		"http://stream.example.com",            // insecure scheme
		"stream.example.com",                   // no scheme
		"https://user:pass@stream.example.com", // userinfo
		"https://",                             // no host
		"https://stream.example.com/admin",     // path
		"https://stream.example.com?x=1",       // query
		"https://stream.example.com#frag",      // fragment
		"*",                                    // wildcard
		"https://a.example.com,https://b.example.com", // list
	}
	for _, origin := range cases {
		if err := ValidateRemoteManagementOrigin(origin); err == nil {
			t.Errorf("ValidateRemoteManagementOrigin(%q) = nil, want an error", origin)
		}
	}
}

func TestCanonicalRemoteManagementOriginNormalizesForm(t *testing.T) {
	got := CanonicalRemoteManagementOrigin("https://stream.example.com:8443")
	want := "https://stream.example.com:8443"
	if got != want {
		t.Errorf("CanonicalRemoteManagementOrigin() = %q, want %q", got, want)
	}
}

func TestRemoteIngestDefaultsToDisabled(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.RemoteIngest.Enabled {
		t.Error("RemoteIngest.Enabled defaulted to true")
	}
	if cfg.RemoteIngest.RTMPSAddress != "" || cfg.RemoteIngest.ServerKeyPath != "" ||
		cfg.RemoteIngest.ServerCertPath != "" || cfg.RemoteIngest.OverlayOrigin != "" {
		t.Error("RemoteIngest fields are non-empty with nothing set")
	}
}

func TestRemoteIngestReadsEveryEnvironmentVariable(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_REMOTE_INGEST", "true")
	t.Setenv("STREAMING_TREE_REMOTE_INGEST_RTMPS_ADDRESS", "0.0.0.0:1936")
	t.Setenv("STREAMING_TREE_REMOTE_INGEST_TLS_KEY_PATH", "/etc/streaming-tree/rtmps.key")
	t.Setenv("STREAMING_TREE_REMOTE_INGEST_TLS_CERT_PATH", "/etc/streaming-tree/rtmps.crt")
	t.Setenv("STREAMING_TREE_REMOTE_INGEST_OVERLAY_ORIGIN", "https://overlay.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if !cfg.RemoteIngest.Enabled {
		t.Error("RemoteIngest.Enabled = false, want true")
	}
	if cfg.RemoteIngest.RTMPSAddress != "0.0.0.0:1936" {
		t.Errorf("RTMPSAddress = %q", cfg.RemoteIngest.RTMPSAddress)
	}
	if cfg.RemoteIngest.ServerKeyPath != "/etc/streaming-tree/rtmps.key" {
		t.Errorf("ServerKeyPath = %q", cfg.RemoteIngest.ServerKeyPath)
	}
	if cfg.RemoteIngest.ServerCertPath != "/etc/streaming-tree/rtmps.crt" {
		t.Errorf("ServerCertPath = %q", cfg.RemoteIngest.ServerCertPath)
	}
	if cfg.RemoteIngest.OverlayOrigin != "https://overlay.example.com" {
		t.Errorf("OverlayOrigin = %q", cfg.RemoteIngest.OverlayOrigin)
	}
}

func TestRemoteIngestRejectsNonBooleanFlag(t *testing.T) {
	clearEnv(t)
	t.Setenv("STREAMING_TREE_REMOTE_INGEST", "sure")

	if _, err := Load(); err == nil {
		t.Error("Load() with a non-boolean STREAMING_TREE_REMOTE_INGEST succeeded, want an error")
	}
}

func TestValidateRemoteIngestPreconditionsAcceptsAWellFormedConfig(t *testing.T) {
	cfg := Config{
		MediaMTX: MediaMTXConfig{RTMPAddress: DefaultRTMPAddress, APIAddress: DefaultAPIAddress},
		RemoteIngest: RemoteIngestConfig{
			Enabled:        true,
			RTMPSAddress:   "0.0.0.0:1936",
			ServerKeyPath:  "/etc/streaming-tree/rtmps.key",
			ServerCertPath: "/etc/streaming-tree/rtmps.crt",
		},
	}
	if err := ValidateRemoteIngestPreconditions(cfg); err != nil {
		t.Errorf("ValidateRemoteIngestPreconditions() = %v, want nil", err)
	}
}

func TestValidateRemoteIngestPreconditionsRejectsInvalid(t *testing.T) {
	base := func() Config {
		return Config{
			MediaMTX: MediaMTXConfig{RTMPAddress: DefaultRTMPAddress, APIAddress: DefaultAPIAddress},
			RemoteIngest: RemoteIngestConfig{
				Enabled:        true,
				RTMPSAddress:   "0.0.0.0:1936",
				ServerKeyPath:  "/etc/streaming-tree/rtmps.key",
				ServerCertPath: "/etc/streaming-tree/rtmps.crt",
			},
		}
	}

	cases := map[string]func(Config) Config{
		"empty RTMPS address": func(c Config) Config {
			c.RemoteIngest.RTMPSAddress = ""
			return c
		},
		"malformed RTMPS address": func(c Config) Config {
			c.RemoteIngest.RTMPSAddress = "not-an-address"
			return c
		},
		"RTMPS address collides with RTMPAddress": func(c Config) Config {
			c.RemoteIngest.RTMPSAddress = c.MediaMTX.RTMPAddress
			return c
		},
		"RTMPS address collides with APIAddress": func(c Config) Config {
			c.RemoteIngest.RTMPSAddress = c.MediaMTX.APIAddress
			return c
		},
		"empty key path": func(c Config) Config {
			c.RemoteIngest.ServerKeyPath = ""
			return c
		},
		"empty cert path": func(c Config) Config {
			c.RemoteIngest.ServerCertPath = ""
			return c
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRemoteIngestPreconditions(mutate(base())); err == nil {
				t.Error("ValidateRemoteIngestPreconditions() = nil, want an error")
			}
		})
	}
}

func TestValidateRemoteOverlayOriginRequiresADifferentHostnameThanManagement(t *testing.T) {
	management := "https://stream.example.com"

	cases := map[string]struct {
		overlay string
		wantErr bool
	}{
		"different hostname":                {"https://overlay.example.com", false},
		"same hostname, no port difference": {"https://stream.example.com", true},
		"same hostname, different port": {
			// RFC 6265 §8.5: cookies are not port-scoped, so this must
			// still be rejected even though it is a different web
			// Origin (docs/remote-ingest.md §10).
			"https://stream.example.com:8443", true,
		},
		"same hostname, trailing slash": {"https://stream.example.com/", true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateRemoteOverlayOrigin(tc.overlay, management)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateRemoteOverlayOrigin(%q, %q) = nil, want an error", tc.overlay, management)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateRemoteOverlayOrigin(%q, %q) = %v, want nil", tc.overlay, management, err)
			}
		})
	}
}

func TestValidateRemoteOverlayOriginRejectsInvalidOrigins(t *testing.T) {
	management := "https://stream.example.com"
	cases := []string{
		"",
		"http://overlay.example.com",          // insecure scheme
		"overlay.example.com",                 // no scheme
		"https://overlay.example.com/widgets", // path
	}
	for _, overlay := range cases {
		if err := ValidateRemoteOverlayOrigin(overlay, management); err == nil {
			t.Errorf("ValidateRemoteOverlayOrigin(%q, ...) = nil, want an error", overlay)
		}
	}
}
