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
	} {
		t.Setenv(key, "")
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
