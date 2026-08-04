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
